package access_test

import (
	"context"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseLibraryType_AcceptsKnownValues(t *testing.T) {
	t.Parallel()

	allure.Test(t, "known library types parse successfully", func(a *allure.Context) {
		t := a.T()
		for _, raw := range []string{"film", testLibrarySeries, "music", "photos", "other"} {
			libraryType, err := access.ParseLibraryType(raw)
			if err != nil {
				t.Fatalf("parse %q: %v", raw, err)
			}
			if string(libraryType) != raw {
				t.Fatalf("parse %q: got %q", raw, libraryType)
			}
		}
	})
}

func TestParseLibraryType_RejectsUnknownValues(t *testing.T) {
	t.Parallel()

	allure.Test(t, "unknown library type is rejected", func(a *allure.Context) {
		t := a.T()
		_, err := access.ParseLibraryType("podcasts")
		if err == nil {
			t.Fatal("expected error for unknown library type")
		}
	})
}

func TestService_ListReadableLibraries_FiltersByReadGrant(t *testing.T) {
	t.Parallel()

	allure.Test(t, "non-admin sees only readable libraries", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{
			grants: map[string]map[string]access.LibraryPermissions{
				testUserOne: {"1": readOnlyPerms()},
			},
		}
		service := access.NewService(store)
		user := auth.PublicUser{ //nolint:exhaustruct // test fixture
			ID:   testUserOne,
			Role: auth.RoleUser,
		}

		libraries, err := service.ListReadableLibraries(context.Background(), user)
		if err != nil {
			t.Fatalf("list readable libraries: %v", err)
		}
		if len(libraries) != 1 {
			t.Fatalf("expected 1 library, got %d", len(libraries))
		}
		if libraries[0].RelPath != testLibrarySeries {
			t.Fatalf("unexpected library: %+v", libraries[0])
		}
		if libraries[0].Type != access.LibraryTypeSeries {
			t.Fatalf("unexpected type: %q", libraries[0].Type)
		}
	})
}

func TestService_UpdateLibrary_ValidatesType(t *testing.T) {
	t.Parallel()

	allure.Test(t, "invalid library type is rejected", func(a *allure.Context) {
		t := a.T()
		service := access.NewService(
			&memoryStore{grants: map[string]map[string]access.LibraryPermissions{}},
		)

		invalid := "invalid"
		_, err := service.UpdateLibrary(
			context.Background(),
			"1",
			nil,
			&invalid,
		)
		if err == nil {
			t.Fatal("expected invalid library type error")
		}
	})
}
