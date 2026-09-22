package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sudoStream/internal/access"
	accesspostgres "sudoStream/internal/access/postgres"
	"sudoStream/internal/auth"
	authpostgres "sudoStream/internal/auth/postgres"
	"sudoStream/internal/db"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const testMoviesSlug = "movies"

//nolint:paralleltest,gocognit,cyclop // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestStore_LibrariesAndGrants(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "postgres access store manages libraries and grants", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		database := openAccessTestDatabase(ctx, t)
		authStore := authpostgres.NewStore(database.GORM)
		store := accesspostgres.NewStore(database.GORM)

		err := authStore.CreateUser(ctx, auth.User{
			Email:              "access-store@example.com",
			Role:               auth.RoleUser,
			Enabled:            true,
			MustChangePassword: false,
		}, "hash")
		if err != nil {
			t.Fatalf("create user: %v", err)
		}

		user, _, err := authStore.GetUserByEmail(ctx, "access-store@example.com")
		if err != nil {
			t.Fatalf("get user: %v", err)
		}

		library, err := store.UpsertLibrary(ctx, access.Library{
			Slug:    testMoviesSlug,
			RelPath: testMoviesSlug,
			Name:    "Movies",
			Type:    access.LibraryTypeOther,
		})
		if err != nil {
			t.Fatalf("upsert library: %v", err)
		}

		updated, err := store.UpsertLibrary(ctx, access.Library{
			Slug:    testMoviesSlug,
			RelPath: testMoviesSlug,
			Name:    "Movies Updated",
			Type:    access.LibraryTypeSeries,
		})
		if err != nil {
			t.Fatalf("re-upsert library: %v", err)
		}
		if updated.Name != "Movies" {
			t.Fatalf("unexpected library name: %q", updated.Name)
		}
		if updated.Type != access.LibraryTypeOther {
			t.Fatalf("expected existing type to be preserved, got %q", updated.Type)
		}

		displayName := "Movies Updated"
		libraryType := access.LibraryTypeSeries
		typed, err := store.UpdateLibrary(ctx, library.ID, &displayName, &libraryType)
		if err != nil {
			t.Fatalf("update library type: %v", err)
		}
		if typed.Type != access.LibraryTypeSeries || typed.Name != displayName {
			t.Fatalf("unexpected library after update: %#v", typed)
		}

		libraries, err := store.ListLibraries(ctx)
		if err != nil {
			t.Fatalf("list libraries: %v", err)
		}
		if len(libraries) != 1 {
			t.Fatalf("expected one library, got %d", len(libraries))
		}

		_, err = store.GetLibrary(ctx, library.ID)
		if err != nil {
			t.Fatalf("get library: %v", err)
		}

		err = store.ReplaceUserGrants(ctx, user.ID, []access.GrantInput{{
			LibraryID: library.ID,
			Permissions: access.LibraryPermissions{
				Read:   true,
				Update: true,
			},
		}})
		if err != nil {
			t.Fatalf("replace grants: %v", err)
		}

		grants, err := store.GetUserGrantMap(ctx, user.ID)
		if err != nil {
			t.Fatalf("get grants: %v", err)
		}
		if !grants[library.ID].Read || !grants[library.ID].Update {
			t.Fatalf("unexpected grants: %#v", grants)
		}

		_, err = store.GetLibrary(ctx, "00000000-0000-0000-0000-000000000099")
		if !errors.Is(err, access.ErrLibraryNotFound) {
			t.Fatalf("expected library not found, got %v", err)
		}
	})
}

const testFriendsSlug = "friends"

