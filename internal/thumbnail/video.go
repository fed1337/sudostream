// Package thumbnail handles generating and caching thumbnail images.
package thumbnail

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
)

const (
	maxPosterWidth  = 640
	maxPosterHeight = 360
)

// ErrThumbnailEmpty is returned when thumbnail generation produces an empty file.
var ErrThumbnailEmpty = errors.New("failed to generate thumbnail: output empty")

// ErrNotCached is returned when a poster has not been generated yet.
var ErrNotCached = errors.New("video thumbnail not cached")

// VideoThumbnailer generates poster thumbnails for video files using ffmpeg.
type VideoThumbnailer struct {
	cacheDir string
}

// NewVideoThumbnailer initializes a new VideoThumbnailer with the given cache directory.
func NewVideoThumbnailer(cacheDir string) (*VideoThumbnailer, error) {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "sudostream", "thumbs")
	}
	//nolint:mnd // permissions
	err := os.MkdirAll(cacheDir, 0o750)
	if err != nil {
		return nil, fmt.Errorf("create thumbnail cache dir: %w", err)
	}

	return &VideoThumbnailer{cacheDir: cacheDir}, nil
}

// CacheKey returns the cache filename key for a media file.
// Paths are normalized (leading slash stripped, slash-separated) so warm and
// HTTP handlers share the same key regardless of Gin *path prefix style.
func CacheKey(mediaPath string, mtime int64, size int64) string {
	normalized := filepath.ToSlash(strings.TrimPrefix(mediaPath, "/"))
	hashData := fmt.Sprintf("%s:%d:%d", normalized, mtime, size)
	hash := sha256.Sum256([]byte(hashData))

	return hex.EncodeToString(hash[:])
}

// CachePath returns the on-disk path for a cached poster.
func (t *VideoThumbnailer) CachePath(mediaPath string, mtime int64, size int64) string {
	return filepath.Join(t.cacheDir, CacheKey(mediaPath, mtime, size)+".webp")
}

// OpenCached returns a cached poster path or ErrNotCached.
func (t *VideoThumbnailer) OpenCached(mediaPath string, mtime int64, size int64) (string, error) {
	if t == nil {
		return "", ErrNotCached
	}

	cachePath := t.CachePath(mediaPath, mtime, size)
	info, err := os.Stat(cachePath)
	if err != nil || info.Size() == 0 {
		return "", ErrNotCached
	}

	return cachePath, nil
}

// Generate creates and caches a poster thumbnail.
func (t *VideoThumbnailer) Generate(
	ctx context.Context,
	mediaPath string,
	mtime int64,
	size int64,
) (string, error) {
	if t == nil {
		return "", ErrNotCached
	}

	cached, openErr := t.OpenCached(mediaPath, mtime, size)
	if openErr == nil {
		return cached, nil
	}

	return t.generatePoster(ctx, mediaPath, mediaPath, mtime, size)
}

// Remove deletes the cached poster for a media file, if present.
func (t *VideoThumbnailer) Remove(mediaPath string, mtime int64, size int64) {
	cachePath := t.CachePath(mediaPath, mtime, size)

	_ = os.Remove(cachePath)
}

// WarmLibrary generates missing posters for all videos under libraryRelPath.
// Returns the number of posters newly generated.
func (t *VideoThumbnailer) WarmLibrary(
	ctx context.Context,
	media *mediafs.Service,
	libraryRelPath string,
) (int, error) {
	if t == nil || media == nil || libraryRelPath == "" {
		return 0, nil
	}

	return t.warmLibrary(ctx, media, libraryRelPath)
}

// WarmLibraryAsync generates missing posters for all videos under libraryRelPath.
func (t *VideoThumbnailer) WarmLibraryAsync(
	ctx context.Context,
	media *mediafs.Service,
	libraryRelPath string,
) {
	if t == nil || media == nil || libraryRelPath == "" {
		return
	}

	go func() {
		_, _ = t.warmLibrary(context.WithoutCancel(ctx), media, libraryRelPath)
	}()
}

