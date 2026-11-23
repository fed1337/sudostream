package access_test

import (
	"context"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"sync/atomic"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

// countingStore wraps memoryStore and counts ListLibraries / GetUserGrantMap calls.
type countingStore struct {
	memoryStore
	listCalls  atomic.Int32
	grantCalls atomic.Int32
}

func (c *countingStore) ListLibraries(ctx context.Context) ([]access.Library, error) {
	c.listCalls.Add(1)

	return c.memoryStore.ListLibraries(ctx)
}

func (c *countingStore) GetUserGrantMap(
	ctx context.Context,
	userID string,
) (map[string]access.LibraryPermissions, error) {
	c.grantCalls.Add(1)

	return c.memoryStore.GetUserGrantMap(ctx, userID)
}

func TestService_ACLCache_ReusesLibrariesAndGrants(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ACL cache serves repeated CanRead without extra store hits", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{
			memoryStore: memoryStore{
				grants: map[string]map[string]access.LibraryPermissions{
					testUserOne: {"1": readOnlyPerms()},
				},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		for range 5 {
			allowed, err := service.CanRead(context.Background(), user, testPathSeriesEpisode)
			if err != nil || !allowed {
				t.Fatalf("CanRead: got %v err %v", allowed, err)
			}
		}

		if store.listCalls.Load() != 1 {
			t.Fatalf("ListLibraries calls: got %d want 1", store.listCalls.Load())
		}
		if store.grantCalls.Load() != 1 {
			t.Fatalf("GetUserGrantMap calls: got %d want 1", store.grantCalls.Load())
		}
	})
}

func TestService_FilterBrowseResponse_SingleSnapshot(t *testing.T) {
	t.Parallel()

	allure.Test(t, "FilterBrowseResponse loads libraries and grants once", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{
			memoryStore: memoryStore{
				grants: map[string]map[string]access.LibraryPermissions{
					testUserOne: {"1": readOnlyPerms()},
				},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		response := mediafs.BrowseResponse{
			Path: "/",
			Folder: mediafs.Item{
				Path: "/",
				Children: []mediafs.Item{
					dirItem(testLibrarySeries, "/series"),
					dirItem(testLibraryFilms, "/films"),
					dirItem(testLibrarySeries, "/series/extra"),
				},
			},
		}

		filtered, err := service.FilterBrowseResponse(context.Background(), user, response)
		if err != nil {
			t.Fatalf("filter: %v", err)
		}
		if len(filtered.Folder.Children) != 2 {
			t.Fatalf("expected 2 children, got %d", len(filtered.Folder.Children))
		}
		if store.listCalls.Load() != 1 {
			t.Fatalf("ListLibraries calls: got %d want 1", store.listCalls.Load())
		}
		if store.grantCalls.Load() != 1 {
			t.Fatalf("GetUserGrantMap calls: got %d want 1", store.grantCalls.Load())
		}
	})
}

func TestService_MarkCanDelete_UsesOneSnapshot(t *testing.T) {
	t.Parallel()

	allure.Test(t, "MarkCanDelete evaluates many paths with one store load", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{
			memoryStore: memoryStore{
				grants: map[string]map[string]access.LibraryPermissions{
					testUserOne: {"2": fullPerms()},
				},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		items := []mediafs.Item{
			{
				Name: testLibraryFilms,
				Path: testPathFilms,
				Children: []mediafs.Item{
					{Name: "movie.mkv", Path: testPathFilmsMovie},
				},
			},
			{Name: "episode.mkv", Path: testPathSeriesEpisode},
		}

		enriched, err := service.MarkCanDelete(context.Background(), user, items)
		if err != nil {
			t.Fatalf("MarkCanDelete: %v", err)
		}
		if !enriched[0].Actions.CanDelete || !enriched[0].Children[0].Actions.CanDelete {
			t.Fatal("expected films paths canDelete")
		}
		if enriched[1].Actions.CanDelete {
			t.Fatal("expected series path canDelete false")
		}
		if store.listCalls.Load() != 1 || store.grantCalls.Load() != 1 {
			t.Fatalf(
				"store hits list=%d grants=%d want 1/1",
				store.listCalls.Load(),
				store.grantCalls.Load(),
			)
		}
	})

	allure.Test(t, "MarkCanDelete grants admin delete on nested items without store hits", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{memoryStore: memoryStore{}}
		service := access.NewService(store)
		admin := auth.PublicUser{ID: "admin-1", Role: auth.RoleAdmin}

		items := []mediafs.Item{
			{
				Path: testPathFilms,
				Children: []mediafs.Item{
					{Path: testPathFilmsMovie},
				},
			},
		}
		enriched, err := service.MarkCanDelete(context.Background(), admin, items)
		if err != nil {
			t.Fatalf("MarkCanDelete: %v", err)
		}
		if !enriched[0].Actions.CanDelete || !enriched[0].Children[0].Actions.CanDelete {
			t.Fatal("expected admin canDelete on all items")
		}
		if store.listCalls.Load() != 0 || store.grantCalls.Load() != 0 {
			t.Fatal("admin MarkCanDelete should not hit store")
		}
	})
}

func TestService_InvalidateACLCache(t *testing.T) {
	t.Parallel()

	allure.Test(t, "InvalidateACLCache forces library reload", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{memoryStore: memoryStore{}}
		service := access.NewService(store)

		_, err := service.ListLibraries(context.Background())
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		service.InvalidateACLCache()
		_, err = service.ListLibraries(context.Background())
		if err != nil {
			t.Fatalf("list after invalidate: %v", err)
		}
		if store.listCalls.Load() != 2 {
			t.Fatalf("list calls: got %d want 2", store.listCalls.Load())
		}
	})
}

