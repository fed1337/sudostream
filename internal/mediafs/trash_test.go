package mediafs_test

import (
	"errors"
	"os"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestService_MoveToTrashPreservesTree(t *testing.T) {
	t.Parallel()

	allure.Test(t, "MoveToTrash relocates a directory under .trash/<id>", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		dir := filepath.Join(root, "series", "show")
		err = os.MkdirAll(dir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(dir, "pilot.mkv"), []byte("video"), 0o600)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		itemID := "11111111-1111-1111-1111-111111111111"
		original, trashRel, err := service.MoveToTrash("series/show", itemID)
		if err != nil {
			t.Fatalf("move: %v", err)
		}
		if original != "series/show" {
			t.Fatalf("original: %q", original)
		}
		if trashRel != ".trash/"+itemID+"/series/show" {
			t.Fatalf("trash rel: %q", trashRel)
		}

		_, err = os.Stat(filepath.Join(root, "series", "show"))
		if !os.IsNotExist(err) {
			t.Fatal("expected source gone")
		}
		_, err = os.Stat(filepath.Join(root, ".trash", itemID, "series", "show", "pilot.mkv"))
		if err != nil {
			t.Fatalf("trashed file: %v", err)
		}
	})
}

func TestService_MoveToTrashRejectsRootAndTrash(t *testing.T) {
	t.Parallel()

	allure.Test(t, "MoveToTrash refuses media root and .trash paths", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		_, _, err = service.MoveToTrash("", "11111111-1111-1111-1111-111111111111")
		if !errors.Is(err, mediafs.ErrTrashForbidden) {
			t.Fatalf("root: %v", err)
		}

		err = os.MkdirAll(filepath.Join(root, ".trash", "x"), 0o750)
		if err != nil {
			t.Fatalf("mkdir trash: %v", err)
		}
		_, _, err = service.MoveToTrash(".trash", "11111111-1111-1111-1111-111111111111")
		if !errors.Is(err, mediafs.ErrTrashForbidden) {
			t.Fatalf("trash dir: %v", err)
		}
	})
}

func TestService_RestoreFromTrashAndConflict(t *testing.T) { //nolint:cyclop
	t.Parallel()

	allure.Test(t, "restore returns a file; collision is 409-class error", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		clip := filepath.Join(root, "movies", "clip.mp4")
		err = os.MkdirAll(filepath.Dir(clip), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(clip, []byte("video"), 0o600)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		itemID := "22222222-2222-2222-2222-222222222222"
		_, trashRel, err := service.MoveToTrash("movies/clip.mp4", itemID)
		if err != nil {
			t.Fatalf("move: %v", err)
		}

		err = service.RestoreFromTrash("movies/clip.mp4", trashRel, itemID)
		if err != nil {
			t.Fatalf("restore: %v", err)
		}
		_, err = os.Stat(clip)
		if err != nil {
			t.Fatalf("restored: %v", err)
		}

		_, trashRel, err = service.MoveToTrash("movies/clip.mp4", itemID)
		if err != nil {
			t.Fatalf("move again: %v", err)
		}
		err = os.MkdirAll(filepath.Dir(clip), 0o750)
		if err != nil {
			t.Fatalf("mkdir dest: %v", err)
		}
		err = os.WriteFile(clip, []byte("other"), 0o600)
		if err != nil {
			t.Fatalf("write dest: %v", err)
		}
		err = service.RestoreFromTrash("movies/clip.mp4", trashRel, itemID)
		if !errors.Is(err, mediafs.ErrRestoreConflict) {
			t.Fatalf("conflict: %v", err)
		}
	})
}

func TestService_TrashInfoRoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "info.json survives a trash item write/read", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		itemID := "33333333-3333-3333-3333-333333333333"
		info := mediafs.TrashInfo{
			ID:              itemID,
			OriginalRelPath: "movies/clip.mp4",
			TrashRelPath:    ".trash/" + itemID + "/movies/clip.mp4",
			DeletedAt:       "2026-01-02T03:04:05Z",
		}
		err = service.WriteTrashInfo(itemID, info)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		got, err := service.ReadTrashInfo(itemID)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if got.OriginalRelPath != info.OriginalRelPath {
			t.Fatalf("got %+v", got)
		}

		ids, err := service.ListTrashItemIDs()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(ids) != 1 || ids[0] != itemID {
			t.Fatalf("ids: %v", ids)
		}
	})
}