func (t *VideoThumbnailer) generatePoster(
	ctx context.Context,
	cacheKeyPath, ffmpegInputPath string,
	mtime, size int64,
) (string, error) {
	cachePath := t.CachePath(cacheKeyPath, mtime, size)

	tempFile, err := os.CreateTemp(t.cacheDir, "poster-*.part.webp")
	if err != nil {
		return "", fmt.Errorf("create temp thumbnail: %w", err)
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()
	defer func() { _ = os.Remove(tempPath) }()

	err = generatePosterWebP(ctx, ffmpegInputPath, tempPath)
	if err != nil {
		observability.RecordVideoPosterGeneration("error")

		return "", err
	}

	stat, statErr := os.Stat(tempPath)
	if statErr != nil || stat.Size() == 0 {
		observability.RecordVideoPosterGeneration("error")

		return "", ErrThumbnailEmpty
	}

	renameErr := os.Rename(tempPath, cachePath)
	if renameErr != nil {
		cached, openErr := t.OpenCached(cacheKeyPath, mtime, size)
		if openErr == nil {
			return cached, nil
		}

		observability.RecordVideoPosterGeneration("error")

		return "", fmt.Errorf("rename temp thumbnail: %w", renameErr)
	}

	observability.RecordVideoPosterGeneration("success")

	return cachePath, nil
}

func (t *VideoThumbnailer) warmLibrary(
	ctx context.Context,
	media *mediafs.Service,
	libraryRelPath string,
) (int, error) {
	rootPath, err := media.DirPath(libraryRelPath)
	if err != nil {
		slog.Warn("poster warmup skipped library",
			slog.String("action", "thumbnail.warm"),
			slog.String("library", libraryRelPath),
			slog.String("error", err.Error()),
		)

		return 0, fmt.Errorf("resolve library path: %w", err)
	}

	var generated int
	walkErr := filepath.WalkDir(rootPath, func(absPath string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		made, entryErr := t.warmLibraryEntry(ctx, media, rootPath, absPath, entry, err)
		if made {
			generated++
		}

		return entryErr
	})
	if walkErr != nil {
		slog.Warn("poster warmup walk failed",
			slog.String("action", "thumbnail.warm"),
			slog.String("library", libraryRelPath),
			slog.String("error", walkErr.Error()),
		)

		return generated, fmt.Errorf("warm library walk: %w", walkErr)
	}

	return generated, nil
}

func (t *VideoThumbnailer) warmLibraryEntry(
	ctx context.Context,
	media *mediafs.Service,
	rootPath, absPath string,
	entry fs.DirEntry,
	walkErr error,
) (bool, error) {
	if walkErr != nil {
		return false, walkErr
	}

	if entry.IsDir() {
		if strings.HasPrefix(entry.Name(), ".") && absPath != rootPath {
			return false, filepath.SkipDir
		}

		return false, nil
	}

	if !isVideoMediaFile(absPath, entry.Name()) {
		return false, nil
	}

	mediaPath, relErr := mediaRelPath(media, absPath)
	if relErr != nil {
		slog.Warn("poster warmup skipped file",
			slog.String("action", "thumbnail.warm"),
			slog.String("path", absPath),
			slog.String("error", relErr.Error()),
		)

		return false, nil
	}

	info, infoErr := entry.Info()
	if infoErr != nil {
		return false, nil //nolint:nilerr // skip unreadable entries
	}

	if t.posterCached(mediaPath, info.ModTime().Unix(), info.Size()) {
		return false, nil
	}

	slog.Debug("poster warmup generating",
		slog.String("action", "thumbnail.warm"),
		slog.String("path", mediaPath),
	)

	_, genErr := t.generatePoster(ctx, mediaPath, absPath, info.ModTime().Unix(), info.Size())
	if genErr != nil {
		slog.Warn("poster generation failed",
			slog.String("action", "thumbnail.warm"),
			slog.String("path", mediaPath),
			slog.String("error", genErr.Error()),
		)

		return false, nil
	}

	return true, nil
}

func mediaRelPath(media *mediafs.Service, absPath string) (string, error) {
	rel, err := filepath.Rel(media.Root(), absPath)
	if err != nil {
		return "", fmt.Errorf("rel path: %w", err)
	}

	return filepath.ToSlash(rel), nil
}

func (t *VideoThumbnailer) posterCached(mediaPath string, mtime, size int64) bool {
	_, err := t.OpenCached(mediaPath, mtime, size)

	return err == nil
}

func isVideoMediaFile(absPath, name string) bool {
	if mediafs.IsVideoExtension(filepath.Ext(name)) {
		return true
	}

	mimeType := mediafs.DetectMimeType(absPath, filepath.Ext(name), false)

	return strings.HasPrefix(mimeType, "video/")
}

func generatePosterWebP(ctx context.Context, mediaPath, tempPath string) error {
	streamIndex, hasArt := embeddedCoverStream(ctx, mediaPath)
	if hasArt {
		err := extractEmbeddedArtWebP(ctx, mediaPath, tempPath, streamIndex)
		if err == nil {
			return nil
		}
	}

	return extractFrameWebP(ctx, mediaPath, tempPath)
}

type ffprobeStreams struct {
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeStream struct {
	CodecType   string            `json:"codec_type"` //nolint:tagliatelle // ffprobe JSON
	Disposition map[string]int    `json:"disposition"`
	Tags        map[string]string `json:"tags"`
}

func embeddedCoverStream(ctx context.Context, mediaPath string) (int, bool) {
	cmd := exec.CommandContext( //nolint:gosec // mediaPath resolved via mediafs
		ctx,
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "v",
		mediaPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, false
	}

	var parsed ffprobeStreams
	err = json.Unmarshal(output, &parsed)
	if err != nil {
		return 0, false
	}

	for index, stream := range parsed.Streams {
		if stream.Disposition["attached_pic"] == 1 {
			return index, true
		}
	}

	return 0, false
}

func scaleFilter() string {
	return fmt.Sprintf(
		"scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2",
		maxPosterWidth,
		maxPosterHeight,
	)
}

func extractEmbeddedArtWebP(
	ctx context.Context,
	mediaPath, tempPath string,
	streamIndex int,
) error {
	cmd := exec.CommandContext( //nolint:gosec // mediaPath resolved via mediafs
		ctx,
		"ffmpeg",
		"-v", "quiet",
		"-i", mediaPath,
		"-an",
		"-map", fmt.Sprintf("0:v:%d", streamIndex),
		"-frames:v", "1",
		"-vf", scaleFilter(),
		"-f", "webp",
		"-c:v", "libwebp",
		"-lossless", "1",
		"-quality", "100",
		"-y", tempPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("ffmpeg embedded art: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

func extractFrameWebP(ctx context.Context, mediaPath, tempPath string) error {
	err := extractFrameAtWebP(ctx, mediaPath, tempPath, "00:00:05")
	if err == nil && frameCaptureOutputOK(tempPath) {
		return nil
	}

	return extractFrameAtWebP(ctx, mediaPath, tempPath, "00:00:00")
}

func frameCaptureOutputOK(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Size() > 0
}

func extractFrameAtWebP(ctx context.Context, mediaPath, tempPath, seek string) error {
	cmd := exec.CommandContext( //nolint:gosec // mediaPath resolved via mediafs
		ctx,
		"ffmpeg",
		"-v", "error",
		"-i", mediaPath,
		"-an",
		"-ss", seek,
		"-frames:v", "1",
		"-vf", scaleFilter(),
		"-f", "webp",
		"-c:v", "libwebp",
		"-lossless", "1",
		"-quality", "100",
		"-y", tempPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("ffmpeg frame capture: %w, stderr: %s", err, stderr.String())
	}

	return nil
}
