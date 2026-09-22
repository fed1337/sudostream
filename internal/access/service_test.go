package access_test

import (
	"context"
	"errors"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testLibrarySeries     = "series"
	testLibraryFilms      = "films"
	testUserOne           = "user-1"
	testPathSeriesEpisode = "/series/episode.mkv"
	testPathFilmsMovie    = "/films/movie.mkv"
	testPathFilms         = "/films"
	testPermCreate        = "create"
	testPermUpdate        = "update"
	testPermDelete        = "delete"
	testPermRead          = "read"
	testRootName          = "root"
)

type memoryStore struct {
	grants map[string]map[string]access.LibraryPermissions
}

func (m *memoryStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return []access.Library{
		{
			ID: "1", Slug: testLibrarySeries, RelPath: testLibrarySeries,
			Name: testLibrarySeries, Type: access.LibraryTypeSeries,
		},
		{
			ID: "2", Slug: testLibraryFilms, RelPath: testLibraryFilms,
			Name: testLibraryFilms, Type: access.LibraryTypeFilm,
		},
	}, nil
}

func (m *memoryStore) GetLibrary(_ context.Context, libraryID string) (access.Library, error) {
	for _, library := range []access.Library{
		{
			ID: "1", Slug: testLibrarySeries, RelPath: testLibrarySeries,
			Name: testLibrarySeries, Type: access.LibraryTypeSeries,
		},
		{
			ID: "2", Slug: testLibraryFilms, RelPath: testLibraryFilms,
			Name: testLibraryFilms, Type: access.LibraryTypeFilm,
		},
	} {
		if library.ID == libraryID {
			return library, nil
		}
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (m *memoryStore) CreateLibrary(
	_ context.Context,
	library access.Library,
) (access.Library, error) {
	if library.Type == "" {
		library.Type = access.LibraryTypeOther
	}
	if len(library.Roots) == 0 && library.RelPath != "" {
		library.Roots = []string{library.RelPath}
	}

	return library, nil
}

func (m *memoryStore) UpsertLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return m.CreateLibrary(ctx, library)
}

func (m *memoryStore) AddRoot(
	ctx context.Context,
	libraryID, relPath string,
) (access.Library, error) {
	library, err := m.GetLibrary(ctx, libraryID)
	if err != nil {
		return access.Library{}, err
	}
	library.Roots = append(library.RootPaths(), relPath)
	library.RelPath = library.Roots[0]

	return library, nil
}

func (m *memoryStore) RemoveRoot(
	ctx context.Context,
	libraryID, relPath string,
) (access.Library, error) {
	library, err := m.GetLibrary(ctx, libraryID)
	if err != nil {
		return access.Library{}, err
	}
	kept := make([]string, 0, len(library.RootPaths()))
	for _, root := range library.RootPaths() {
		if root != relPath {
			kept = append(kept, root)
		}
	}
	library.Roots = kept
	if len(kept) == 0 {
		return access.Library{}, access.ErrLibraryNotFound
	}
	library.RelPath = kept[0]

	return library, nil
}

func (m *memoryStore) DeleteLibrary(ctx context.Context, libraryID string) error {
	_, err := m.GetLibrary(ctx, libraryID)

	return err
}

func (m *memoryStore) UpdateLibrary(
	_ context.Context,
	libraryID string,
	name *string,
	libraryType *access.LibraryType,
) (access.Library, error) {
	for _, library := range []access.Library{
		{
			ID: "1", Slug: testLibrarySeries, RelPath: testLibrarySeries,
			Name: testLibrarySeries, Type: access.LibraryTypeSeries,
		},
		{
			ID: "2", Slug: testLibraryFilms, RelPath: testLibraryFilms,
			Name: testLibraryFilms, Type: access.LibraryTypeFilm,
		},
	} {
		if library.ID == libraryID {
			if name != nil {
				library.Name = *name
			}
			if libraryType != nil {
				library.Type = *libraryType
			}

			return library, nil
		}
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (m *memoryStore) GetUserGrantMap(
	_ context.Context,
	userID string,
) (map[string]access.LibraryPermissions, error) {
	return m.grants[userID], nil
}

func (m *memoryStore) ReplaceUserGrants(_ context.Context, _ string, _ []access.GrantInput) error {
	return nil
}

func readOnlyPerms() access.LibraryPermissions {
	return access.LibraryPermissions{Read: true}
}

func fullPerms() access.LibraryPermissions {
	return access.LibraryPermissions{Create: true, Read: true, Update: true, Delete: true}
}

func deleteOnlyPerms() access.LibraryPermissions {
	return access.LibraryPermissions{Delete: true}
}

func TestService_CanRead_RespectsGrants(t *testing.T) {
	t.Parallel()

	allure.Test(t, "user can read only granted libraries", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {
					"1": readOnlyPerms(),
				},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{
			ID:                 testUserOne,
			Email:              "user@hpserver.lan",
			Role:               auth.RoleUser,
			MustChangePassword: false,
		}

		allowedSeries, err := service.CanRead(context.Background(), user, testPathSeriesEpisode)
		if err != nil {
			t.Fatalf("series access: %v", err)
		}
		if !allowedSeries {
			t.Fatal("expected series access")
		}

		allowedFilms, err := service.CanRead(context.Background(), user, testPathFilmsMovie)
		if err != nil {
			t.Fatalf("films access: %v", err)
		}
		if allowedFilms {
			t.Fatal("expected films access denied")
		}
	})
}

func TestService_ReadOnlyGrantBlocksWrites(t *testing.T) {
	t.Parallel()

	allure.Test(t, "read-only grant allows read but not writes", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"1": readOnlyPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}
		path := testPathSeriesEpisode

		canRead, err := service.CanRead(context.Background(), user, path)
		if err != nil || !canRead {
			t.Fatalf("read: got %v err %v", canRead, err)
		}

		for _, check := range []struct {
			name string
			fn   func(context.Context, auth.PublicUser, string) (bool, error)
		}{
			{testPermCreate, service.CanCreate},
			{testPermUpdate, service.CanUpdate},
			{testPermDelete, service.CanDelete},
		} {
			allowed, err := check.fn(context.Background(), user, path)
			if err != nil {
				t.Fatalf("%s: %v", check.name, err)
			}
			if allowed {
				t.Fatalf("expected %s denied", check.name)
			}
		}
	})
}

