package thumbnail

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
)

// ImageThumbnailer caches resized JPEG thumbnails for still images.
type ImageThumbnailer struct {
	cacheDir string
}

// NewImageThumbnailer initializes an ImageThumbnailer with the given cache directory.
func NewImageThumbnailer(cacheDir string) (*ImageThumbnailer, error) {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "sudostream", "thumbs")
	}
	//nolint:mnd // permissions
	err := os.MkdirAll(cacheDir, 0o750)
	if err != nil {
		return nil, fmt.Errorf("create image thumbnail cache dir: %w", err)
	}

	return &ImageThumbnailer{cacheDir: cacheDir}, nil
}

// ImageCacheKey returns the cache filename key for an image thumbnail at a given size.
func ImageCacheKey(mediaPath string, mtime, size int64, width, height int) string {
	return fmt.Sprintf("%s_w%dh%d", CacheKey(mediaPath, mtime, size), width, height)
}

// CachePath returns the on-disk path for a cached image thumbnail.
func (t *ImageThumbnailer) CachePath(mediaPath string, mtime, size int64, width, height int) string {
	return filepath.Join(t.cacheDir, ImageCacheKey(mediaPath, mtime, size, width, height)+".jpg")
}

// OpenCached returns a cached image thumbnail path or ErrNotCached.
func (t *ImageThumbnailer) OpenCached(
	mediaPath string,
	mtime, size int64,
	width, height int,
) (string, error) {
	if t == nil {
		return "", ErrNotCached
	}

	cachePath := t.CachePath(mediaPath, mtime, size, width, height)
	info, err := os.Stat(cachePath)
	if err != nil || info.Size() == 0 {
		return "", ErrNotCached
	}

	return cachePath, nil
}

// OpenOrGenerate returns a cached JPEG thumbnail, generating and caching it on miss.
func (t *ImageThumbnailer) OpenOrGenerate(
	filePath, mediaPath string,
	mtime, size int64,
	width, height int,
) (string, error) {
	if t == nil {
		return "", ErrNotCached
	}

	cached, err := t.OpenCached(mediaPath, mtime, size, width, height)
	if err == nil {
		return cached, nil
	}

	return t.generate(filePath, mediaPath, mtime, size, width, height)
}

func (t *ImageThumbnailer) generate(
	filePath, mediaPath string,
	mtime, size int64,
	width, height int,
) (string, error) {
	cachePath := t.CachePath(mediaPath, mtime, size, width, height)

	img, err := imaging.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open image: %w", err)
	}

	thumb := imaging.Thumbnail(img, width, height, imaging.Lanczos)

	tempFile, err := os.CreateTemp(t.cacheDir, "image-*.part.jpg")
	if err != nil {
		return "", fmt.Errorf("create temp image thumbnail: %w", err)
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()
	defer func() { _ = os.Remove(tempPath) }()

	const jpegQuality = 85
	err = imaging.Save(thumb, tempPath, imaging.JPEGQuality(jpegQuality))
	if err != nil {
		return "", fmt.Errorf("encode image thumbnail: %w", err)
	}

	renameErr := os.Rename(tempPath, cachePath)
	if renameErr != nil {
		cached, openErr := t.OpenCached(mediaPath, mtime, size, width, height)
		if openErr == nil {
			return cached, nil
		}

		return "", fmt.Errorf("rename image thumbnail: %w", renameErr)
	}

	return cachePath, nil
}
