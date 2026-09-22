package watch_test

import (
	"context"
	"errors"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/watch"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testWatchLibraryID   = "lib-1"
	testWatchLibraryPath = "movies"
	testWatchUserID      = "user-1"
	testWatchMediaPath   = "movies/demo.mp4"
)

var (
	errTestWatchStore = errors.New("watch store down")
	errTestAccessList = errors.New("access list failed")
)

type memoryWatchStore struct {
	rows map[string]watch.State
}

func (m *memoryWatchStore) Get(
	_ context.Context,
	userID, libraryID, relPath string,
) (watch.State, error) {
	return m.rows[m.rowKey(userID, libraryID, relPath)], nil
}

func (m *memoryWatchStore) Upsert(
	_ context.Context,
	userID, libraryID, relPath string,
	state watch.State,
) error {
	m.rows[m.rowKey(userID, libraryID, relPath)] = state

	return nil
}

func (m *memoryWatchStore) Clear(_ context.Context, userID, libraryID, relPath string) error {
	delete(m.rows, m.rowKey(userID, libraryID, relPath))

	return nil
}

func (m *memoryWatchStore) ListForPaths(
	_ context.Context,
	userID, libraryID string,
	relPaths []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(relPaths))
	for _, relPath := range relPaths {
		if m.rows[m.rowKey(userID, libraryID, relPath)].Watched {
			result[relPath] = true
		}
	}

	return result, nil
}

func (m *memoryWatchStore) DeleteForPath(_ context.Context, libraryID, relPath string) error {
	suffix := "|" + libraryID + "|" + relPath
	for key := range m.rows {
		if len(key) >= len(suffix) && key[len(key)-len(suffix):] == suffix {
			delete(m.rows, key)
		}
	}

	return nil
}

func (m *memoryWatchStore) rowKey(userID, libraryID, relPath string) string {
	return userID + "|" + libraryID + "|" + relPath
}

type memoryAccessStore struct{}

func (memoryAccessStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return []access.Library{{
		ID:      testWatchLibraryID,
		RelPath: testWatchLibraryPath,
		Slug:    testWatchLibraryPath,
		Name:    "Movies",
		Type:    access.LibraryTypeFilm,
	}}, nil
}

