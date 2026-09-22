package mediafs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestOpenFile_AllowsTildeNames(t *testing.T) {
	t.Parallel()

	allure.Test(t, "OpenFile allows tilde names", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()

		screencasts := filepath.Join(root, "screencasts")
		err := os.MkdirAll(screencasts, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		filePath := filepath.Join(screencasts, "SDSO8J~0")
		err = os.WriteFile(filePath, []byte("video-bytes"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		svc, err := New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		file, info, err := svc.OpenFile("/screencasts/SDSO8J~0")
		if err != nil {
			t.Fatalf("open file: %v", err)
		}
		defer func() { _ = file.Close() }()

		if info.Name() != "SDSO8J~0" {
			t.Fatalf("unexpected file name: %s", info.Name())
		}
	})
}

func TestBrowse_WebMFilesAreStreamableVideo(t *testing.T) {
	t.Parallel()

	allure.Test(t, "webm files get video/webm mime and play action", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		library := filepath.Join(root, "clips")
		err := os.MkdirAll(library, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		filePath := filepath.Join(library, "Screencast from 2024-10-25 15-02-01.webm")
		err = os.WriteFile(filePath, []byte("fake-webm-bytes"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		svc, err := New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		response, err := svc.Browse("clips")
		if err != nil {
			t.Fatalf("browse: %v", err)
		}

		if len(response.Folder.Children) != 1 {
			t.Fatalf("expected one child, got %d", len(response.Folder.Children))
		}

		item := response.Folder.Children[0]
		if item.MimeType != webmMimeType {
			t.Fatalf("mime: got %q want video/webm", item.MimeType)
		}
		if item.Actions.Play == "" {
			t.Fatal("expected play action for webm file")
		}
	})
}

func TestBrowse_ExtensionlessVideoMimeGetsPlayAction(t *testing.T) {
	t.Parallel()

	allure.Test(t, "extensionless video/webm sniff gets play action", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		library := filepath.Join(root, "clips")
		err := os.MkdirAll(library, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Minimal EBML header resembling webm enough for DetectContentType in some builds;
		// force mime via buildActions fallback when sniff returns video/webm.
		filePath := filepath.Join(library, "SDSO8J~0")
		payload := []byte{0x1a, 0x45, 0xdf, 0xa3, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1f}
		err = os.WriteFile(filePath, payload, 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		svc, err := New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		response, err := svc.Browse("clips")
		if err != nil {
			t.Fatalf("browse: %v", err)
		}

		item := response.Folder.Children[0]
		if !strings.HasPrefix(item.MimeType, "video/") && item.Actions.Play == "" {
			t.Fatalf("expected play for sniffed video file, mime=%q", item.MimeType)
		}
	})
}

func TestResolve_BlocksPathTraversal(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Resolve blocks path traversal", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()

		err := os.MkdirAll(filepath.Join(root, "safe"), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		svc, err := New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		_, _, err = svc.OpenFile("/../../etc/passwd")
		if err == nil {
			t.Fatal("expected traversal error")
		}
		if !errors.Is(err, ErrPathOutsideRoot) && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected outside-root or not-exist error, got: %v", err)
		}
	})
}

func TestDirPath_ResolvesLibraryFolder(t *testing.T) {
	t.Parallel()

	allure.Test(t, "dir path resolves library folder under media root", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		library := filepath.Join(root, "series")
		err := os.MkdirAll(library, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		svc, err := New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		dirPath, err := svc.DirPath("series")
		if err != nil {
			t.Fatalf("dir path: %v", err)
		}
		if dirPath != library {
			t.Fatalf("dir path: got %q want %q", dirPath, library)
		}

		_, err = svc.DirPath("series/missing")
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected missing dir error, got %v", err)
		}
	})
}
