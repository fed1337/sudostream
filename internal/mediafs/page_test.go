package mediafs

import (
	"os"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestNormalizePageOpts(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   PageOpts
		want PageOpts
	}{
		{
			name: "zero limit and negative offset",
			in:   PageOpts{Limit: 0, Offset: -3},
			want: PageOpts{Limit: DefaultPageLimit, Offset: 0},
		},
		{
			name: "clamps oversize limit",
			in:   PageOpts{Limit: 500, Offset: 2},
			want: PageOpts{Limit: MaxPageLimit, Offset: 2},
		},
	} {
		allure.Test(t, "normalize page opts — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got := NormalizePageOpts(tc.in)
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestSlicePage(t *testing.T) {
	t.Parallel()

	items := []int{1, 2, 3, 4, 5}
	for _, tc := range []struct {
		name      string
		opts      PageOpts
		wantLen   int
		wantFirst int
		wantTotal int
	}{
		{name: "mid page", opts: PageOpts{Limit: 2, Offset: 2}, wantLen: 2, wantFirst: 3, wantTotal: 5},
		{name: "offset past end", opts: PageOpts{Limit: 2, Offset: 99}, wantLen: 0, wantTotal: 5},
	} {
		allure.Test(t, "slice page — "+tc.name, func(a *allure.Context) {
			t := a.T()
			page, meta := SlicePage(items, tc.opts)
			if meta.Total != tc.wantTotal || meta.Limit != tc.opts.Limit || meta.Offset != tc.opts.Offset {
				t.Fatalf("meta=%+v", meta)
			}
			if len(page) != tc.wantLen {
				t.Fatalf("page=%v", page)
			}
			if tc.wantLen > 0 && page[0] != tc.wantFirst {
				t.Fatalf("first=%d want %d", page[0], tc.wantFirst)
			}
		})
	}
}

func TestBrowse_ShallowAndPaged(t *testing.T) {
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
