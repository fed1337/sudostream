package transcode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sudoStream/internal/observability"
	"sync"
	"time"
)

const (
	completeMarker      = ".complete"
	errorMarker         = ".error"
	hlsPlaylistHeader   = "#EXTM3U"
	hlsPlaylistVersion6 = "#EXT-X-VERSION:6"
)

// Status represents the state of a transcode job.
type Status string

const (
	// StatusIdle indicates no transcode job or cache exists yet.
	StatusIdle Status = "idle"
	// StatusReady indicates playlists are published and segments can be pulled on demand.
	StatusReady Status = "ready"
	// StatusProcessing indicates the source is still being probed.
	StatusProcessing Status = "processing"
	// StatusError indicates the job failed.
	StatusError Status = "error"
)

// JobStatus holds the current state of a transcode job.
type JobStatus struct {
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Service manages HLS playlist publication and on-demand segment sessions.
type Service struct {
	cacheDir     string
	jobs         map[string]*Job
	sessions     map[string]map[string]*segmentSession
	resolveLocks map[string]*sync.Mutex
	lastUsed     map[string]time.Time
	packageBytes map[string]int64
	cacheBytes   int64
	mu           sync.Mutex
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup
	settings     func() TranscodeSettings
}

// NewService initializes a transcode service with the given cache directory.
func NewService(cacheDir string) (*Service, error) {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "sudostream_hls")
	}
	//nolint:mnd // permissions
	err := os.MkdirAll(cacheDir, 0o750)
	if err != nil {
		return nil, fmt.Errorf("create hls cache dir: %w", err)
	}

	service := &Service{
		cacheDir:     cacheDir,
		jobs:         make(map[string]*Job),
		sessions:     make(map[string]map[string]*segmentSession),
		lastUsed:     make(map[string]time.Time),
		packageBytes: make(map[string]int64),
		stopCh:       make(chan struct{}),
		settings:     DefaultTranscodeSettings,
	}
	service.hydrateCacheAccounting()

	service.wg.Add(1)

	go service.reapIdleSessions()

	return service, nil
}

// SetSettingsProvider sets how the service reads live admin transcode settings.
func (s *Service) SetSettingsProvider(provider func() TranscodeSettings) {
	if s == nil {
		return
	}
	if provider == nil {
		s.settings = DefaultTranscodeSettings

		return
	}

	s.settings = provider
}

// SetHwAccelProvider sets how the service reads the admin HW preference.
// Deprecated path kept for tests; prefer SetSettingsProvider.
func (s *Service) SetHwAccelProvider(provider func() HwAccel) {
	if s == nil {
		return
	}
	if provider == nil {
		s.settings = DefaultTranscodeSettings

		return
	}

	s.settings = func() TranscodeSettings {
		settings := DefaultTranscodeSettings()
		settings.HwAccel = provider()

		return settings
	}
}

// Shutdown cancels running ffmpeg jobs and waits for workers to exit.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil || s.stopCh == nil {
		return nil
	}

	s.stopOnce.Do(func() {
		close(s.stopCh)
	})

	done := make(chan struct{})

	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("transcode shutdown: %w", ctx.Err())
	}
}

// CacheKey returns the cache key for a media file based on path, mtime, size, and tonemap prefs.
// Version suffix bumps invalidate caches when encode-graph semantics change (FI-5 tonemap/downmix).
func (s *Service) CacheKey(mediaPath string, mtime int64, size int64) string {
	settings := s.currentSettings()
	tmFlag := "0"
	if settings.ToneMappingEnabled {
		tmFlag = "1"
	}
	hashData := fmt.Sprintf(
		"%s:%d:%d:v26:tm=%s:%s:dm=%s:%g",
		mediaPath,
		mtime,
		size,
		tmFlag,
		settings.ToneMappingAlgorithm,
		settings.DownmixAlgorithm,
		settings.DownmixBoost,
	)
	hash := sha256.Sum256([]byte(hashData))

	return hex.EncodeToString(hash[:])
}

// CacheOutDir returns the on-disk cache directory for a cache key.
func (s *Service) CacheOutDir(cacheKey string) string {
	return filepath.Join(s.cacheDir, cacheKey)
}

// MasterPlaylistPath returns the path to the HLS master playlist for a cache key.
func (s *Service) MasterPlaylistPath(cacheKey string) string {
	return filepath.Join(s.cacheDir, cacheKey, masterPlaylistName)
}

