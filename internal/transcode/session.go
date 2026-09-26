package transcode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sudoStream/internal/observability"
	"sync"
	"time"
)

const (
	// segmentGapThreshold is how far ahead of the running encoder a request may land before it
	// is cheaper to restart ffmpeg at the new position than to wait for it to catch up.
	segmentGapThreshold = 4
	segmentWaitTimeout  = 90 * time.Second
	segmentPollInterval = 40 * time.Millisecond
	// sessionIdleTimeout kills encoders nobody is pulling from, so an abandoned tab does not
	// transcode a whole film in the background.
	sessionIdleTimeout = 45 * time.Second
	sessionReapPeriod  = 15 * time.Second
	stderrTailLimit    = 4096

	segmentResultHit     = "hit"
	segmentResultWait    = "wait"
	segmentResultRestart = "restart"
)

// ErrSegmentUnavailable is returned when ffmpeg finished without producing the segment.
var ErrSegmentUnavailable = errors.New("segment unavailable")

// segmentSession is one running ffmpeg process feeding a single rendition directory.
type segmentSession struct {
	variant    Variant
	variantDir string
	startIndex int
	cancel     context.CancelFunc
	done       chan struct{}

	mu         sync.Mutex
	err        error
	stderr     string
	lastAccess time.Time
}

func (ss *segmentSession) finished() bool {
	select {
	case <-ss.done:
		return true
	default:
		return false
	}
}

func (ss *segmentSession) touch() {
	ss.mu.Lock()
	ss.lastAccess = time.Now()
	ss.mu.Unlock()
}

func (ss *segmentSession) idleFor(now time.Time) time.Duration {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	return now.Sub(ss.lastAccess)
}

func (ss *segmentSession) failure() (string, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if ss.err == nil {
		return "", false
	}

	return ss.stderr, true
}

// ResolveSegment returns the on-disk path of a rendition segment, producing it if needed.
//
// Segments are never generated ahead of demand: a request either lands inside the window a
// running encoder is about to write, or it restarts ffmpeg at that exact offset. Both quality
// and audio switches therefore cost one seek, not a re-encode of the remainder.
func (s *Service) ResolveSegment(
	ctx context.Context,
	cacheKey, mediaPath string,
	variant Variant,
	index int,
) (string, error) {
	outDir := s.CacheOutDir(cacheKey)

	meta, ok := ReadSourceMeta(outDir)
	if !ok {
		return "", ErrSourceMetaUnavailable
	}

	if !validVariant(meta, variant) {
		return "", ErrInvalidVariant
	}

	if index < 0 || index >= len(meta.Segments) {
		return "", fmt.Errorf("%w: %d", ErrSegmentOutOfRange, index)
	}

	variantDir := filepath.Join(outDir, variant.Dir())
	segmentPath := filepath.Join(variantDir, SegmentName(index))

	lock := s.resolveLock(cacheKey, variant)
	lock.Lock()
	defer lock.Unlock()

	session := s.session(cacheKey, variant)
	if segmentComplete(variantDir, index, session) {
		observability.RecordHLSSegmentRequest(segmentResultHit)

		if session != nil {
			session.touch()
		}

		return segmentPath, nil
	}

	if needsRestart(session, variantDir, index) {
		observability.RecordHLSSegmentRequest(segmentResultRestart)
		s.stopSession(cacheKey, variant)

		var err error

		session, err = s.startSession(ctx, cacheKey, mediaPath, meta, variant, index)
		if err != nil {
			return "", err
		}
	} else {
		observability.RecordHLSSegmentRequest(segmentResultWait)
	}

	return waitForSegment(ctx, session, variantDir, index, segmentPath)
}

// needsRestart decides between waiting for the live encoder and seeking a new one into place.
func needsRestart(session *segmentSession, variantDir string, index int) bool {
	if session == nil || session.finished() {
		return true
	}

	if index < session.startIndex {
		return true
	}

	highest, ok := highestSegmentIndex(variantDir)
	if !ok {
		return index > session.startIndex+segmentGapThreshold
	}

	return index > highest+segmentGapThreshold
}

