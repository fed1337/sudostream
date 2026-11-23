package thumbnail

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/disintegration/imaging"
)

func TestImageCacheKey_IncludesDimensions(t *testing.T) {
	t.Parallel()

	allure.Test(t, "image cache key includes width and height", func(a *allure.Context) {
		t := a.T()
		base := CacheKey("photos/a.jpg", 10, 20)
		got := ImageCacheKey("photos/a.jpg", 10, 20, 64, 48)
		if got != base+"_w64h48" {
			t.Fatalf("got %q want %s_w64h48", got, base)
		}
		if ImageCacheKey("photos/a.jpg", 10, 20, 32, 32) == got {
			t.Fatal("different dims must change cache key")
		}
	})
}

func TestImageThumbnailer_CachesOnGenerate(t *testing.T) {
	t.Parallel()

	allure.Test(t, "OpenOrGenerate writes JPEG cache and reuses it", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		src := filepath.Join(root, "still.png")
		err := imaging.Save(imaging.New(80, 60, color.White), src)
		if err != nil {
			t.Fatalf("save source: %v", err)
		}
		info, err := os.Stat(src)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}

		thumb, err := NewImageThumbnailer(filepath.Join(root, "cache"))
		if err != nil {
			t.Fatalf("new image thumbnailer: %v", err)
		}

		first, err := thumb.OpenOrGenerate(src, "photos/still.png", info.ModTime().Unix(), info.Size(), 40, 30)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		second, err := thumb.OpenOrGenerate(src, "photos/still.png", info.ModTime().Unix(), info.Size(), 40, 30)
		if err != nil {
			t.Fatalf("cache hit: %v", err)
		}
		if first != second {
			t.Fatalf("paths differ: %q vs %q", first, second)
		}
		stat, err := os.Stat(first)
		if err != nil || stat.Size() == 0 {
			t.Fatalf("cached file missing or empty: %v", err)
		}
	})
}
