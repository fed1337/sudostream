package thumbnail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

var errDirEntryStubNoInfo = errors.New("no info in stub")

func TestNewVideoThumbnailer_UsesTempDirWhenEmpty(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"empty cache dir defaults to temp sudostream thumbs path",
		func(a *allure.Context) {
			t := a.T()
			thumb, err := NewVideoThumbnailer("")
			if err != nil {
				t.Fatalf("new thumbnailer: %v", err)
			}
			if thumb == nil {
				t.Fatal("expected thumbnailer")
			}
		},
	)
}

func TestOpenCached_NilReceiverReturnsErrNotCached(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil thumbnailer OpenCached returns ErrNotCached", func(a *allure.Context) {
		t := a.T()
		var thumb *VideoThumbnailer
		_, err := thumb.OpenCached("movies/a.mp4", 1, 2)
		if !errors.Is(err, ErrNotCached) {
			t.Fatalf("expected ErrNotCached, got %v", err)
		}
	})
}

func TestWarmLibrary_SkipsInvalidLibraryPath(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"warmLibrary logs and returns when library path is invalid",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			media, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("media service: %v", err)
			}

			cacheRoot := t.TempDir()
			thumb, err := NewVideoThumbnailer(cacheRoot)
			if err != nil {
				t.Fatalf("new thumbnailer: %v", err)
			}

			_, _ = thumb.warmLibrary(context.Background(), media, "missing-library")
		},
	)
}

func TestWarmLibraryEntry_SkipsHiddenDirectories(t *testing.T) {
	t.Parallel()

	allure.Test(t, "warmLibraryEntry skips dot directories", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		hiddenDir := filepath.Join(root, "movies", ".hidden")
		err := os.MkdirAll(hiddenDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir hidden: %v", err)
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

		_, err = thumb.warmLibraryEntry(
			context.Background(),
			media,
			filepath.Join(root, "movies"),
			hiddenDir,
			dirEntryStub{name: ".hidden", dir: true},
			nil,
		)
		if !errors.Is(err, filepath.SkipDir) {
			t.Fatalf("expected SkipDir, got %v", err)
		}
	})
}

func TestEmbeddedCoverStream_InvalidPathReturnsFalse(t *testing.T) {
	t.Parallel()

	allure.Test(t, "embeddedCoverStream returns false for missing media", func(a *allure.Context) {
		t := a.T()
		_, ok := embeddedCoverStream(
			context.Background(),
			filepath.Join(t.TempDir(), "missing.mp4"),
		)
		if ok {
			t.Fatal("expected no embedded cover for missing file")
		}
	})
}

type dirEntryStub struct {
	name string
	dir  bool
}

func (d dirEntryStub) Name() string               { return d.name }
func (d dirEntryStub) IsDir() bool                { return d.dir }
func (d dirEntryStub) Type() os.FileMode          { return 0 }
func (d dirEntryStub) Info() (os.FileInfo, error) { return nil, errDirEntryStubNoInfo }