// segmentComplete reports whether a segment file is fully written.
//
// The segment muxer keeps the current file open, so a segment only counts as complete once a
// later one exists or the process has exited.
func segmentComplete(variantDir string, index int, session *segmentSession) bool {
	info, err := os.Stat(filepath.Join(variantDir, SegmentName(index)))
	if err != nil || info.Size() == 0 {
		return false
	}

	if session == nil || session.finished() {
		return true
	}

	highest, ok := highestSegmentIndex(variantDir)

	return ok && highest > index
}

func highestSegmentIndex(variantDir string) (int, bool) {
	entries, err := os.ReadDir(variantDir)
	if err != nil {
		return 0, false
	}

	highest := -1

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if index, ok := SegmentIndexFromName(entry.Name()); ok && index > highest {
			highest = index
		}
	}

	return highest, highest >= 0
}

func waitForSegment(
	ctx context.Context,
	session *segmentSession,
	variantDir string,
	index int,
	segmentPath string,
) (string, error) {
	deadline := time.NewTimer(segmentWaitTimeout)
	defer deadline.Stop()

	ticker := time.NewTicker(segmentPollInterval)
	defer ticker.Stop()

	for {
		if segmentComplete(variantDir, index, session) {
			session.touch()

			return segmentPath, nil
		}

		if session.finished() {
			if detail, failed := session.failure(); failed {
				return "", fmt.Errorf("%w: %s", ErrSegmentUnavailable, detail)
			}

			return "", fmt.Errorf("%w: segment %d not produced", ErrSegmentUnavailable, index)
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("wait segment %d: %w", index, ctx.Err())
		case <-deadline.C:
			return "", fmt.Errorf("%w: timeout on segment %d", ErrSegmentUnavailable, index)
		case <-ticker.C:
		}
	}
}

func (s *Service) startSession(
	ctx context.Context,
	cacheKey, mediaPath string,
	meta SourceMeta,
	variant Variant,
	index int,
) (*segmentSession, error) {
	variantDir := filepath.Join(s.CacheOutDir(cacheKey), variant.Dir())

	//nolint:mnd // permissions
	err := os.MkdirAll(variantDir, 0o750)
	if err != nil {
		return nil, fmt.Errorf("create variant dir: %w", err)
	}

	// A restart rewrites from `index` onward; the tail of the previous run may be truncated.
	removeSegmentsFrom(variantDir, index)

	err = ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("start segment session: %w", err)
	}

	// Encoder outlives the HTTP request; cancel only on seek-restart / idle reap / service stop.
	sessionCtx, cancel := s.detachedContext(ctx)

	session := &segmentSession{
		variant:    variant,
		variantDir: variantDir,
		startIndex: index,
		cancel:     cancel,
		done:       make(chan struct{}),
		lastAccess: time.Now(),
	}

	settings := s.currentSettings()
	encoder, render := ResolveVideoEncoder(settings.HwAccel)
	toneMap := ResolveToneMapMode(
		encoder,
		checkGPUVendor(),
		meta.NeedsToneMap() && settings.ToneMappingEnabled,
	)
	args := buildSegmentArgs(segmentRun{
		mediaPath:    mediaPath,
		variantDir:   variantDir,
		meta:         meta,
		variant:      variant,
		startIndex:   index,
		encoder:      encoder,
		render:       render,
		toneMap:      toneMap,
		toneAlgo:     settings.ToneMappingAlgorithm,
		downmix:      settings.DownmixAlgorithm,
		downmixBoost: settings.DownmixBoost,
	})

	cmd := exec.CommandContext(sessionCtx, "ffmpeg", args...)

	stderr := &tailBuffer{limit: stderrTailLimit}
	cmd.Stderr = stderr

	err = cmd.Start()
	if err != nil {
		cancel()

		return nil, fmt.Errorf("start ffmpeg segment session: %w", err)
	}

	s.registerSession(cacheKey, variant, session)
	s.wg.Add(1)

	go s.awaitSession(cacheKey, cmd, session, stderr)

	return session, nil
}

