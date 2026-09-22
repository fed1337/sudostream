package thumbnail_test

import (
	"context"
	"errors"
	"os"
	"sudoStream/internal/thumbnail"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestCacheKey_IsStable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cache key is deterministic for path mtime size", func(a *allure.Context) {
		t := a.T()
		keyA := thumbnail.CacheKey("/media/a.mp4", 100, 200)
		keyB := thumbnail.CacheKey("/media/a.mp4", 100, 200)
		if keyA != keyB {
			t.Fatalf("keys differ: %q vs %q", keyA, keyB)
		}

		keyC := thumbnail.CacheKey("/media/a.mp4", 101, 200)
		if keyA == keyC {
			t.Fatal("expected different key after mtime change")
		}
	})
}

func TestCacheKey_NormalizesLeadingSlash(t *testing.T) {
	t.Parallel()

	allure.Test(t, "leading slash does not change cache key", func(a *allure.Context) {
		t := a.T()
		withSlash := thumbnail.CacheKey("/movies/a.mkv", 100, 200)
		withoutSlash := thumbnail.CacheKey("movies/a.mkv", 100, 200)
		if withSlash != withoutSlash {
			t.Fatalf("keys differ: %q vs %q", withSlash, withoutSlash)
		}
	})
}

func TestOpenCached_MissingReturnsErrNotCached(t *testing.T) {
	t.Parallel()

	allure.Test(t, "missing cached poster returns ErrNotCached", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		thumb, err := thumbnail.NewVideoThumbnailer(root)
		if err != nil {
			t.Fatalf("new thumbnailer: %v", err)
		}

		_, err = thumb.OpenCached("/media/missing.webm", 1, 2)
		if !errors.Is(err, thumbnail.ErrNotCached) {
			t.Fatalf("expected ErrNotCached, got %v", err)
		}
	})
}

func TestCachePath_AndRemove(t *testing.T) {
	t.Parallel()

	allure.Test(t, "CachePath and Remove manage cached poster file", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		thumb, err := thumbnail.NewVideoThumbnailer(root)
		if err != nil {
			t.Fatalf("new thumbnailer: %v", err)
		}

		mediaPath := "/media/a.mp4"
		cachePath := thumb.CachePath(mediaPath, 100, 200)
		err = os.WriteFile(cachePath, []byte("webp"), 0o600)
		if err != nil {
			t.Fatalf("write cache: %v", err)
		}

		got, openErr := thumb.OpenCached(mediaPath, 100, 200)
		if openErr != nil {
			t.Fatalf("open cached: %v", openErr)
		}
		if got != cachePath {
			t.Fatalf("cache path: got %q want %q", got, cachePath)
		}

		thumb.Remove(mediaPath, 100, 200)
		_, openErr = thumb.OpenCached(mediaPath, 100, 200)
		if !errors.Is(openErr, thumbnail.ErrNotCached) {
			t.Fatalf("expected ErrNotCached after remove, got %v", openErr)
		}
	})
}

func TestGenerate_ReturnsCachedPosterWithoutFFmpeg(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Generate short-circuits when poster already cached", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		thumb, err := thumbnail.NewVideoThumbnailer(root)
		if err != nil {
			t.Fatalf("new thumbnailer: %v", err)
		}

		mediaPath := "/media/cached.mp4"
		cachePath := thumb.CachePath(mediaPath, 10, 20)
		err = os.WriteFile(cachePath, []byte("cached-webp"), 0o600)
		if err != nil {
			t.Fatalf("write cache: %v", err)
		}

		got, genErr := thumb.Generate(context.Background(), mediaPath, 10, 20)
		if genErr != nil {
			t.Fatalf("generate: %v", genErr)
		}
		if got != cachePath {
			t.Fatalf("got %q want %q", got, cachePath)
		}
	})
}

func TestWarmLibraryAsync_NilReceiverIsNoOp(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil thumbnailer warmup is a no-op", func(_ *allure.Context) {
		var thumb *thumbnail.VideoThumbnailer
		thumb.WarmLibraryAsync(context.Background(), nil, "movies")
	})
}
