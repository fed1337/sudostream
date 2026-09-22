package access_test

import (
	"context"
	"os"
	"path/filepath"
	"sudoStream/internal/access"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type syncMemoryStore struct {
	libraries map[string]access.Library
}

func newSyncMemoryStore(libs ...access.Library) *syncMemoryStore {
	store := &syncMemoryStore{libraries: make(map[string]access.Library)}
	for _, library := range libs {
		if len(library.Roots) == 0 && library.RelPath != "" {
			library.Roots = []string{library.RelPath}
		}
		store.libraries[library.ID] = library
	}

	return store
}

func (m *syncMemoryStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	out := make([]access.Library, 0, len(m.libraries))
	for _, library := range m.libraries {
		out = append(out, library)
	}

	return out, nil
}

func (m *syncMemoryStore) GetLibrary(_ context.Context, libraryID string) (access.Library, error) {
	library, ok := m.libraries[libraryID]
	if !ok {
		return access.Library{}, access.ErrLibraryNotFound
	}

	return library, nil
}

func (m *syncMemoryStore) CreateLibrary(
	_ context.Context,
	library access.Library,
) (access.Library, error) {
	library.ID = "id-" + library.Slug
	if library.Type == "" {
		library.Type = access.LibraryTypeOther
	}
	m.libraries[library.ID] = library

	return library, nil
}

func (m *syncMemoryStore) UpsertLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return m.CreateLibrary(ctx, library)
}

func (m *syncMemoryStore) DeleteLibrary(_ context.Context, libraryID string) error {
	if _, ok := m.libraries[libraryID]; !ok {
		return access.ErrLibraryNotFound
	}
	delete(m.libraries, libraryID)

	return nil
}

func (m *syncMemoryStore) UpdateLibrary(
	_ context.Context,
	libraryID string,
	name *string,
	libraryType *access.LibraryType,
) (access.Library, error) {
	library, ok := m.libraries[libraryID]
	if !ok {
		return access.Library{}, access.ErrLibraryNotFound
	}
	if name != nil {
		library.Name = *name
	}
	if libraryType != nil {
		library.Type = *libraryType
	}
	m.libraries[libraryID] = library

	return library, nil
}

func (m *syncMemoryStore) AddRoot(
	_ context.Context,
	libraryID, relPath string,
) (access.Library, error) {
	library, ok := m.libraries[libraryID]
	if !ok {
		return access.Library{}, access.ErrLibraryNotFound
	}
	library.Roots = append(library.RootPaths(), relPath)
	library.RelPath = library.Roots[0]
	m.libraries[libraryID] = library

	return library, nil
}

func (m *syncMemoryStore) RemoveRoot(
	_ context.Context,
	libraryID, relPath string,
) (access.Library, error) {
	library, ok := m.libraries[libraryID]
	if !ok {
		return access.Library{}, access.ErrLibraryNotFound
	}
	kept := make([]string, 0, len(library.RootPaths()))
	for _, root := range library.RootPaths() {
		if root != relPath {
			kept = append(kept, root)
		}
	}
	if len(kept) == 0 {
		delete(m.libraries, libraryID)

		return access.Library{}, access.ErrLibraryNotFound
	}
	library.Roots = kept
	library.RelPath = kept[0]
	m.libraries[libraryID] = library

	return library, nil
}

func (m *syncMemoryStore) GetUserGrantMap(
	_ context.Context,
	_ string,
) (map[string]access.LibraryPermissions, error) {
	return map[string]access.LibraryPermissions{}, nil
}

func (m *syncMemoryStore) ReplaceUserGrants(
	_ context.Context,
	_ string,
	_ []access.GrantInput,
) error {
	return nil
}

const (
	orphanLibrarySlug = "gone"
	testMoviesDir     = "movies"
)

func TestPruneMissingRoots_RemovesMissingOnly( //nolint:cyclop // prune vs auto-create assertions
	t *testing.T,
) {
	t.Parallel()

	allure.Test(
		t,
		"PruneMissingRoots drops missing dirs and does not create libraries from new folders",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			err := os.MkdirAll(filepath.Join(root, testMoviesDir), 0o750)
			if err != nil {
				t.Fatal(err)
			}
			err = os.MkdirAll(filepath.Join(root, "extra"), 0o750)
			if err != nil {
				t.Fatal(err)
			}

			store := newSyncMemoryStore(
				access.Library{
					ID:      "movies-id",
					Slug:    testMoviesDir,
					RelPath: testMoviesDir,
					Name:    testMoviesDir,
					Type:    access.LibraryTypeOther,
					Roots:   []string{testMoviesDir},
				},
				access.Library{
					ID:      orphanLibrarySlug,
					Slug:    orphanLibrarySlug,
					RelPath: orphanLibrarySlug,
					Name:    orphanLibrarySlug,
					Type:    access.LibraryTypeOther,
					Roots:   []string{orphanLibrarySlug},
				},
			)
			svc := access.NewService(store)

			result, err := svc.PruneMissingRoots(context.Background(), root)
			if err != nil {
				t.Fatalf("prune: %v", err)
			}
			if result.Added != 0 {
				t.Fatalf("expected no auto-add, got added=%d", result.Added)
			}
			if result.Removed != 1 || result.Kept != 1 {
				t.Fatalf("result=%+v", result)
			}
			if _, ok := store.libraries[orphanLibrarySlug]; ok {
				t.Fatal("orphan library should be deleted")
			}
			if _, ok := store.libraries["movies-id"]; !ok {
				t.Fatal("movies library should be kept")
			}

			result, err = svc.PruneMissingRoots(context.Background(), root)
			if err != nil {
				t.Fatalf("second prune: %v", err)
			}
			if result.Added != 0 || result.Removed != 0 || result.Kept != 1 {
				t.Fatalf("second result=%+v", result)
			}
		},
	)
}

func TestMatchLibrary_LongestPrefix(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"friends/S01E01 resolves to series library with extra roots",
		func(a *allure.Context) {
			t := a.T()
			libraries := []access.Library{{
				ID:    "series-id",
				Slug:  testLibrarySeries,
				Name:  "Series",
				Type:  access.LibraryTypeSeries,
				Roots: []string{testLibrarySeries, "friends"},
			}}

			library, ok := access.MatchLibrary(libraries, "friends/S01E01.mkv")
			if !ok || library.ID != "series-id" {
				t.Fatalf("match=%v library=%+v", ok, library)
			}

			_, ok = access.MatchLibrary(libraries, "dumps/file.mkv")
			if ok {
				t.Fatal("unassigned path should not match")
			}
		},
	)
}
