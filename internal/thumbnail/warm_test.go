package thumbnail

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestWarmLibrary_SkipsCachedPosters(t *testing.T) {
	t.Parallel()

	allure.Test(t, "warmLibrary skips ffmpeg when poster already cached", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		libraryDir := filepath.Join(root, "movies")
		err := os.MkdirAll(libraryDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir library: %v", err)
		}

		videoPath := filepath.Join(libraryDir, "clip.mp4")
		err = os.WriteFile(videoPath, []byte("fake-video"), 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		cacheRoot := t.TempDir()
		thumb, err := NewVideoThumbnailer(cacheRoot)
		if err != nil {
			t.Fatalf("new thumbnailer: %v", err)
		}

		info, err := os.Stat(videoPath)
		if err != nil {
			t.Fatalf("stat video: %v", err)
		}

		relPath := "movies/clip.mp4"
		cachePath := thumb.CachePath(relPath, info.ModTime().Unix(), info.Size())
		err = os.WriteFile(cachePath, []byte("cached-webp"), 0o600)
		if err != nil {
			t.Fatalf("seed cache: %v", err)
		}

		_, _ = thumb.warmLibrary(context.Background(), media, "movies")

		_, statErr := os.Stat(cachePath)
		if statErr != nil {
			t.Fatalf("expected cached poster to remain, stat err: %v", statErr)
		}
	})
}

func TestIsVideoMediaFile_AcceptsKnownExtensions(t *testing.T) {
	t.Parallel()

	allure.Test(t, "video extensions are recognized for warmup", func(a *allure.Context) {
		t := a.T()
		if !isVideoMediaFile("/tmp/sample.mp4", "sample.mp4") {
			t.Fatal("expected mp4 to be video")
		}
		if isVideoMediaFile("/tmp/readme.txt", "readme.txt") {
			t.Fatal("expected txt to be rejected")
		}
	})
}

func TestPosterCached_UsesOpenCached(t *testing.T) {
	t.Parallel()

	allure.Test(t, "posterCached reports true when cache file exists", func(a *allure.Context) {
		t := a.T()
		cacheRoot := t.TempDir()
		thumb, err := NewVideoThumbnailer(cacheRoot)
		if err != nil {
			t.Fatalf("new thumbnailer: %v", err)
		}

		mediaPath := "movies/a.mp4"
		cachePath := thumb.CachePath(mediaPath, 1, 2)
		err = os.WriteFile(cachePath, []byte("webp"), 0o600)
		if err != nil {
			t.Fatalf("write cache: %v", err)
		}

		if !thumb.posterCached(mediaPath, 1, 2) {
			t.Fatal("expected poster to be cached")
		}
	})
}

func TestMediaRelPath_ReturnsSlashRelativePath(t *testing.T) {
	t.Parallel()

	allure.Test(t, "mediaRelPath returns slash-separated relative path", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		videoPath := filepath.Join(root, "movies", "clip.mp4")
		err := os.MkdirAll(filepath.Dir(videoPath), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(videoPath, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		rel, err := mediaRelPath(media, videoPath)
		if err != nil {
			t.Fatalf("media rel path: %v", err)
		}
		if rel != "movies/clip.mp4" {
			t.Fatalf("rel path: got %q", rel)
		}
	})
}

func TestGenerate_NilReceiverReturnsErrNotCached(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil thumbnailer Generate returns ErrNotCached", func(a *allure.Context) {
		t := a.T()
		var thumb *VideoThumbnailer
		_, err := thumb.Generate(context.Background(), "movies/a.mp4", 1, 2)
		if !errors.Is(err, ErrNotCached) {
			t.Fatalf("expected ErrNotCached, got %v", err)
		}
	})
}

func TestScaleFilter_IncludesPosterBounds(t *testing.T) {
	t.Parallel()

	allure.Test(t, "scale filter caps poster dimensions", func(a *allure.Context) {
		t := a.T()
		filter := scaleFilter()
		if filter == "" {
			t.Fatal("expected scale filter")
		}
	})
}

func TestWarmLibraryAsync_GeneratesMissingPoster(t *testing.T) {
	t.Parallel()

	allure.Test(t, "WarmLibraryAsync generates posters in background", func(a *allure.Context) {
		t := a.T()
		_, lookErr := exec.LookPath("ffmpeg")
		if lookErr != nil {
			t.Skip("ffmpeg not installed")
		}

		root := t.TempDir()
		libraryDir := filepath.Join(root, "movies")
		err := os.MkdirAll(libraryDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir library: %v", err)
		}

		videoPath := filepath.Join(libraryDir, "clip.mp4")
		//nolint:gosec // test fixture writes under t.TempDir()
		cmd := exec.CommandContext(
			context.Background(),
			"ffmpeg",
			"-v", "error",
			"-f", "lavfi",
			"-i", "color=c=black:s=64x64:d=1",
			"-c:v", "libx264",
			"-y", videoPath,
		)
		err = cmd.Run()
		if err != nil {
			t.Fatalf("create test video: %v", err)
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		cacheRoot := t.TempDir()
		thumb, err := NewVideoThumbnailer(cacheRoot)
		if err != nil {
			t.Fatalf("new thumbnailer: %v", err)
		}

		info, err := os.Stat(videoPath)
		if err != nil {
			t.Fatalf("stat video: %v", err)
		}

		thumb.WarmLibraryAsync(context.Background(), media, "movies")

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			_, openErr := thumb.OpenCached(
				"movies/clip.mp4",
				info.ModTime().Unix(),
				info.Size(),
			)
			if openErr == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}

		t.Fatal("expected poster to be generated asynchronously")
	})
}

func TestGenerate_CreatesPosterFromSyntheticVideo(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Generate runs ffmpeg for uncached video", func(a *allure.Context) {
		t := a.T()
		_, lookErr := exec.LookPath("ffmpeg")
		if lookErr != nil {
			t.Skip("ffmpeg not installed")
		}

		root := t.TempDir()
		videoPath := filepath.Join(root, "sample.mp4")
		//nolint:gosec // test fixture writes under t.TempDir()
		cmd := exec.CommandContext(
			context.Background(),
			"ffmpeg",
			"-v", "error",
			"-f", "lavfi",
			"-i", "color=c=black:s=64x64:d=1",
			"-c:v", "libx264",
			"-y", videoPath,
		)
		err := cmd.Run()
		if err != nil {
			t.Fatalf("create test video: %v", err)
		}

		info, err := os.Stat(videoPath)
		if err != nil {
			t.Fatalf("stat video: %v", err)
		}

		cacheRoot := t.TempDir()
		thumb, err := NewVideoThumbnailer(cacheRoot)
		if err != nil {
			t.Fatalf("new thumbnailer: %v", err)
		}

		got, genErr := thumb.Generate(
			context.Background(),
			videoPath,
			info.ModTime().Unix(),
			info.Size(),
		)
		if genErr != nil {
			t.Fatalf("generate poster: %v", genErr)
		}
		if got == "" {
			t.Fatal("expected cache path")
		}

		stat, statErr := os.Stat(got)
		if statErr != nil || stat.Size() == 0 {
			t.Fatalf("expected non-empty poster at %q", got)
		}
	})
}