func TestService_FullGrantAllowsAllOperations(t *testing.T) {
	t.Parallel()

	allure.Test(t, "full grant allows all operations", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"1": fullPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}
		path := testPathSeriesEpisode

		for _, check := range []struct {
			name string
			fn   func(context.Context, auth.PublicUser, string) (bool, error)
		}{
			{testPermRead, service.CanRead},
			{testPermCreate, service.CanCreate},
			{testPermUpdate, service.CanUpdate},
			{testPermDelete, service.CanDelete},
		} {
			allowed, err := check.fn(context.Background(), user, path)
			if err != nil || !allowed {
				t.Fatalf("%s: got %v err %v", check.name, allowed, err)
			}
		}
	})
}

func TestService_DeleteOnlyGrantAllowsDeleteNotRead(t *testing.T) {
	t.Parallel()

	allure.Test(t, "delete-only grant allows delete but not read", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"2": deleteOnlyPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}
		path := testPathFilmsMovie

		canDelete, err := service.CanDelete(context.Background(), user, path)
		if err != nil || !canDelete {
			t.Fatalf("delete: got %v err %v", canDelete, err)
		}

		canRead, err := service.CanRead(context.Background(), user, path)
		if err != nil || canRead {
			t.Fatalf("read: got %v err %v", canRead, err)
		}
	})
}

