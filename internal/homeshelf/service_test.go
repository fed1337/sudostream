package homeshelf_test

import (
	"context"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/favorite"
	"sudoStream/internal/homeshelf"
	"sudoStream/internal/metadata"
	"sudoStream/internal/watch"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testLibraryID     = "lib-1"
	testLibraryPath   = "movies"
	testContinuePath  = "movies/d.mp4"
	testContinuePathB = "movies/e.mp4"
)

type fakeAccess struct{}

func (fakeAccess) ListLibraries(_ context.Context) ([]access.Library, error) {
	return []access.Library{testLibrary()}, nil
}

func (fakeAccess) GetLibrary(_ context.Context, id string) (access.Library, error) {
	if id == testLibraryID {
		return testLibrary(), nil
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (fakeAccess) UpsertLibrary(_ context.Context, library access.Library) (access.Library, error) {
	return library, nil
}

func (fakeAccess) CreateLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return (fakeAccess{}).UpsertLibrary(ctx, library)
}

func (fakeAccess) AddRoot(ctx context.Context, libraryID, _ string) (access.Library, error) {
	return (fakeAccess{}).GetLibrary(ctx, libraryID)
}

func (fakeAccess) RemoveRoot(ctx context.Context, libraryID, _ string) (access.Library, error) {
	return (fakeAccess{}).GetLibrary(ctx, libraryID)
}

func (fakeAccess) DeleteLibrary(_ context.Context, _ string) error { return nil }

func (fakeAccess) UpdateLibrary(
	ctx context.Context,
	libraryID string,
	name *string,
	libraryType *access.LibraryType,
) (access.Library, error) {
	library, err := (fakeAccess{}).GetLibrary(ctx, libraryID)
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

func (fakeAccess) GetUserGrantMap(
	_ context.Context,
	_ string,
) (map[string]access.LibraryPermissions, error) {
	return map[string]access.LibraryPermissions{
		testLibraryID: {Read: true},
	}, nil
}

func (fakeAccess) ReplaceUserGrants(_ context.Context, _ string, _ []access.GrantInput) error {
	return nil
}

func testLibrary() access.Library {
	return access.Library{
		ID: testLibraryID, RelPath: testLibraryPath, Slug: testLibraryPath, Name: "Movies",
		Type: access.LibraryTypeFilm,
	}
}

type favStore struct{ rows []favorite.Row }

func (f favStore) ListForUser(
	_ context.Context,
	_ string,
	_ []string,
	limit, offset int,
) ([]favorite.Row, error) {
	return pageSlice(f.rows, limit, offset), nil
}

func (f favStore) CountForUser(_ context.Context, _ string, _ []string) (int64, error) {
	return int64(len(f.rows)), nil
}

type watchStore struct {
	rows         []watch.Row
	continueRows []watch.Row
	counts       map[string]int64
}

func (w watchStore) ListForUser(
	_ context.Context,
	_ string,
	_ []string,
	limit, offset int,
) ([]watch.Row, error) {
	return pageSlice(w.rows, limit, offset), nil
}

func (w watchStore) ListContinue(
	_ context.Context,
	_ string,
	_ []string,
	limit, offset int,
) ([]watch.Row, error) {
	return pageSlice(w.continueRows, limit, offset), nil
}

func (w watchStore) CountForUser(_ context.Context, _ string, _ []string) (int64, error) {
	return int64(len(w.rows)), nil
}

func (w watchStore) CountContinue(_ context.Context, _ string, _ []string) (int64, error) {
	return int64(len(w.continueRows)), nil
}

func (w watchStore) CountByLibraries(
	_ context.Context,
	_ string,
	_ []string,
) (map[string]int64, error) {
	return w.counts, nil
}

type metaStore struct {
	totals    map[string]int64
	unwatched []metadata.ShelfRow
	items     []metadata.ShelfRow
}

func (m metaStore) CountByLibraries(_ context.Context, _ []string) (map[string]int64, error) {
	return m.totals, nil
}

func (m metaStore) ListUnwatched(
	_ context.Context,
	_ string,
	_ []string,
	limit, offset int,
) ([]metadata.ShelfRow, error) {
	return pageSlice(m.unwatched, limit, offset), nil
}

func (m metaStore) CountUnwatched(_ context.Context, _ string, _ []string) (int64, error) {
	return int64(len(m.unwatched)), nil
}

func (m metaStore) ListShelfItems(
	_ context.Context,
	_ []string,
	_ []string,
) ([]metadata.ShelfRow, error) {
	return m.items, nil
}

func TestService_ShelvesAndStats(t *testing.T) {
	t.Parallel()

	allure.Test(t, "homeshelf favorites watched unwatched stats", func(a *allure.Context) {
		t := a.T()
		svc := newTestService()
		user := auth.PublicUser{ID: "u1", Role: auth.RoleUser}
		assertShelf(t, "favorites", svc.Favorites, user, "A")
		assertShelf(t, "watched", svc.Watched, user, "B")
		assertShelf(t, "unwatched", svc.Unwatched, user, "C")
		assertShelf(t, "continue", svc.Continue, user, "D")
		stats, err := svc.Stats(context.Background(), user)
		if err != nil || len(stats) != 1 || stats[0].WatchedPercent != 25 {
			t.Fatalf("stats: %#v err=%v", stats, err)
		}
	})
}

func TestService_ContinueOffset(t *testing.T) {
	t.Parallel()

	allure.Test(t, "continue shelf honors limit and offset", func(a *allure.Context) {
		t := a.T()
		svc := homeshelf.NewService(
			access.NewService(fakeAccess{}),
			favStore{},
			watchStore{
				continueRows: []watch.Row{
					{LibraryID: testLibraryID, RelPath: testContinuePath},
					{LibraryID: testLibraryID, RelPath: testContinuePathB},
				},
			},
			metaStore{
				items: []metadata.ShelfRow{
					{LibraryID: testLibraryID, RelPath: testContinuePath, Title: "D"},
					{LibraryID: testLibraryID, RelPath: testContinuePathB, Title: "E"},
				},
			},
		)
		user := auth.PublicUser{ID: "u1", Role: auth.RoleUser}
		page, err := svc.Continue(context.Background(), user, 1, 1)
		if err != nil || page.Total != 2 || page.Limit != 1 || page.Offset != 1 ||
			len(page.Items) != 1 || page.Items[0].Title != "E" {
			t.Fatalf("continue page: %#v err=%v", page, err)
		}
	})
}

func newTestService() *homeshelf.Service {
	return homeshelf.NewService(
		access.NewService(fakeAccess{}),
		favStore{rows: []favorite.Row{{LibraryID: testLibraryID, RelPath: "movies/a.mp4"}}},
		watchStore{
			rows:         []watch.Row{{LibraryID: testLibraryID, RelPath: "movies/b.mp4"}},
			continueRows: []watch.Row{{LibraryID: testLibraryID, RelPath: testContinuePath}},
			counts:       map[string]int64{testLibraryID: 1},
		},
		metaStore{
			totals: map[string]int64{testLibraryID: 4},
			unwatched: []metadata.ShelfRow{{
				LibraryID: testLibraryID, RelPath: "movies/c.mp4", Title: "C",
			}},
			items: []metadata.ShelfRow{
				{LibraryID: testLibraryID, RelPath: "movies/a.mp4", Title: "A"},
				{LibraryID: testLibraryID, RelPath: "movies/b.mp4", Title: "B"},
				{LibraryID: testLibraryID, RelPath: testContinuePath, Title: "D"},
			},
		},
	)
}

func pageSlice[T any](rows []T, limit, offset int) []T {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || offset >= len(rows) {
		return []T{}
	}
	end := min(offset+limit, len(rows))

	return rows[offset:end]
}

func assertShelf(
	t *testing.T,
	name string,
	list func(context.Context, auth.PublicUser, int, int) (homeshelf.ItemPage, error),
	user auth.PublicUser,
	wantTitle string,
) {
	t.Helper()
	page, err := list(context.Background(), user, 10, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Title != wantTitle ||
		page.Total != 1 || page.Limit != 10 || page.Offset != 0 {
		t.Fatalf("%s: %#v err=%v", name, page, err)
	}
}