func (memoryAccessStore) GetLibrary(_ context.Context, libraryID string) (access.Library, error) {
	if libraryID == testWatchLibraryID {
		return access.Library{
			ID:      testWatchLibraryID,
			RelPath: testWatchLibraryPath,
			Slug:    testWatchLibraryPath,
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

func (memoryAccessStore) DeleteLibrary(_ context.Context, _ string) error {
	return nil
}

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

type failingAccessStore struct {
	listErr error
}

func (f failingAccessStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return nil, f.listErr
}

func (failingAccessStore) GetLibrary(_ context.Context, _ string) (access.Library, error) {
	return access.Library{}, access.ErrLibraryNotFound
}

func (failingAccessStore) UpsertLibrary(
	_ context.Context,
	library access.Library,
) (access.Library, error) {
	return library, nil
}

func (failingAccessStore) CreateLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return (failingAccessStore{}).UpsertLibrary(ctx, library)
}

func (failingAccessStore) AddRoot(
	_ context.Context,
	_, _ string,
) (access.Library, error) {
	return access.Library{}, access.ErrLibraryNotFound
}

func (failingAccessStore) RemoveRoot(
	_ context.Context,
	_, _ string,
) (access.Library, error) {
	return access.Library{}, access.ErrLibraryNotFound
}

func (failingAccessStore) DeleteLibrary(_ context.Context, _ string) error {
	return nil
}

func (failingAccessStore) UpdateLibrary(
	_ context.Context,
	_ string,
	_ *string,
	_ *access.LibraryType,
) (access.Library, error) {
	return access.Library{}, access.ErrLibraryNotFound
}

func (failingAccessStore) GetUserGrantMap(
	_ context.Context,
	_ string,
) (map[string]access.LibraryPermissions, error) {
	return map[string]access.LibraryPermissions{}, nil
}

func (failingAccessStore) ReplaceUserGrants(
	_ context.Context,
	_ string,
	_ []access.GrantInput,
) error {
	return nil
}

type errorWatchStore struct {
	getErr    error
	setErr    error
	listErr   error
	deleteErr error
	rows      map[string]watch.State
}

func (e *errorWatchStore) Get(
	_ context.Context,
	userID, libraryID, relPath string,
) (watch.State, error) {
	if e.getErr != nil {
		return watch.State{}, e.getErr
	}

	return e.rows[userID+"|"+libraryID+"|"+relPath], nil
}

func (e *errorWatchStore) Upsert(
	_ context.Context,
	userID, libraryID, relPath string,
	state watch.State,
) error {
	if e.setErr != nil {
		return e.setErr
	}

	e.rows[userID+"|"+libraryID+"|"+relPath] = state

	return nil
}

func (e *errorWatchStore) Clear(_ context.Context, userID, libraryID, relPath string) error {
	if e.setErr != nil {
		return e.setErr
	}
	delete(e.rows, userID+"|"+libraryID+"|"+relPath)

	return nil
}

func (e *errorWatchStore) ListForPaths(
	_ context.Context,
	_, _ string,
	_ []string,
) (map[string]bool, error) {
	if e.listErr != nil {
		return nil, e.listErr
	}

	return map[string]bool{}, nil
}

func (e *errorWatchStore) DeleteForPath(_ context.Context, _, _ string) error {
	return e.deleteErr
}

func TestService_PatchProgress(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"patch ignores short starts then saves continue progress",
		func(a *allure.Context) {
			t := a.T()
			store := &memoryWatchStore{rows: map[string]watch.State{}}
			accessSvc := access.NewService(memoryAccessStore{})
			service := watch.NewService(nil, accessSvc, store)
			short := 4.0
			duration := 120.0
			state, err := service.Patch(
				context.Background(),
				testWatchUserID,
				testWatchMediaPath,
				watch.Patch{
					PositionSeconds: &short,
					DurationSeconds: &duration,
				},
			)
			if err != nil || state.HasProgress() {
				t.Fatalf("short start: %#v err=%v", state, err)
			}
			position := 25.0
			state, err = service.Patch(
				context.Background(),
				testWatchUserID,
				testWatchMediaPath,
				watch.Patch{
					PositionSeconds: &position,
					DurationSeconds: &duration,
				},
			)
			if err != nil || !state.HasProgress() || state.Watched {
				t.Fatalf("progress: %#v err=%v", state, err)
			}
			complete := 110.0
			state, err = service.Patch(
				context.Background(),
				testWatchUserID,
				testWatchMediaPath,
				watch.Patch{
					PositionSeconds: &complete,
					DurationSeconds: &duration,
				},
			)
			if err != nil || !state.Watched {
				t.Fatalf("complete: %#v err=%v", state, err)
			}
		},
	)
}

func TestService_SetAndGetWatched(t *testing.T) {
	t.Parallel()

	allure.Test(t, "set watched persists for user and path", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		state, err := service.Set(context.Background(), testWatchUserID, testWatchMediaPath, true)
		if err != nil {
			t.Fatalf("set watched: %v", err)
		}
		if !state.Watched {
			t.Fatal("expected watched true")
		}

		state, err = service.Get(context.Background(), testWatchUserID, testWatchMediaPath)
		if err != nil {
			t.Fatalf("get watched: %v", err)
		}
		if !state.Watched {
			t.Fatal("expected watched true on get")
		}

		state, err = service.Set(context.Background(), testWatchUserID, testWatchMediaPath, false)
		if err != nil {
			t.Fatalf("clear watched: %v", err)
		}
		if state.Watched {
			t.Fatal("expected watched false")
		}
	})
}