func TestService_AdminBypassesACL(t *testing.T) {
	t.Parallel()

	allure.Test(t, "admin can access without grants", func(a *allure.Context) {
		t := a.T()
		service := access.NewService(
			&memoryStore{grants: map[string]map[string]access.LibraryPermissions{}},
		)
		admin := auth.PublicUser{ID: "admin-1", Role: auth.RoleAdmin}

		allowed, err := service.CanRead(context.Background(), admin, testPathFilmsMovie)
		if err != nil || !allowed {
			t.Fatalf("admin read: got %v err %v", allowed, err)
		}
	})
}

func TestService_CanBrowse_AllowsMediaRoot(t *testing.T) {
	t.Parallel()

	allure.Test(t, "non-admin can browse media root", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"1": readOnlyPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		allowed, err := service.CanBrowse(context.Background(), user, "/")
		if err != nil || !allowed {
			t.Fatalf("root browse: got %v err %v", allowed, err)
		}
	})
}

//nolint:cyclop,funlen // table-driven browse filtering scenarios
func TestService_FilterBrowseResponse_HidesDeniedChildren(t *testing.T) {
	t.Parallel()

	allure.Test(t, "root children filtered by can_read only", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"1": readOnlyPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		response := mediafs.BrowseResponse{
			Path: "/",
			Breadcrumbs: []mediafs.Breadcrumb{
				{Name: testRootName, Path: "/"},
			},
			Folder: mediafs.Item{
				Name:     "root",
				Path:     "/",
				IsDir:    true,
				MimeType: directoryMimeType(),
				Actions:  emptyActions(),
				Children: []mediafs.Item{
					dirItem(testLibrarySeries, "/series"),
					dirItem(testLibraryFilms, "/films"),
				},
			},
		}

		filtered, err := service.FilterBrowseResponse(context.Background(), user, response)
		if err != nil {
			t.Fatalf("filter browse: %v", err)
		}
		if len(filtered.Folder.Children) != 1 {
			t.Fatalf("expected 1 child, got %d", len(filtered.Folder.Children))
		}
		if filtered.Folder.Children[0].Name != testLibrarySeries {
			t.Fatalf("unexpected child: %+v", filtered.Folder.Children[0])
		}
	})

	allure.Test(t, "user with no read grants sees empty root browse", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"2": deleteOnlyPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		allowed, err := service.CanBrowse(context.Background(), user, "/")
		if err != nil || !allowed {
			t.Fatalf("root browse: got %v err %v", allowed, err)
		}

		rootResponse := mediafs.BrowseResponse{
			Path: "/",
			Folder: mediafs.Item{
				Path: "/",
				Children: []mediafs.Item{
					dirItem(testLibraryFilms, "/films"),
					dirItem(testLibrarySeries, "/series"),
				},
			},
		}

		filtered, err := service.FilterBrowseResponse(context.Background(), user, rootResponse)
		if err != nil {
			t.Fatalf("filter root: %v", err)
		}
		if len(filtered.Folder.Children) != 0 {
			t.Fatalf("expected empty root, got %d children", len(filtered.Folder.Children))
		}
	})

	allure.Test(t, "delete-only library hidden at root and inside browse", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"2": deleteOnlyPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ID: testUserOne, Role: auth.RoleUser}

		rootResponse := mediafs.BrowseResponse{
			Path: "/",
			Folder: mediafs.Item{
				Path: "/",
				Children: []mediafs.Item{
					dirItem(testLibraryFilms, "/films"),
				},
			},
		}

		filteredRoot, err := service.FilterBrowseResponse(
			context.Background(),
			user,
			rootResponse,
		)
		if err != nil {
			t.Fatalf("filter root: %v", err)
		}
		if len(filteredRoot.Folder.Children) != 0 {
			t.Fatal("expected delete-only library hidden at root")
		}

		insideResponse := mediafs.BrowseResponse{
			Path: testPathFilms,
			Folder: mediafs.Item{
				Path: testPathFilms,
				Children: []mediafs.Item{
					{Name: "movie.mkv", Path: "/films/movie.mkv", IsDir: false},
				},
			},
		}

		filteredInside, err := service.FilterBrowseResponse(
			context.Background(),
			user,
			insideResponse,
		)
		if err != nil {
			t.Fatalf("filter inside: %v", err)
		}
		if len(filteredInside.Folder.Children) != 0 {
			t.Fatal("expected no readable children inside delete-only library")
		}
	},
	)
}