func TestService_ACLCache_InvalidatesOnGrantChange(t *testing.T) {
	t.Parallel()

	allure.Test(t, "SetUserGrants invalidates grant cache for that user", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{
			memoryStore: memoryStore{
				grants: map[string]map[string]access.LibraryPermissions{
					testUserOne: {"1": readOnlyPerms()},
				},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		allowed, err := service.CanRead(context.Background(), user, testPathSeriesEpisode)
		if err != nil || !allowed {
			t.Fatalf("first read: got %v err %v", allowed, err)
		}

		store.grants[testUserOne] = map[string]access.LibraryPermissions{}
		err = service.SetUserGrants(context.Background(), testUserOne, auth.RoleUser, nil)
		if err != nil {
			t.Fatalf("SetUserGrants: %v", err)
		}

		allowed, err = service.CanRead(context.Background(), user, testPathSeriesEpisode)
		if err != nil {
			t.Fatalf("second read: %v", err)
		}
		if allowed {
			t.Fatal("expected read denied after grants cleared")
		}
		if store.grantCalls.Load() < 2 {
			t.Fatalf("expected grant reload after invalidate, got %d", store.grantCalls.Load())
		}
	})
}

func TestService_ACLCache_InvalidatesOnLibraryMutation(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DeleteLibrary invalidates library cache", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{
			memoryStore: memoryStore{
				grants: map[string]map[string]access.LibraryPermissions{
					testUserOne: {"1": readOnlyPerms()},
				},
			},
		}
		service := access.NewService(store)

		_, err := service.ListLibraries(context.Background())
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if store.listCalls.Load() != 1 {
			t.Fatalf("list calls after warm: %d", store.listCalls.Load())
		}

		err = service.DeleteLibrary(context.Background(), "1")
		if err != nil {
			t.Fatalf("delete: %v", err)
		}

		_, err = service.ListLibraries(context.Background())
		if err != nil {
			t.Fatalf("list after delete: %v", err)
		}
		if store.listCalls.Load() != 2 {
			t.Fatalf("expected second ListLibraries after invalidate, got %d", store.listCalls.Load())
		}
	})
}

func TestService_ACLCache_TTLExpiry(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ACL cache reloads after TTL expires", func(a *allure.Context) {
		t := a.T()
		store := &countingStore{
			memoryStore: memoryStore{
				grants: map[string]map[string]access.LibraryPermissions{
					testUserOne: {"1": readOnlyPerms()},
				},
			},
		}
		service := access.NewServiceWithCacheTTL(store, 20*time.Millisecond)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		_, err := service.CanRead(context.Background(), user, testPathSeriesEpisode)
		if err != nil {
			t.Fatalf("first: %v", err)
		}
		time.Sleep(40 * time.Millisecond)
		_, err = service.CanRead(context.Background(), user, testPathSeriesEpisode)
		if err != nil {
			t.Fatalf("second: %v", err)
		}
		if store.listCalls.Load() < 2 || store.grantCalls.Load() < 2 {
			t.Fatalf(
				"expected reload after TTL list=%d grants=%d",
				store.listCalls.Load(),
				store.grantCalls.Load(),
			)
		}
	})
}