func (s *Service) awaitSession(
	cacheKey string,
	cmd *exec.Cmd,
	session *segmentSession,
	stderr *tailBuffer,
) {
	defer s.wg.Done()

	start := time.Now()
	err := cmd.Wait()

	session.mu.Lock()
	session.err = err
	session.stderr = stderr.String()
	session.mu.Unlock()

	close(session.done)
	session.cancel()

	outcome := jobMetricSuccess

	// A killed session is the normal cost of a seek or switch, not a failure.
	if err != nil && !errors.Is(err, context.Canceled) && cmd.ProcessState.ExitCode() > 0 {
		outcome = jobMetricError

		observability.RecordFFmpegError("segment")
		slog.Warn("segment session failed",
			slog.String("variant", session.variant.Dir()),
			slog.Int("start_segment", session.startIndex),
			slog.String("error", strings.TrimSpace(stderr.String())),
		)
	}

	observability.RecordTranscodeSegmentJob(outcome, time.Since(start))
	s.clearSession(cacheKey, session)
}

func (s *Service) registerSession(cacheKey string, variant Variant, session *segmentSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessions == nil {
		s.sessions = make(map[string]map[string]*segmentSession)
	}

	if s.sessions[cacheKey] == nil {
		s.sessions[cacheKey] = make(map[string]*segmentSession)
	}

	s.sessions[cacheKey][variant.Dir()] = session
	s.publishJobGaugesLocked()
}

func (s *Service) session(cacheKey string, variant Variant) *segmentSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sessions[cacheKey][variant.Dir()]
}

func (s *Service) clearSession(cacheKey string, session *segmentSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessions[cacheKey][session.variant.Dir()] == session {
		delete(s.sessions[cacheKey], session.variant.Dir())
		s.publishJobGaugesLocked()
	}
}

func (s *Service) stopSession(cacheKey string, variant Variant) {
	session := s.session(cacheKey, variant)
	if session == nil {
		return
	}

	session.cancel()
	<-session.done
}

func (s *Service) resolveLock(cacheKey string, variant Variant) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.resolveLocks == nil {
		s.resolveLocks = make(map[string]*sync.Mutex)
	}

	key := cacheKey + "|" + variant.Dir()
	if lock, ok := s.resolveLocks[key]; ok {
		return lock
	}

	lock := &sync.Mutex{}
	s.resolveLocks[key] = lock

	return lock
}

// reapIdleSessions terminates encoders that no client has pulled from recently.
func (s *Service) reapIdleSessions() {
	defer s.wg.Done()

	ticker := time.NewTicker(sessionReapPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			for _, session := range s.idleSessions(now) {
				slog.Debug("reaping idle segment session",
					slog.String("variant", session.variant.Dir()),
				)
				session.cancel()
			}
		}
	}
}

func (s *Service) idleSessions(now time.Time) []*segmentSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	var idle []*segmentSession

	for _, byVariant := range s.sessions {
		for _, session := range byVariant {
			if session.idleFor(now) > sessionIdleTimeout {
				idle = append(idle, session)
			}
		}
	}

	return idle
}

func removeSegmentsFrom(variantDir string, index int) {
	entries, err := os.ReadDir(variantDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if existing, ok := SegmentIndexFromName(entry.Name()); ok && existing >= index {
			_ = os.Remove(filepath.Join(variantDir, entry.Name()))
		}
	}
}

// tailBuffer keeps only the trailing bytes of ffmpeg stderr for error reporting.
type tailBuffer struct {
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}

	return len(p), nil
}

func (b *tailBuffer) String() string {
	return strings.TrimSpace(string(b.data))
}