func TestService_LibraryForRelPath(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"friends episode resolves to Series when that library owns the friends root",
		func(a *allure.Context) {
			t := a.T()
			service := access.NewService(&memoryStore{})

			library, matched, err := service.LibraryForRelPath(
				context.Background(),
				testPathSeriesEpisode,
			)
			if err != nil || !matched || library.ID != "1" {
				t.Fatalf("series path: library=%+v matched=%v err=%v", library, matched, err)
			}

			_, matched, err = service.LibraryForRelPath(context.Background(), "dumps/file.mkv")
			if err != nil || matched {
				t.Fatalf("unassigned path: matched=%v err=%v", matched, err)
			}
		},
	)
}

func TestService_RemoveRootAndDeleteLibrary(t *testing.T) {
	t.Parallel()

	allure.Test(t, "remove root and delete library go through the store", func(a *allure.Context) {
		t := a.T()
		service := access.NewService(&memoryStore{})

		_, err := service.RemoveRoot(context.Background(), "1", "")
		if !errors.Is(err, access.ErrInvalidLibraryRoot) {
			t.Fatalf("empty root: %v", err)
		}

		_, err = service.RemoveRoot(context.Background(), "1", testLibrarySeries)
		if !errors.Is(err, access.ErrLibraryNotFound) {
			t.Fatalf("last root: %v", err)
		}

		err = service.DeleteLibrary(context.Background(), "1")
		if err != nil {
			t.Fatalf("delete: %v", err)
		}

		err = service.DeleteLibrary(context.Background(), "missing")
		if !errors.Is(err, access.ErrLibraryNotFound) {
			t.Fatalf("missing delete: %v", err)
		}
	})
}

func TestService_AddRoot_RejectsOverlap(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"adding a descendant of another library root is rejected",
		func(a *allure.Context) {
			t := a.T()
			service := access.NewService(&memoryStore{})

			_, err := service.AddRoot(context.Background(), "1", "films/extra")
			if !errors.Is(err, access.ErrLibraryRootConflict) {
				t.Fatalf("expected conflict, got %v", err)
			}

			library, err := service.AddRoot(context.Background(), "1", testLibrarySeries)
			if err != nil {
				t.Fatalf("same root should be idempotent: %v", err)
			}
			if library.ID != "1" {
				t.Fatalf("library=%+v", library)
			}
		},
	)
}

func TestService_FilterBrowseResponse_HidesUnassigned(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"unassigned folders are hidden from browse including admins",
		func(a *allure.Context) {
			t := a.T()
			service := access.NewService(&memoryStore{})
			admin := auth.PublicUser{ID: "admin", Role: auth.RoleAdmin}
			response := mediafs.BrowseResponse{
				Path: "/",
				Folder: mediafs.Item{
					Path: "/",
					Children: []mediafs.Item{
						dirItem(testLibrarySeries, "/series"),
						dirItem("dumps", "/dumps"),
					},
				},
			}

			filtered, err := service.FilterBrowseResponse(context.Background(), admin, response)
			if err != nil {
				t.Fatalf("filter: %v", err)
			}
			if len(filtered.Folder.Children) != 1 ||
				filtered.Folder.Children[0].Name != testLibrarySeries {
				t.Fatalf("children=%+v", filtered.Folder.Children)
			}
		},
	)
}

func directoryMimeType() string {
	return "inode/directory"
}

func emptyActions() mediafs.Action {
	return mediafs.Action{Play: "", Thumbnail: "", Download: ""}
}

func dirItem(name, path string) mediafs.Item {
	return mediafs.Item{
		Name:     name,
		Path:     path,
		IsDir:    true,
		MimeType: directoryMimeType(),
		Actions:  emptyActions(),
		Children: nil,
	}
}
