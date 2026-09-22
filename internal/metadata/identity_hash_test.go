package metadata

import (
	"context"
	"os"
	"path/filepath"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/mediahash"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestAttachIdentityHashes_PreservesExisting(t *testing.T) {
	t.Parallel()

	allure.Test(t, "attachIdentityHashes never overwrites existing movie/ed2k hashes", func(a *allure.Context) {
		t := a.T()
		path := writePatternFile(t, 128*1024)

		existingMovie := "deadbeefdeadbeef"
		existingEd2k := "cafebabe0123456789abcdef01234567"
		existingSize := int64(1)
		existing := VideoFields{
			MovieHash:    &existingMovie,
			Ed2kHash:     &existingEd2k,
			HashFileSize: &existingSize,
		}

		title := "probed"
		out := attachIdentityHashes(context.Background(), path, VideoFields{Title: &title}, existing)
		assertPtrString(t, "movie hash", out.MovieHash, existingMovie)
		assertPtrString(t, "ed2k hash", out.Ed2kHash, existingEd2k)
		assertPtrInt64(t, "hash file size", out.HashFileSize, existingSize)
		assertPtrString(t, "title", out.Title, title)
	})
}

func TestAttachIdentityHashes_ComputesWhenMissing(t *testing.T) {
	t.Parallel()

	allure.Test(t, "attachIdentityHashes computes hashes when absent", func(a *allure.Context) {
		t := a.T()
		path := writePatternFile(t, 128*1024)

		wantMovie, _, err := mediahash.MovieHash(path)
		if err != nil {
			t.Fatal(err)
		}
		wantEd2k, _, err := mediahash.Ed2k(path)
		if err != nil {
			t.Fatal(err)
		}

		out := attachIdentityHashes(context.Background(), path, VideoFields{}, VideoFields{})
		assertPtrString(t, "movie hash", out.MovieHash, wantMovie)
		assertPtrString(t, "ed2k hash", out.Ed2kHash, wantEd2k)
		assertPtrInt64(t, "hash file size", out.HashFileSize, 128*1024)
	})
}

func TestIdentityHashesIncomplete(t *testing.T) {
	t.Parallel()

	allure.Test(t, "IdentityHashesIncomplete requires ed2k and moviehash when large enough", func(a *allure.Context) {
		t := a.T()
		ed2k := "aabb"
		movie := "ccdd"
		large := int64(mediahash.MovieHashMinSize)
		small := int64(4)

		if !IdentityHashesIncomplete(VideoFields{}, large) {
			t.Fatal("empty fields incomplete")
		}
		if !IdentityHashesIncomplete(VideoFields{Ed2kHash: &ed2k}, large) {
			t.Fatal("large file without moviehash incomplete")
		}
		if IdentityHashesIncomplete(VideoFields{Ed2kHash: &ed2k}, small) {
			t.Fatal("small file with ed2k should be complete")
		}
		if IdentityHashesIncomplete(VideoFields{Ed2kHash: &ed2k, MovieHash: &movie}, large) {
			t.Fatal("both hashes present should be complete")
		}
	})
}

func TestIndexer_PreservesHashesAfterRetouch(t *testing.T) {
	t.Parallel()

	allure.Test(t, "second index after touch keeps movie_hash and ed2k_hash", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		libraryDir := filepath.Join(root, testMoviesLibrary)
		err := os.MkdirAll(libraryDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir library: %v", err)
		}

		videoPath := filepath.Join(libraryDir, "film.mkv")
		data := make([]byte, 128*1024)
		for i := range data {
			data[i] = byte(i % 251)
		}
		err = os.WriteFile(videoPath, data, 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}

		wantMovie, _, err := mediahash.MovieHash(videoPath)
		if err != nil {
			t.Fatal(err)
		}
		wantEd2k, _, err := mediahash.Ed2k(videoPath)
		if err != nil {
			t.Fatal(err)
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new media service: %v", err)
		}

		store := newMemoryIndexStore()
		indexer := NewIndexer(media, store)
		firstTitle := "First"
		indexer.probe = func(context.Context, string) (VideoFields, error) {
			return VideoFields{Title: &firstTitle}, nil
		}

		library := access.Library{
			ID:      "lib-2",
			RelPath: testMoviesLibrary,
			Slug:    testMoviesLibrary,
		}
		indexer.indexLibrary(context.Background(), library)

		rel := testMoviesLibrary + "/film.mkv"
		first, found := store.originals[store.key("lib-2", rel)]
		if !found {
			t.Fatal("expected first index row")
		}
		assertPtrString(t, "first movie hash", first.fields.MovieHash, wantMovie)
		assertPtrString(t, "first ed2k hash", first.fields.Ed2kHash, wantEd2k)

		newTime := first.mtime.Add(2 * time.Second)
		err = os.Chtimes(videoPath, newTime, newTime)
		if err != nil {
			t.Fatalf("chtimes: %v", err)
		}

		secondTitle := "Second"
		indexer.probe = func(context.Context, string) (VideoFields, error) {
			return VideoFields{Title: &secondTitle}, nil
		}
		indexer.indexLibrary(context.Background(), library)

		second, found := store.originals[store.key("lib-2", rel)]
		if !found {
			t.Fatal("expected second index row")
		}
		assertPtrString(t, "title", second.fields.Title, secondTitle)
		assertPtrString(t, "movie hash", second.fields.MovieHash, wantMovie)
		assertPtrString(t, "ed2k hash", second.fields.Ed2kHash, wantEd2k)
	})
}

func writePatternFile(t *testing.T, size int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.bin")
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	err := os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

func assertPtrString(t *testing.T, label string, got *string, want string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s: got %#v want %q", label, got, want)
	}
}

func assertPtrInt64(t *testing.T, label string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s: got %#v want %d", label, got, want)
	}
}
