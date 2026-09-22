package metadata

import (
	"context"
	"os"
	"path/filepath"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testIndexedProbedTitle = "Probed"
	testMoviesLibrary      = "movies"
)

type memoryIndexStore struct {
	originals map[string]indexedOriginal
}

type indexedOriginal struct {
	fields   VideoFields
	mtime    time.Time
	size     int64
	probedAt time.Time
}

func newMemoryIndexStore() *memoryIndexStore {
	return &memoryIndexStore{originals: map[string]indexedOriginal{}}
}

func (m *memoryIndexStore) Get(_ context.Context, libraryID, relPath string) (Row, error) {
	row, ok := m.originals[m.key(libraryID, relPath)]
	if !ok {
		return Row{}, ErrNotFound
	}

	return Row{
		LibraryID: libraryID,
		RelPath:   relPath,
		Original:  row.fields,
		FileMtime: &row.mtime,
		FileSize:  &row.size,
		ProbedAt:  &row.probedAt,
	}, nil
}

func (m *memoryIndexStore) UpsertOriginal(
	_ context.Context,
	libraryID, relPath string,
	original VideoFields,
	fileMtime time.Time,
	fileSize int64,
	probedAt time.Time,
) error {
	m.originals[m.key(libraryID, relPath)] = indexedOriginal{
		fields:   original,
		mtime:    fileMtime,
		size:     fileSize,
		probedAt: probedAt,
	}

	return nil
}

func (m *memoryIndexStore) ListIndexedPaths(_ context.Context, libraryID string) ([]string, error) {
	prefix := libraryID + "|"
	paths := make([]string, 0)
	for key := range m.originals {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			paths = append(paths, key[len(prefix):])
		}
	}

	return paths, nil
}

func (m *memoryIndexStore) DeletePath(_ context.Context, libraryID, relPath string) error {
	delete(m.originals, m.key(libraryID, relPath))

	return nil
}

func (m *memoryIndexStore) NeedsProbe(
	_ context.Context,
	libraryID, relPath string,
	fileMtime time.Time,
	fileSize int64,
) (bool, error) {
	row, ok := m.originals[m.key(libraryID, relPath)]
	if !ok {
		return true, nil
	}

	if !row.mtime.Equal(fileMtime) || row.size != fileSize {
		return true, nil
	}

	return IdentityHashesIncomplete(row.fields, fileSize), nil
}

func (m *memoryIndexStore) key(libraryID, relPath string) string {
	return libraryID + "|" + relPath
}

type stubFrozenPaths struct {
	prefixes []string
}

func (s stubFrozenPaths) OriginalPrefixes(_ context.Context) ([]string, error) {
	return s.prefixes, nil
}

func TestIndexer_ProbesNewFilesAndDeletesStalePaths(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"indexer probes new videos and removes stale metadata rows",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			libraryDir := filepath.Join(root, testLibrarySlug)
			err := os.MkdirAll(libraryDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir library: %v", err)
			}

			videoPath := filepath.Join(libraryDir, "S01E01.mkv")
			err = os.WriteFile(videoPath, []byte("video"), 0o600)
			if err != nil {
				t.Fatalf("write video: %v", err)
			}

			stalePath := testLibrarySlug + "/removed.mkv"
			title := "Stale"
			store := newMemoryIndexStore()
			store.originals[store.key("lib-1", stalePath)] = indexedOriginal{
				fields: VideoFields{Title: &title},
			}

			media, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("new media service: %v", err)
			}

			indexer := NewIndexer(media, store)
			probedTitle := testIndexedProbedTitle
			indexer.probe = func(context.Context, string) (VideoFields, error) {
				return VideoFields{Title: &probedTitle}, nil
			}

			library := access.Library{
				ID:      "lib-1",
				Slug:    testLibrarySlug,
				RelPath: testLibrarySlug,
				Type:    access.LibraryTypeSeries,
			}

			indexer.indexLibrary(context.Background(), library)

			indexed, ok := store.originals[store.key("lib-1", testLibrarySlug+"/S01E01.mkv")]
			if !ok {
				t.Fatal("expected new video to be indexed")
			}
			if indexed.fields.Title == nil || *indexed.fields.Title != testIndexedProbedTitle {
				t.Fatalf("unexpected probed title: %#v", indexed.fields.Title)
			}

			if _, ok := store.originals[store.key("lib-1", stalePath)]; ok {
				t.Fatal("expected stale metadata row to be deleted")
			}
		},
	)
}

