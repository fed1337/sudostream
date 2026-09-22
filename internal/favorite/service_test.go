package favorite_test

import (
	"context"
	"sudoStream/internal/access"
	"sudoStream/internal/favorite"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testFavLibraryID   = "lib-1"
	testFavLibraryPath = "movies"
	testFavUserID      = "user-1"
	testFavMediaPath   = "movies/demo.mp4"
)

type memoryFavoriteStore struct {
	rows map[string]bool
}

func (m *memoryFavoriteStore) Get(
	_ context.Context,
	userID, libraryID, relPath string,
) (bool, error) {
	return m.rows[userID+"|"+libraryID+"|"+relPath], nil
}

func (m *memoryFavoriteStore) Set(
	_ context.Context,
	userID, libraryID, relPath string,
	favorited bool,
) error {
	key := userID + "|" + libraryID + "|" + relPath
	if favorited {
		m.rows[key] = true
	} else {
		delete(m.rows, key)
	}

	return nil
}

func (m *memoryFavoriteStore) ListForPaths(
	_ context.Context,
	userID, libraryID string,
	relPaths []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(relPaths))
	for _, relPath := range relPaths {
		if m.rows[userID+"|"+libraryID+"|"+relPath] {
			result[relPath] = true
		}
	}

	return result, nil
}

func (m *memoryFavoriteStore) DeleteForPath(_ context.Context, libraryID, relPath string) error {
	suffix := "|" + libraryID + "|" + relPath
	for key := range m.rows {
		if len(key) >= len(suffix) && key[len(key)-len(suffix):] == suffix {
			delete(m.rows, key)
		}
	}

	return nil
}

type memoryAccessStore struct{}

func (memoryAccessStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return []access.Library{{
		ID:      testFavLibraryID,
		RelPath: testFavLibraryPath,
		Slug:    testFavLibraryPath,
		Name:    "Movies",
		Type:    access.LibraryTypeFilm,
	}}, nil
}

func (memoryAccessStore) GetLibrary(_ context.Context, libraryID string) (access.Library, error) {
	if libraryID == testFavLibraryID {
		return access.Library{
			ID:      testFavLibraryID,
			RelPath: testFavLibraryPath,
			Slug:    testFavLibraryPath,
			Name:    "Movies",
			Type:    access.LibraryTypeFilm,
		}, nil
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (memoryAccessStore) UpsertLibrary(
	_ context.Context,
	library access.Library,
) (access.Library, error) {
	return library, nil
}

func (s memoryAccessStore) CreateLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return s.UpsertLibrary(ctx, library)
}

func (s memoryAccessStore) AddRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (s memoryAccessStore) RemoveRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (memoryAccessStore) DeleteLibrary(_ context.Context, _ string) error { return nil }

func (memoryAccessStore) UpdateLibrary(
	ctx context.Context,
	libraryID string,
	name *string,
	libraryType *access.LibraryType,
) (access.Library, error) {
	library, err := (memoryAccessStore{}).GetLibrary(ctx, libraryID)
	if err != nil {
		return access.Library{}, err
	}
	if name != nil {
		library.Name = *name
	}
	if libraryType != nil {
		library.Type = *libraryType
	}

	return library, nil
}

func (memoryAccessStore) GetUserGrantMap(
	_ context.Context,
	_ string,
) (map[string]access.LibraryPermissions, error) {
	return map[string]access.LibraryPermissions{}, nil
}

func (memoryAccessStore) ReplaceUserGrants(
	_ context.Context,
	_ string,
	_ []access.GrantInput,
) error {
	return nil
}

func newFavoriteService(t *testing.T) *favorite.Service {
	t.Helper()

	media, err := mediafs.New(t.TempDir())
	if err != nil {
		t.Fatalf("mediafs: %v", err)
	}

	return favorite.NewService(
		media,
		access.NewService(memoryAccessStore{}),
		&memoryFavoriteStore{rows: map[string]bool{}},
	)
}

func TestService_SetGetClear(t *testing.T) {
	t.Parallel()

	allure.Test(t, "set favorite persists for user and path", func(a *allure.Context) {
		t := a.T()
		svc := newFavoriteService(t)
		ctx := context.Background()

		state, err := svc.Set(ctx, testFavUserID, testFavMediaPath, true)
		if err != nil {
			t.Fatalf("set: %v", err)
		}
		if !state.Favorited {
			t.Fatal("expected favorited true")
		}

		got, err := svc.Get(ctx, testFavUserID, testFavMediaPath)
		if err != nil || !got.Favorited {
			t.Fatalf("get: %#v err=%v", got, err)
		}

		cleared, err := svc.Set(ctx, testFavUserID, testFavMediaPath, false)
		if err != nil || cleared.Favorited {
			t.Fatalf("clear: %#v err=%v", cleared, err)
		}
	})
}

func TestService_FavoritedForPathsAndDelete(t *testing.T) {
	t.Parallel()

	allure.Test(t, "FavoritedForPaths and DeleteForPath", func(a *allure.Context) {
		t := a.T()
		svc := newFavoriteService(t)
		ctx := context.Background()

		_, err := svc.Set(ctx, testFavUserID, testFavMediaPath, true)
		if err != nil {
			t.Fatalf("set: %v", err)
		}

		flags, err := svc.FavoritedForPaths(
			ctx,
			testFavUserID,
			testFavLibraryPath,
			[]string{testFavMediaPath, "movies/other.mp4"},
		)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if !flags[testFavMediaPath] || flags["movies/other.mp4"] {
			t.Fatalf("flags=%v", flags)
		}

		err = svc.DeleteForPath(ctx, testFavMediaPath)
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		got, err := svc.Get(ctx, testFavUserID, testFavMediaPath)
		if err != nil || got.Favorited {
			t.Fatalf("after delete: %#v err=%v", got, err)
		}
	})
}