func TestService_WatchedForPaths(t *testing.T) {
	t.Parallel()

	allure.Test(t, "WatchedForPaths returns flags for folder items", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		_, err := service.Set(context.Background(), testWatchUserID, testWatchMediaPath, true)
		if err != nil {
			t.Fatalf("set watched: %v", err)
		}

		flags, err := service.WatchedForPaths(
			context.Background(),
			testWatchUserID,
			testWatchLibraryPath,
			[]string{testWatchMediaPath, "movies/other.mp4"},
		)
		if err != nil {
			t.Fatalf("list watched: %v", err)
		}
		if !flags[testWatchMediaPath] {
			t.Fatalf("expected watched flag for demo, got %+v", flags)
		}
		if flags["movies/other.mp4"] {
			t.Fatal("expected other file unwatched")
		}
	})

	allure.Test(t, "WatchedForPaths matches legacy percent-encoded rows", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		legacyPath := "movies/melrose%20place/Pilot.avi"
		decodedPath := "movies/melrose place/Pilot.avi"
		err := store.Upsert(
			context.Background(),
			testWatchUserID,
			testWatchLibraryID,
			legacyPath,
			watch.State{Watched: true},
		)
		if err != nil {
			t.Fatalf("seed legacy row: %v", err)
		}

		flags, err := service.WatchedForPaths(
			context.Background(),
			testWatchUserID,
			testWatchLibraryPath,
			[]string{decodedPath},
		)
		if err != nil {
			t.Fatalf("list watched: %v", err)
		}
		if !flags[decodedPath] {
			t.Fatalf("expected decoded path watched, got %+v", flags)
		}
	})

	allure.Test(t, "WatchedForPaths returns empty map for empty input", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		flags, err := service.WatchedForPaths(
			context.Background(),
			testWatchUserID,
			testWatchLibraryPath,
			nil,
		)
		if err != nil {
			t.Fatalf("list watched: %v", err)
		}
		if len(flags) != 0 {
			t.Fatalf("expected empty map, got %+v", flags)
		}
	})
}

func TestService_DeleteForPath(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DeleteForPath removes watch rows for media path", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		_, err := service.Set(context.Background(), testWatchUserID, testWatchMediaPath, true)
		if err != nil {
			t.Fatalf("set watched: %v", err)
		}

		err = service.DeleteForPath(context.Background(), testWatchMediaPath)
		if err != nil {
			t.Fatalf("delete watch rows: %v", err)
		}

		state, err := service.Get(context.Background(), testWatchUserID, testWatchMediaPath)
		if err != nil {
			t.Fatalf("get watched: %v", err)
		}
		if state.Watched {
			t.Fatal("expected watched cleared after delete")
		}
	})

	allure.Test(t, "DeleteForPath ignores unknown library paths", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		err := service.DeleteForPath(context.Background(), "unknown/demo.mp4")
		if err != nil {
			t.Fatalf("delete unknown path: %v", err)
		}
	})
}

func TestService_NilServiceGuards(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil service returns store unavailable", func(a *allure.Context) {
		t := a.T()
		var service *watch.Service

		_, err := service.Get(context.Background(), testWatchUserID, testWatchMediaPath)
		if !errors.Is(err, watch.ErrStoreUnavailable) {
			t.Fatalf("get: got %v", err)
		}

		_, err = service.Set(context.Background(), testWatchUserID, testWatchMediaPath, true)
		if !errors.Is(err, watch.ErrStoreUnavailable) {
			t.Fatalf("set: got %v", err)
		}

		_, err = service.Patch(
			context.Background(),
			testWatchUserID,
			testWatchMediaPath,
			watch.Patch{},
		)
		if !errors.Is(err, watch.ErrStoreUnavailable) {
			t.Fatalf("patch: got %v", err)
		}

		flags, err := service.WatchedForPaths(
			context.Background(),
			testWatchUserID,
			testWatchLibraryPath,
			[]string{testWatchMediaPath},
		)
		if err != nil {
			t.Fatalf("watched for paths: %v", err)
		}
		if len(flags) != 0 {
			t.Fatalf("expected empty map, got %+v", flags)
		}

		err = service.DeleteForPath(context.Background(), testWatchMediaPath)
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
	})
}