// ResourcePath returns the path to an HLS resource under a cache key.
func (s *Service) ResourcePath(cacheKey, resource string) string {
	if resource == "" || strings.Contains(resource, "..") {
		return ""
	}

	return filepath.Join(s.cacheDir, cacheKey, filepath.FromSlash(resource))
}

// Status reports whether the HLS timeline for a cache key is published yet.
func (s *Service) Status(cacheKey string) JobStatus {
	outDir := filepath.Join(s.cacheDir, cacheKey)

	s.mu.Lock()
	job, hasJob := s.jobs[cacheKey]
	s.mu.Unlock()

	if hasJob && job.Failed {
		return JobStatus{Status: StatusError, Error: job.ErrorMsg}
	}

	if isTranscodeComplete(outDir) {
		return JobStatus{Status: StatusReady}
	}

	if errMsg, ok := readTranscodeError(outDir); ok {
		return JobStatus{Status: StatusError, Error: errMsg}
	}

	if hasJob || dirExists(outDir) {
		return JobStatus{Status: StatusProcessing}
	}

	return JobStatus{Status: StatusIdle}
}

// HoldProcessingJob registers an in-flight job without starting ffmpeg.
// Handler tests use it to exercise processing responses without real media files.
func (s *Service) HoldProcessingJob(cacheKey string) {
	if s == nil || cacheKey == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs[cacheKey] = &Job{
		cacheKey: cacheKey,
		outDir:   filepath.Join(s.cacheDir, cacheKey),
	}
	s.publishJobGaugesLocked()
}

// StartJob probes a source and publishes its HLS playlists if that has not happened yet.
func (s *Service) StartJob(_ context.Context, cacheKey, mediaPath string) error {
	s.mu.Lock()

	outDir := filepath.Join(s.cacheDir, cacheKey)
	if isTranscodeComplete(outDir) {
		s.mu.Unlock()
		observability.RecordHLSCacheLookup("hit")
		s.touchCache(cacheKey)

		return nil
	}

	if _, ok := s.jobs[cacheKey]; ok {
		s.mu.Unlock()

		return nil
	}

	if _, hasErr := readTranscodeError(outDir); hasErr {
		s.mu.Unlock()

		return nil
	}

	observability.RecordHLSCacheLookup("miss")

	//nolint:mnd // permissions
	err := os.MkdirAll(outDir, 0o750)
	if err != nil {
		s.mu.Unlock()

		return fmt.Errorf("create out dir: %w", err)
	}

	job := &Job{cacheKey: cacheKey, mediaPath: mediaPath, outDir: outDir}
	s.jobs[cacheKey] = job
	s.publishJobGaugesLocked()
	s.wg.Add(1)
	s.mu.Unlock()

	//nolint:contextcheck // background probe uses service-owned shutdown context
	go job.Run(s)
	s.touchCache(cacheKey)

	return nil
}

// RemoveCache deletes the cached HLS package for a cache key.
func (s *Service) RemoveCache(cacheKey string) {
	if cacheKey == "" {
		return
	}

	s.mu.Lock()
	sessions := s.sessions[cacheKey]
	delete(s.jobs, cacheKey)
	delete(s.sessions, cacheKey)
	s.publishJobGaugesLocked()
	s.mu.Unlock()

	for _, session := range sessions {
		session.cancel()
		<-session.done
	}

	s.forgetPackageAccounting(cacheKey)
	_ = os.RemoveAll(filepath.Join(s.cacheDir, cacheKey))
	s.publishCacheStats()
}

// PurgeStaleCache deletes top-level HLS packages that are not in jobs/sessions and have
// been unused for at least unusedFor (default 15m). Eligibility uses top-level dir mtime
// plus in-memory last-used; sizes come from tracked package bytes (no full-tree walk).
func (s *Service) PurgeStaleCache(unusedFor time.Duration) (int, int64, error) {
	if s == nil || s.cacheDir == "" {
		return 0, 0, nil
	}
	if unusedFor <= 0 {
		unusedFor = defaultUnusedGrace
	}

	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return 0, 0, fmt.Errorf("read hls cache dir: %w", err)
	}

	now := time.Now()
	inUse := s.inUseCacheKeys()
	var deleted int
	var bytesRemoved int64
	for _, entry := range entries {
		removed, size := s.purgePackageIfUnused(entry, now, unusedFor, inUse)
		if removed {
			deleted++
			bytesRemoved += size
		}
	}

	s.publishCacheStats()

	return deleted, bytesRemoved, nil
}

