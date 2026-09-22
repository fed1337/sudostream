package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DefaultCacheRoot is where provider artifacts live on disk (FI-1 L7). Media files under
// /media are never touched by poster or subtitle tasks.
const DefaultCacheRoot = "/var/lib/sudostream/cache/providers"

const (
	cacheDirPerm      = 0o750
	cacheFilePerm     = 0o600
	cacheNameHexChars = 32
)

// ErrCachePathOutsideRoot is returned when a cache path escapes the provider cache root.
var ErrCachePathOutsideRoot = errors.New("provider cache path outside root")

// Cache stores provider-fetched files under a single root directory. Cache paths recorded in
// provider_artifacts are always relative to that root, so the root can move between deploys.
type Cache struct {
	root string
}

// NewCache creates the cache root if needed and returns a writer for it.
func NewCache(root string) (*Cache, error) {
	if strings.TrimSpace(root) == "" {
		root = DefaultCacheRoot
	}

	err := os.MkdirAll(root, cacheDirPerm)
	if err != nil {
		return nil, fmt.Errorf("create provider cache dir: %w", err)
	}

	return &Cache{root: root}, nil
}

// Root returns the absolute cache root.
func (c *Cache) Root() string {
	if c == nil {
		return ""
	}

	return c.root
}

// Path resolves a stored cache path to an absolute path, rejecting anything that would escape
// the cache root (defense in depth: cache paths come from the database).
func (c *Cache) Path(cachePath string) (string, error) {
	if c == nil {
		return "", ErrCachePathOutsideRoot
	}

	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(cachePath, "/")))
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return "", ErrCachePathOutsideRoot
	}

	absPath := filepath.Join(c.root, cleaned)
	rel, err := filepath.Rel(c.root, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrCachePathOutsideRoot
	}

	return absPath, nil
}

// Write stores data at cachePath, replacing any previous content atomically.
func (c *Cache) Write(cachePath string, data []byte) error {
	absPath, err := c.Path(cachePath)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(absPath), cacheDirPerm)
	if err != nil {
		return fmt.Errorf("create provider cache subdir: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(absPath), ".part-*")
	if err != nil {
		return fmt.Errorf("create provider cache temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	_, err = tempFile.Write(data)
	closeErr := tempFile.Close()
	if err != nil {
		return fmt.Errorf("write provider cache temp file: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close provider cache temp file: %w", closeErr)
	}

	err = os.Chmod(tempPath, cacheFilePerm)
	if err != nil {
		return fmt.Errorf("chmod provider cache temp file: %w", err)
	}

	err = os.Rename(tempPath, absPath)
	if err != nil {
		return fmt.Errorf("publish provider cache file: %w", err)
	}

	return nil
}

// Remove deletes a cached file. A missing file is not an error.
func (c *Cache) Remove(cachePath string) error {
	absPath, err := c.Path(cachePath)
	if err != nil {
		return err
	}

	err = os.Remove(absPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove provider cache file: %w", err)
	}

	return nil
}

// PosterCachePath is the cache-relative path for a poster artifact. The filename is derived
// from the media path so a re-fetch overwrites in place instead of orphaning the old file.
func PosterCachePath(libraryID, relPath, contentType string) string {
	return path.Join(
		"posters",
		libraryID,
		cacheFileName(relPath)+posterExtension(contentType),
	)
}

// SubtitleCachePath is the cache-relative path for a subtitle artifact (E3).
func SubtitleCachePath(libraryID, relPath, lang string) string {
	return path.Join("subtitles", libraryID, cacheFileName(relPath), lang+".vtt")
}

func cacheFileName(relPath string) string {
	sum := sha256.Sum256([]byte(strings.TrimPrefix(filepath.ToSlash(relPath), "/")))

	return hex.EncodeToString(sum[:])[:cacheNameHexChars]
}

func posterExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/webp":
		return ".webp"
	case "image/png":
		return ".png"
	case "image/avif":
		return ".avif"
	default:
		return ".jpg"
	}
}

// PosterContentType maps a cached poster filename back to its media type for HTTP responses.
func PosterContentType(cachePath string) string {
	switch strings.ToLower(path.Ext(cachePath)) {
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".avif":
		return "image/avif"
	default:
		return "image/jpeg"
	}
}
