package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sudoStream/internal/db"
	"sudoStream/internal/favorite"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testFavUserID     = "00000000-0000-0000-0000-000000000111"
	testFavLibraryID  = "00000000-0000-0000-0000-000000000211"
	testFavRelPath    = "movies/demo.mp4"
	testFavRelPathB   = "movies/other.mp4"
	testFavNestedPath = "movies/show/ep.mp4"
)

func TestFavoriteModel_TableName(t *testing.T) {
	t.Parallel()

	allure.Test(t, "favorite model maps to favorites table", func(a *allure.Context) {
		t := a.T()
		if (favoriteModel{}).TableName() != "favorites" {
			t.Fatalf("unexpected table name: %q", (favoriteModel{}).TableName())
		}
	})
}

//nolint:paralleltest,gocognit,cyclop,funlen,gocyclo // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestStore_FavoriteCRUD(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"postgres favorite store get set list delete prefix and ping",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := setupFavoriteTestDatabase(ctx, t)
			store := NewStore(database.GORM)

			favorited, err := store.Get(ctx, testFavUserID, testFavLibraryID, testFavRelPath)
			if err != nil {
				t.Fatalf("get empty: %v", err)
			}
			if favorited {
				t.Fatal("expected not favorited before set")
			}

			err = store.Set(ctx, testFavUserID, testFavLibraryID, testFavRelPath, true)
			if err != nil {
				t.Fatalf("set favorite: %v", err)
			}

			favorited, err = store.Get(ctx, testFavUserID, testFavLibraryID, testFavRelPath)
			if err != nil {
				t.Fatalf("get after set: %v", err)
			}
			if !favorited {
				t.Fatal("expected favorited after set")
			}

			err = store.Set(ctx, testFavUserID, testFavLibraryID, testFavRelPath, true)
			if err != nil {
				t.Fatalf("set favorite again: %v", err)
			}

			err = store.Set(ctx, testFavUserID, testFavLibraryID, testFavRelPathB, true)
			if err != nil {
				t.Fatalf("set second path: %v", err)
			}
			err = store.Set(ctx, testFavUserID, testFavLibraryID, testFavNestedPath, true)
			if err != nil {
				t.Fatalf("set nested path: %v", err)
			}

			flags, err := store.ListForPaths(
				ctx,
				testFavUserID,
				testFavLibraryID,
				[]string{testFavRelPath, testFavRelPathB, "movies/missing.mp4"},
			)
			if err != nil {
				t.Fatalf("list for paths: %v", err)
			}
			if !flags[testFavRelPath] || !flags[testFavRelPathB] {
				t.Fatalf("unexpected flags: %#v", flags)
			}
			if flags["movies/missing.mp4"] {
				t.Fatal("missing path should not be favorited")
			}

			empty, err := store.ListForPaths(ctx, testFavUserID, testFavLibraryID, nil)
			if err != nil {
				t.Fatalf("list empty paths: %v", err)
			}
			if len(empty) != 0 {
				t.Fatalf("expected empty map, got %#v", empty)
			}

			emptyIDs, err := store.ListForUser(ctx, testFavUserID, nil, 10, 0)
			if err != nil || len(emptyIDs) != 0 {
				t.Fatalf("list empty libraries: %#v err=%v", emptyIDs, err)
			}

			rows, err := store.ListForUser(ctx, testFavUserID, []string{testFavLibraryID}, 10, 0)
			if err != nil || len(rows) != 3 {
				t.Fatalf("list for user: n=%d err=%v", len(rows), err)
			}
			page, err := store.ListForUser(ctx, testFavUserID, []string{testFavLibraryID}, 1, 1)
			if err != nil || len(page) != 1 {
				t.Fatalf("list offset: n=%d err=%v", len(page), err)
			}
			total, err := store.CountForUser(ctx, testFavUserID, []string{testFavLibraryID})
			if err != nil || total != 3 {
				t.Fatalf("count for user: %d err=%v", total, err)
			}

			err = store.Set(ctx, testFavUserID, testFavLibraryID, testFavRelPath, false)
			if err != nil {
				t.Fatalf("clear favorite: %v", err)
			}

			favorited, err = store.Get(ctx, testFavUserID, testFavLibraryID, testFavRelPath)
			if err != nil {
				t.Fatalf("get after clear: %v", err)
			}
			if favorited {
				t.Fatal("expected not favorited after clear")
			}

			err = store.DeleteForPath(ctx, testFavLibraryID, "movies/show")
			if err != nil {
				t.Fatalf("delete prefix: %v", err)
			}
			nested, err := store.Get(ctx, testFavUserID, testFavLibraryID, testFavNestedPath)
			if err != nil || nested {
				t.Fatalf("expected nested path cleared, got %v err=%v", nested, err)
			}

			err = store.DeleteForPath(ctx, testFavLibraryID, testFavRelPathB)
			if err != nil {
				t.Fatalf("delete for path: %v", err)
			}

			err = store.Ping(ctx)
			if err != nil {
				t.Fatalf("ping: %v", err)
			}
		},
	)
}

func TestStore_PingUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil favorite store reports unavailable", func(a *allure.Context) {
		t := a.T()
		var store *Store

		err := store.Ping(context.Background())
		if !errors.Is(err, favorite.ErrStoreUnavailable) {
			t.Fatalf("expected ErrStoreUnavailable, got %v", err)
		}

		empty := &Store{}
		err = empty.Ping(context.Background())
		if !errors.Is(err, favorite.ErrStoreUnavailable) {
			t.Fatalf("expected ErrStoreUnavailable for nil db, got %v", err)
		}
	})
}

func setupFavoriteTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	schema := provisionFavoriteSchema(ctx, t)

	opts := db.PoolOptionsFromEnv()
	opts.SearchPath = schema

	database, err := db.Open(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), opts)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(database.Close)

	err = db.Migrate(database.SQLDB())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, role, enabled, must_change_password)
		VALUES ($1, 'favorite-store-test@example.com', 'hash', 'user', TRUE, FALSE)
	`, testFavUserID)
	if err != nil {
		t.Fatalf("seed favorite store test user: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO libraries (id, slug, name, type)
		VALUES ($1, 'movies', 'Movies', 'film')
	`, testFavLibraryID)
	if err != nil {
		t.Fatalf("seed favorite store test library: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO library_roots (library_id, rel_path)
		VALUES ($1, 'movies')
	`, testFavLibraryID)
	if err != nil {
		t.Fatalf("seed favorite store test library root: %v", err)
	}

	return database
}

func provisionFavoriteSchema(ctx context.Context, t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := "fav_it_" + hex.EncodeToString(buf)

	err = db.CreateSchema(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), schema)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	//nolint:contextcheck // cleanup runs after the test context is cancelled
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dropErr := db.DropSchema(dropCtx, os.Getenv("SUDOSTREAM_DATABASE_URL"), schema)
		if dropErr != nil {
			t.Errorf("drop schema %s: %v", schema, dropErr)
		}
	})

	return schema
}