func TestService_PathValidation(t *testing.T) {
	t.Parallel()

	allure.Test(t, "empty path is rejected", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		_, err := service.Get(context.Background(), testWatchUserID, " / ")
		if !errors.Is(err, mediafs.ErrInvalidPath) {
			t.Fatalf("expected invalid path, got %v", err)
		}
	})

	allure.Test(t, "unknown library path is rejected", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		_, err := service.Get(context.Background(), testWatchUserID, "unknown/demo.mp4")
		if !errors.Is(err, watch.ErrUnknownLibrary) {
			t.Fatalf("expected unknown library, got %v", err)
		}
	})
}

func TestService_StoreErrors(t *testing.T) {
	t.Parallel()

	allure.Test(t, "store errors are wrapped", func(a *allure.Context) {
		t := a.T()
		store := &errorWatchStore{getErr: errTestWatchStore, rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		_, err := service.Get(context.Background(), testWatchUserID, testWatchMediaPath)
		if err == nil || !errors.Is(err, errTestWatchStore) {
			t.Fatalf("expected wrapped get error, got %v", err)
		}

		store = &errorWatchStore{setErr: errTestWatchStore, rows: map[string]watch.State{}}
		service = watch.NewService(nil, accessSvc, store)
		_, err = service.Set(context.Background(), testWatchUserID, testWatchMediaPath, true)
		if err == nil || !errors.Is(err, errTestWatchStore) {
			t.Fatalf("expected wrapped set error, got %v", err)
		}

		store = &errorWatchStore{listErr: errTestWatchStore, rows: map[string]watch.State{}}
		service = watch.NewService(nil, accessSvc, store)
		_, err = service.WatchedForPaths(
			context.Background(),
			testWatchUserID,
			testWatchLibraryPath,
			[]string{testWatchMediaPath},
		)
		if err == nil || !errors.Is(err, errTestWatchStore) {
			t.Fatalf("expected wrapped list error, got %v", err)
		}

		store = &errorWatchStore{deleteErr: errTestWatchStore, rows: map[string]watch.State{}}
		service = watch.NewService(nil, accessSvc, store)
		err = service.DeleteForPath(context.Background(), testWatchMediaPath)
		if err == nil || !errors.Is(err, errTestWatchStore) {
			t.Fatalf("expected wrapped delete error, got %v", err)
		}
	})
}

func TestService_WatchedForPathsSkipsBlankPaths(t *testing.T) {
	t.Parallel()

	allure.Test(t, "WatchedForPaths skips blank item paths", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(memoryAccessStore{})
		service := watch.NewService(nil, accessSvc, store)

		flags, err := service.WatchedForPaths(
			context.Background(),
			testWatchUserID,
			testWatchLibraryPath,
			[]string{"", "  ", testWatchMediaPath},
		)
		if err != nil {
			t.Fatalf("list watched: %v", err)
		}
		if len(flags) != 0 {
			t.Fatalf("expected no watched flags, got %+v", flags)
		}
	})
}

func TestService_AccessResolveErrors(t *testing.T) {
	t.Parallel()

	allure.Test(t, "resolve errors from access list propagate", func(a *allure.Context) {
		t := a.T()
		store := &memoryWatchStore{rows: map[string]watch.State{}}
		accessSvc := access.NewService(failingAccessStore{listErr: errTestAccessList})
		service := watch.NewService(nil, accessSvc, store)

		_, err := service.Get(context.Background(), testWatchUserID, testWatchMediaPath)
		if err == nil {
			t.Fatal("expected resolve error")
		}

		err = service.DeleteForPath(context.Background(), testWatchMediaPath)
		if err == nil {
			t.Fatal("expected delete resolve error")
		}
	})
}
