package mediafs

import (
	"os"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestService_RootFilePathAndHealth(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"Root, FilePath, and Health expose resolved media paths",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			clip := filepath.Join(root, "movies", "clip.mp4")
			err := os.MkdirAll(filepath.Dir(clip), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(clip, []byte("video"), 0o600)
			if err != nil {
				t.Fatalf("write clip: %v", err)
			}

			svc, err := New(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			if svc.Root() != root {
				t.Fatalf("root: got %q want %q", svc.Root(), root)
			}
			err = svc.Health()
			if err != nil {
				t.Fatalf("health: %v", err)
			}

			resolved, err := svc.FilePath("movies/clip.mp4")
			if err != nil {
				t.Fatalf("file path: %v", err)
			}
			if resolved != clip {
				t.Fatalf("file path: got %q want %q", resolved, clip)
			}

			err = os.RemoveAll(root)
			if err != nil {
				t.Fatalf("remove root: %v", err)
			}
			healthErr := svc.Health()
			if healthErr == nil {
				t.Fatal("expected health error after root removal")
			}
		},
	)
}

func TestIsVideoExtension_RecognizesKnownContainers(t *testing.T) {
	t.Parallel()

	allure.Test(t, "known video extensions are recognized", func(a *allure.Context) {
		t := a.T()
		for _, ext := range []string{".mp4", ".MKV", ".webm", ".mov"} {
			if !IsVideoExtension(ext) {
				t.Fatalf("expected %q to be video", ext)
			}
		}
		if IsVideoExtension(".txt") {
			t.Fatal("expected txt to be non-video")
		}
	})
}
