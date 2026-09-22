package mediafs

import (
	"os"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestNormalizePageOpts(t *testing.T) {
	t.Parallel()

	allure.Test(t, "clamps limit and offset", func(a *allure.Context) {
		t := a.T()
		got := NormalizePageOpts(PageOpts{Limit: 0, Offset: -3})
		if got.Limit != DefaultPageLimit || got.Offset != 0 {
			t.Fatalf("got %+v", got)
		}
		got = NormalizePageOpts(PageOpts{Limit: 500, Offset: 2})
		if got.Limit != MaxPageLimit || got.Offset != 2 {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestSlicePage(t *testing.T) {
	t.Parallel()

	allure.Test(t, "slices and reports total", func(a *allure.Context) {
		t := a.T()
		items := []int{1, 2, 3, 4, 5}
		page, meta := SlicePage(items, PageOpts{Limit: 2, Offset: 2})
		if meta.Total != 5 || meta.Limit != 2 || meta.Offset != 2 {
			t.Fatalf("meta=%+v", meta)
		}
		if len(page) != 2 || page[0] != 3 || page[1] != 4 {
			t.Fatalf("page=%v", page)
		}
		empty, meta := SlicePage(items, PageOpts{Limit: 2, Offset: 99})
		if len(empty) != 0 || meta.Total != 5 {
			t.Fatalf("empty=%v meta=%+v", empty, meta)
		}
	})
}

func TestBrowse_ShallowAndPaged(t *testing.T) { //nolint:cyclop // multi-assert browse paging
	t.Parallel()

	allure.Test(t, "browse is shallow dirs-first and ApplyPage slices", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		lib := filepath.Join(root, "lib")
		nested := filepath.Join(lib, "subdir")
		err := os.MkdirAll(nested, 0o750)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(nested, "hidden.mkv"), []byte("x"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(lib, "b.mkv"), []byte("x"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(lib, "a.mkv"), []byte("x"), 0o600)
		if err != nil {
			t.Fatal(err)
		}

		svc, err := New(root)
		if err != nil {
			t.Fatal(err)
		}

		response, err := svc.Browse("lib")
		if err != nil {
			t.Fatalf("browse: %v", err)
		}
		if len(response.Folder.Children) != 3 {
			t.Fatalf("children=%d", len(response.Folder.Children))
		}
		if !response.Folder.Children[0].IsDir {
			t.Fatal("expected dirs first")
		}
		if len(response.Folder.Children[0].Children) != 0 {
			t.Fatal("expected shallow dir with no nested children")
		}

		ApplyPage(&response, PageOpts{Limit: 2, Offset: 0})
		if response.Total != 3 || response.Limit != 2 || len(response.Folder.Children) != 2 {
			t.Fatalf("paged=%+v children=%d", response, len(response.Folder.Children))
		}
	})
}