func TestIndexer_SkipsTrashedPathsDuringPrune(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"indexer does not wipe metadata for paths in the recycle bin",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			libraryDir := filepath.Join(root, testLibrarySlug)
			err := os.MkdirAll(libraryDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir library: %v", err)
			}

			videoPath := filepath.Join(libraryDir, "S01E01.mkv")
			err = os.WriteFile(videoPath, []byte("video"), 0o600)
			if err != nil {
				t.Fatalf("write video: %v", err)
			}

			trashed := testLibrarySlug + "/pilot.mkv"
			title := "Keep"
			store := newMemoryIndexStore()
			store.originals[store.key("lib-1", trashed)] = indexedOriginal{
				fields: VideoFields{Title: &title},
			}

			media, err := mediafs.New(root)
			if err != nil {
				t.Fatalf("new media service: %v", err)
			}

			indexer := NewIndexer(media, store)
			indexer.SetFrozenPaths(stubFrozenPaths{prefixes: []string{trashed}})
			indexer.probe = func(context.Context, string) (VideoFields, error) {
				probed := testIndexedProbedTitle

				return VideoFields{Title: &probed}, nil
			}

			indexer.indexLibrary(context.Background(), access.Library{
				ID:      "lib-1",
				Slug:    testLibrarySlug,
				RelPath: testLibrarySlug,
				Type:    access.LibraryTypeSeries,
			})

			if _, ok := store.originals[store.key("lib-1", trashed)]; !ok {
				t.Fatal("expected trashed metadata to be kept")
			}
		},
	)
}

func TestIndexer_SkipsUnchangedFiles(t *testing.T) {
	t.Parallel()

	allure.Test(t, "indexer skips probe when file stat matches cache", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		libraryDir := filepath.Join(root, testMoviesLibrary)
		err := os.MkdirAll(libraryDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir library: %v", err)
		}

		videoPath := filepath.Join(libraryDir, "film.mkv")
		err = os.WriteFile(videoPath, []byte("video"), 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}

		info, err := os.Stat(videoPath)
		if err != nil {
			t.Fatalf("stat video: %v", err)
		}

		cachedTitle := "Cached"
		movieHash := "0123456789abcdef"
		ed2kHash := "aabbccddeeff00112233445566778899"
		hashSize := info.Size()
		store := newMemoryIndexStore()
		store.originals[store.key("lib-2", testMoviesLibrary+"/film.mkv")] = indexedOriginal{
			fields: VideoFields{
				Title:        &cachedTitle,
				MovieHash:    &movieHash,
				Ed2kHash:     &ed2kHash,
				HashFileSize: &hashSize,
			},
			mtime:    info.ModTime(),
			size:     info.Size(),
			probedAt: time.Now().UTC(),
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new media service: %v", err)
		}

		indexer := NewIndexer(media, store)
		probeCalls := 0
		indexer.probe = func(context.Context, string) (VideoFields, error) {
			probeCalls++

			return VideoFields{}, nil
		}

		indexer.indexLibrary(context.Background(), access.Library{
			ID:      "lib-2",
			RelPath: testMoviesLibrary,
			Slug:    testMoviesLibrary,
		})

		if probeCalls != 0 {
			t.Fatalf("expected no probe calls, got %d", probeCalls)
		}
	})
}