func TestStore_AddRootMergesAndReassigns( //nolint:cyclop,paralleltest // merge scenario
	t *testing.T,
) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"moving friends into Series reassigns metadata and deletes the old library",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := openAccessTestDatabase(ctx, t)
			store := accesspostgres.NewStore(database.GORM)

			friends, err := store.UpsertLibrary(ctx, access.Library{
				Slug:    testFriendsSlug,
				RelPath: testFriendsSlug,
				Name:    "Friends",
				Type:    access.LibraryTypeSeries,
			})
			if err != nil {
				t.Fatalf("upsert friends: %v", err)
			}

			series, err := store.CreateLibrary(ctx, access.Library{
				Slug: "series",
				Name: "Series",
				Type: access.LibraryTypeSeries,
			})
			if err != nil {
				t.Fatalf("create series: %v", err)
			}

			err = database.GORM.Exec(
				`INSERT INTO media_metadata (library_id, rel_path, original_fields, override_fields)
			 VALUES (?, 'friends/S01E01.mkv', '{}', '{}')`,
				friends.ID,
			).Error
			if err != nil {
				t.Fatalf("insert metadata: %v", err)
			}

			merged, err := store.AddRoot(ctx, series.ID, testFriendsSlug)
			if err != nil {
				t.Fatalf("add root: %v", err)
			}
			if len(merged.Roots) != 1 || merged.Roots[0] != testFriendsSlug {
				t.Fatalf("merged roots=%v", merged.Roots)
			}

			_, err = store.GetLibrary(ctx, friends.ID)
			if !errors.Is(err, access.ErrLibraryNotFound) {
				t.Fatalf("expected friends library gone, got %v", err)
			}

			var owner string
			err = database.GORM.Raw(
				`SELECT library_id FROM media_metadata WHERE rel_path = ?`,
				"friends/S01E01.mkv",
			).Scan(&owner).Error
			if err != nil {
				t.Fatalf("lookup metadata: %v", err)
			}
			if owner != series.ID {
				t.Fatalf("metadata library_id=%q want %q", owner, series.ID)
			}
		},
	)
}

func TestStore_RemoveRootAndDeleteLibrary( //nolint:cyclop,paralleltest // integration root lifecycle
	t *testing.T,
) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"remove root drops prefix metadata; last root deletes the library",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := openAccessTestDatabase(ctx, t)
			store := accesspostgres.NewStore(database.GORM)

			library, err := store.CreateLibrary(ctx, access.Library{
				Slug: "series",
				Name: "Series",
				Type: access.LibraryTypeSeries,
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}

			_, err = store.AddRoot(ctx, library.ID, testMoviesSlug)
			if err != nil {
				t.Fatalf("add movies: %v", err)
			}
			_, err = store.AddRoot(ctx, library.ID, testFriendsSlug)
			if err != nil {
				t.Fatalf("add friends: %v", err)
			}

			err = database.GORM.Exec(
				`INSERT INTO media_metadata (library_id, rel_path, original_fields, override_fields)
				 VALUES (?, 'friends/S01E01.mkv', '{}', '{}')`,
				library.ID,
			).Error
			if err != nil {
				t.Fatalf("insert metadata: %v", err)
			}

			kept, err := store.RemoveRoot(ctx, library.ID, testFriendsSlug)
			if err != nil {
				t.Fatalf("remove friends: %v", err)
			}
			if len(kept.Roots) != 1 || kept.Roots[0] != testMoviesSlug {
				t.Fatalf("roots=%v", kept.Roots)
			}

			var leftover int64
			err = database.GORM.Raw(
				`SELECT count(*) FROM media_metadata WHERE rel_path LIKE 'friends%'`,
			).Scan(&leftover).Error
			if err != nil {
				t.Fatalf("count metadata: %v", err)
			}
			if leftover != 0 {
				t.Fatalf("expected prefix metadata gone, got %d", leftover)
			}

			_, err = store.RemoveRoot(ctx, library.ID, testMoviesSlug)
			if !errors.Is(err, access.ErrLibraryNotFound) {
				t.Fatalf("last root: %v", err)
			}

			err = store.DeleteLibrary(ctx, library.ID)
			if !errors.Is(err, access.ErrLibraryNotFound) {
				t.Fatalf("delete gone library: %v", err)
			}

			other, err := store.CreateLibrary(ctx, access.Library{
				Slug: "other",
				Name: "Other",
				Type: access.LibraryTypeOther,
			})
			if err != nil {
				t.Fatalf("create other: %v", err)
			}
			err = store.DeleteLibrary(ctx, other.ID)
			if err != nil {
				t.Fatalf("delete other: %v", err)
			}
		},
	)
}

func openAccessTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
	schema := uniqueAccessSchema()

	err := db.CreateSchema(ctx, databaseURL, schema)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	//nolint:contextcheck // cleanup runs after the test context is cancelled
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dropErr := db.DropSchema(dropCtx, databaseURL, schema)
		if dropErr != nil {
			t.Errorf("drop schema %s: %v", schema, dropErr)
		}
	})

	opts := db.PoolOptionsFromEnv()
	opts.SearchPath = schema

	database, err := db.Open(ctx, databaseURL, opts)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(database.Close)

	err = db.Migrate(database.SQLDB())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return database
}

func uniqueAccessSchema() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	return "access_it_" + hex.EncodeToString(buf)
}