// CacheDirBytes returns the tracked total size of HLS cache packages.
func (s *Service) CacheDirBytes() int64 {
	_, bytes := s.CacheDirStats()

	return bytes
}

// CacheDirStats returns top-level package count and tracked total bytes (no full-tree walk).
func (s *Service) CacheDirStats() (int, int64) {
	if s == nil || s.cacheDir == "" {
		return 0, 0
	}

	dirEntries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		return 0, 0
	}
	entries := 0
	for _, entry := range dirEntries {
		if entry.IsDir() {
			entries++
		}
	}

	s.mu.Lock()
	bytes := s.cacheBytes
	s.mu.Unlock()

	return entries, bytes
}

// CurrentSettings returns the live admin transcode preferences.
func (s *Service) CurrentSettings() TranscodeSettings {
	return s.currentSettings()
}

func (s *Service) publishCacheStats() {
	entries, bytes := s.CacheDirStats()
	observability.SetHLSCacheStats(entries, bytes)
}

func (s *Service) currentSettings() TranscodeSettings {
	if s == nil || s.settings == nil {
		return DefaultTranscodeSettings()
	}

	return s.settings()
}

func (s *Service) workerContext() (context.Context, context.CancelFunc) {
	return s.detachedContext(context.Background())
}

// detachedContext cancels when the service stops, but not when parent is canceled (HTTP request end).
func (s *Service) detachedContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))

	go func() {
		select {
		case <-s.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func (s *Service) finishJob(job *Job) {
	s.mu.Lock()
	delete(s.jobs, job.cacheKey)
	s.publishJobGaugesLocked()
	s.mu.Unlock()

	if job.Failed {
		writeTranscodeError(job.outDir, job.ErrorMsg)

		return
	}

	markTranscodeComplete(job.outDir)
	s.recordPackageSize(job.cacheKey)
	s.touchCache(job.cacheKey)
	s.publishCacheStats()
}

// publishJobGaugesLocked refreshes active job/session Prometheus gauges. Caller holds s.mu.
func (s *Service) publishJobGaugesLocked() {
	sessionCount := 0
	for _, byVariant := range s.sessions {
		sessionCount += len(byVariant)
	}

	observability.SetTranscodeJobsActive(len(s.jobs))
	observability.SetTranscodeSegmentSessionsActive(sessionCount)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}

func isTranscodeComplete(outDir string) bool {
	_, err := os.Stat(filepath.Join(outDir, completeMarker))

	return err == nil
}

func readTranscodeError(outDir string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(outDir, errorMarker))
	if err != nil {
		return "", false
	}

	msg := strings.TrimSpace(string(raw))
	if msg == "" {
		return "transcode failed", true
	}

	return msg, true
}

func writeTranscodeError(outDir, message string) {
	//nolint:mnd // outDir is under the transcode cache root
	_ = os.WriteFile(filepath.Join(outDir, errorMarker), []byte(message), 0o600)
	_ = os.Remove(filepath.Join(outDir, completeMarker))
	_ = os.Remove(filepath.Join(outDir, masterPlaylistName))
}

func markTranscodeComplete(outDir string) {
	//nolint:mnd // outDir is under the transcode cache root
	_ = os.WriteFile(filepath.Join(outDir, completeMarker), []byte("ok"), 0o600)
	_ = os.Remove(filepath.Join(outDir, errorMarker))
}

func (s *Service) purgePackageIfUnused(
	entry os.DirEntry,
	now time.Time,
	unusedFor time.Duration,
	inUse map[string]struct{},
) (bool, int64) {
	if !entry.IsDir() {
		return false, 0
	}

	cacheKey := entry.Name()
	if _, busy := inUse[cacheKey]; busy {
		return false, 0
	}

	info, err := entry.Info()
	if err != nil {
		return false, 0
	}
	lastUsed := s.lastUsedOf(cacheKey, info.ModTime())
	if now.Sub(lastUsed) < unusedFor {
		return false, 0
	}

	s.mu.Lock()
	size := s.packageSizeLocked(cacheKey)
	s.mu.Unlock()
	if size == 0 {
		size = dirSize(filepath.Join(s.cacheDir, cacheKey))
	}

	slog.Debug("purging unused hls cache",
		slog.String("action", "playback.cache.purge"),
		slog.String("cache_key", cacheKey),
		slog.Duration("unused_for", now.Sub(lastUsed)),
	)
	s.RemoveCache(cacheKey)

	return true, size
}
