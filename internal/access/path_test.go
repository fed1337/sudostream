package access

import (
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testFilesRoot      = "files"
	testFilesWin95Path = "files/win95.mp4"
	testScreensRoot    = "Screencasts (Copy)"
)

func TestCanonicalRelPath_CollapsesEncodedParent(t *testing.T) {
	t.Parallel()

	allure.Test(t, "encoded parent segments collapse before ACL matching", func(a *allure.Context) {
		t := a.T()
		cases := []struct {
			raw  string
			want string
		}{
			{testScreensRoot + "/..%2ffiles/win95.mp4", testFilesWin95Path},
			{testScreensRoot + "/%2e%2e/files/win95.mp4", testFilesWin95Path},
			{testScreensRoot + "/%2e%2e%2ffiles/win95.mp4", testFilesWin95Path},
			{testFilesWin95Path, testFilesWin95Path},
			{"files/../files/win95.mp4", testFilesWin95Path},
			{"", ""},
		}
		for _, tc := range cases {
			got, err := CanonicalRelPath(tc.raw)
			if err != nil {
				t.Fatalf("%q: unexpected err %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("%q: got %q want %q", tc.raw, got, tc.want)
			}
		}
	})
}

func TestCanonicalRelPath_RejectsResidualEncoding(t *testing.T) {
	t.Parallel()

	allure.Test(t, "over-encoded path segments are rejected", func(a *allure.Context) {
		t := a.T()
		// Three layers of encoding leave %2e after two unescape rounds.
		_, err := CanonicalRelPath("%25252e%25252e%25252ffiles/x.mp4")
		if err == nil {
			t.Fatal("expected ErrInvalidPath for residual encoding")
		}
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("got %v want ErrInvalidPath", err)
		}
	})
}

func TestCanonicalRelPath_RejectsNullByte(t *testing.T) {
	t.Parallel()

	allure.Test(t, "null bytes in path are rejected", func(a *allure.Context) {
		t := a.T()
		_, err := CanonicalRelPath(testFilesWin95Path + "\x00.txt")
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("got %v want ErrInvalidPath", err)
		}
	})
}

func TestMatchLibrary_EncodedBypassResolvesToTargetLibrary(t *testing.T) {
	t.Parallel()

	allure.Test(t, "MatchLibrary uses canonical path so spoofed prefix maps to real library", func(a *allure.Context) {
		t := a.T()
		libraries := []Library{
			{ID: "screens", Slug: "screens", Roots: []string{testScreensRoot}},
			{ID: testFilesRoot, Slug: testFilesRoot, Roots: []string{testFilesRoot}},
		}
		lib, ok := MatchLibrary(libraries, testScreensRoot+"/..%2ffiles/win95.mp4")
		if !ok {
			t.Fatal("expected match")
		}
		if lib.ID != testFilesRoot {
			t.Fatalf("got library %q want files", lib.ID)
		}
	})
}
