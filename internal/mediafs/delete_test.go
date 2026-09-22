package mediafs_test

import (
	"errors"
	"os"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestService_DeleteFile(t *testing.T) {
	t.Parallel()

	allure.Test(t, "delete removes a file under media root", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		target := filepath.Join(root, "clip.mp4")
		err = os.WriteFile(target, []byte("video"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		err = service.Delete("clip.mp4", false)
		if err != nil {
			t.Fatalf("delete: %v", err)
		}

		_, statErr := os.Stat(target)
		if !os.IsNotExist(statErr) {
			t.Fatal("expected file removed")
		}
	})
}

func TestService_DeleteNonEmptyDirRequiresRecursive(t *testing.T) {
	t.Parallel()

	allure.Test(t, "non-empty directory delete without recursive fails", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		dir := filepath.Join(root, "folder")
		err = os.MkdirAll(dir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(dir, "child.txt"), []byte("x"), 0o600)
		if err != nil {
			t.Fatalf("write child: %v", err)
		}

		err = service.Delete("folder", false)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestService_DeleteRecursiveRemovesDirectoryTree(t *testing.T) {
	t.Parallel()

	allure.Test(t, "recursive delete removes non-empty directories", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		dir := filepath.Join(root, "series", "season-1")
		err = os.MkdirAll(dir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(dir, "episode.mkv"), []byte("video"), 0o600)
		if err != nil {
			t.Fatalf("write episode: %v", err)
		}

		err = service.Delete("series", true)
		if err != nil {
			t.Fatalf("delete: %v", err)
		}

		_, statErr := os.Stat(filepath.Join(root, "series"))
		if !os.IsNotExist(statErr) {
			t.Fatal("expected series folder removed")
		}
	})
}

func TestService_DeleteEmptyDirectory(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"empty directory delete succeeds without recursive flag",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			service, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			dir := filepath.Join(root, "empty")
			err = os.MkdirAll(dir, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			err = service.Delete("empty", false)
			if err != nil {
				t.Fatalf("delete empty dir: %v", err)
			}
		},
	)
}

func TestDeletePrefix_RemovesCachedArtifacts(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DeletePrefix removes cache subtree best effort", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		target := filepath.Join(root, "movies", "clip.mp4")
		err := os.MkdirAll(filepath.Dir(target), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(target, []byte("cache"), 0o600)
		if err != nil {
			t.Fatalf("write cache: %v", err)
		}

		mediafs.DeletePrefix(root, "movies/clip.mp4")

		_, statErr := os.Stat(target)
		if !os.IsNotExist(statErr) {
			t.Fatal("expected cache prefix removed")
		}

		mediafs.DeletePrefix("", "movies")
		mediafs.DeletePrefix(root, "")
	})
}

func TestService_DeleteReadOnlyDirectory(t *testing.T) {
	t.Parallel()

	allure.Test(t, "delete fails when parent directory is read-only", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		dir := filepath.Join(root, "movies")
		err = os.MkdirAll(dir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		target := filepath.Join(dir, "clip.mp4")
		err = os.WriteFile(target, []byte("video"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		err = os.Chmod( //nolint:gosec // simulate read-only parent directory for delete test
			dir,
			0o555,
		)
		if err != nil {
			t.Fatalf("chmod dir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(dir, 0o750) //nolint:gosec // restore permissions for temp dir cleanup
		})

		err = service.Delete("movies/clip.mp4", false)
		if err == nil {
			t.Fatal("expected read-only delete error")
		}
		if !errors.Is(err, mediafs.ErrDeleteReadOnly) {
			t.Fatalf("expected ErrDeleteReadOnly, got %v", err)
		}
	})
}

func TestService_DeleteRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	allure.Test(t, "delete rejects paths outside media root", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		err = service.Delete("/../../etc/passwd", false)
		if err == nil {
			t.Fatal("expected traversal error")
		}
		if !errors.Is(err, mediafs.ErrPathOutsideRoot) && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected ErrPathOutsideRoot or not exist, got %v", err)
		}
	})
}
