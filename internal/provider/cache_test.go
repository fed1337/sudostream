package provider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const cacheTestLibraryID = "lib-1"

func TestCache_WriteReadRemove(t *testing.T) { //nolint:cyclop // cache lifecycle branches
	t.Parallel()

	allure.Test(t, "cache writes, resolves, and removes artifact files", func(a *allure.Context) {
		t := a.T()
		cache, err := NewCache(filepath.Join(t.TempDir(), "providers"))
		if err != nil {
			t.Fatalf("new cache: %v", err)
		}

		cachePath := PosterCachePath(cacheTestLibraryID, "films/Movie (2020).mkv", contentTypeWebP)
		if !strings.HasPrefix(cachePath, "posters/lib-1/") ||
			!strings.HasSuffix(cachePath, ".webp") {
			t.Fatalf("poster cache path: %s", cachePath)
		}

		err = cache.Write(cachePath, []byte("poster"))
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		absPath, err := cache.Path(cachePath)
		if err != nil {
			t.Fatalf("path: %v", err)
		}
		content, err := os.ReadFile(absPath) //nolint:gosec // path resolved through Cache
		if err != nil || string(content) != "poster" {
			t.Fatalf("read back: %q %v", content, err)
		}

		// Re-writing the same media path overwrites in place instead of orphaning a file.
		err = cache.Write(cachePath, []byte("poster-v2"))
		if err != nil {
			t.Fatalf("rewrite: %v", err)
		}
		entries, err := os.ReadDir(filepath.Dir(absPath))
		if err != nil || len(entries) != 1 {
			t.Fatalf("want single cache file, got %d (%v)", len(entries), err)
		}

		err = cache.Remove(cachePath)
		if err != nil {
			t.Fatalf("remove: %v", err)
		}
		err = cache.Remove(cachePath)
		if err != nil {
			t.Fatalf("remove missing file must be a no-op, got %v", err)
		}
	})
}

func TestCache_RejectsEscapingPaths(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cache paths from the DB cannot escape the cache root", func(a *allure.Context) {
		t := a.T()
		root := filepath.Join(t.TempDir(), "providers")
		cache, err := NewCache(root)
		if err != nil {
			t.Fatalf("new cache: %v", err)
		}

		for _, bad := range []string{"../escape.webp", "posters/../../escape.webp", "/", ""} {
			_, err = cache.Path(bad)
			if !errors.Is(err, ErrCachePathOutsideRoot) {
				t.Fatalf("%q: want ErrCachePathOutsideRoot, got %v", bad, err)
			}
		}

		var nilCache *Cache
		_, pathErr := nilCache.Path("posters/x.webp")
		if !errors.Is(pathErr, ErrCachePathOutsideRoot) {
			t.Fatalf("nil cache: %v", pathErr)
		}
		if nilCache.Root() != "" {
			t.Fatal("nil cache root must be empty")
		}
	})
}

func TestCachePaths_MapContentTypes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "poster extension and content type round-trip", func(a *allure.Context) {
		t := a.T()

		cases := map[string]string{
			contentTypeWebP:                 contentTypeWebP,
			"image/png":                     "image/png",
			"image/avif":                    "image/avif",
			contentTypeJPEG + "; charset=x": contentTypeJPEG,
			"application/octet-strea":       contentTypeJPEG,
		}
		for contentType, want := range cases {
			cachePath := PosterCachePath("lib", "a.mkv", contentType)
			if got := PosterContentType(cachePath); got != want {
				t.Fatalf("%s: want %s, got %s", contentType, want, got)
			}
		}

		subtitlePath := SubtitleCachePath("lib", "shows/ep.mkv", "en")
		if !strings.HasPrefix(subtitlePath, "subtitles/lib/") ||
			!strings.HasSuffix(subtitlePath, "/en.vtt") {
			t.Fatalf("subtitle cache path: %s", subtitlePath)
		}
	})
}
