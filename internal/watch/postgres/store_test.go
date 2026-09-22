package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sudoStream/internal/db"
	"sudoStream/internal/watch"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testWatchUserID    = "00000000-0000-0000-0000-000000000101"
	testWatchLibraryID = "00000000-0000-0000-0000-000000000201"
	testWatchRelPath   = "movies/demo.mp4"
	testWatchRelPathB  = "movies/other.mp4"
)

func TestWatchStateModel_TableName(t *testing.T) {
	t.Parallel()

	allure.Test(t, "watch state model maps to watch_state table", func(a *allure.Context) {
		t := a.T()
		if (watchStateModel{}).TableName() != "watch_state" {
			t.Fatalf("unexpected table name: %q", (watchStateModel{}).TableName())
		}
	})
}

//nolint:paralleltest,gocognit,cyclop,funlen,gocyclo // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestStore_WatchCRUD(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "postgres watch store get set list delete and ping", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		database := setupWatchTestDatabase(ctx, t)
		store := NewStore(database.GORM)

		watched, err := store.Get(ctx, testWatchUserID, testWatchLibraryID, testWatchRelPath)
		if err != nil {
			t.Fatalf("get empty: %v", err)
		}
		if watched.Watched {
			t.Fatal("expected unwatched before set")
		}

		err = store.Upsert(
			ctx,
			testWatchUserID,
			testWatchLibraryID,
			testWatchRelPath,
			watch.State{Watched: true},
		)
		if err != nil {
			t.Fatalf("set watched: %v", err)
		}

		watched, err = store.Get(ctx, testWatchUserID, testWatchLibraryID, testWatchRelPath)
		if err != nil {
			t.Fatalf("get after set: %v", err)
		}
		if !watched.Watched {
			t.Fatal("expected watched after set")
		}

		// Upsert again should succeed (OnConflict update).
		err = store.Upsert(
			ctx,
			testWatchUserID,
			testWatchLibraryID,
			testWatchRelPath,
			watch.State{Watched: true},
		)
		if err != nil {
			t.Fatalf("set watched again: %v", err)
		}

		err = store.Upsert(
			ctx,
			testWatchUserID,
			testWatchLibraryID,
			testWatchRelPathB,
			watch.State{Watched: true},
		)
		if err != nil {
			t.Fatalf("set second path: %v", err)
		}

		flags, err := store.ListForPaths(
			ctx,
			testWatchUserID,
			testWatchLibraryID,
			[]string{testWatchRelPath, testWatchRelPathB, "movies/missing.mp4"},
		)
		if err != nil {
			t.Fatalf("list for paths: %v", err)
		}
		if !flags[testWatchRelPath] || !flags[testWatchRelPathB] {
			t.Fatalf("unexpected flags: %#v", flags)
		}
		if flags["movies/missing.mp4"] {
			t.Fatal("missing path should not be watched")
		}

		empty, err := store.ListForPaths(ctx, testWatchUserID, testWatchLibraryID, nil)
		if err != nil {
			t.Fatalf("list empty paths: %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("expected empty map, got %#v", empty)
		}

		err = store.Clear(ctx, testWatchUserID, testWatchLibraryID, testWatchRelPath)
		if err != nil {
			t.Fatalf("clear watched: %v", err)
		}

		watched, err = store.Get(ctx, testWatchUserID, testWatchLibraryID, testWatchRelPath)
		if err != nil {
			t.Fatalf("get after clear: %v", err)
		}
		if watched.Watched {
			t.Fatal("expected unwatched after clear")
		}

		err = store.DeleteForPath(ctx, testWatchLibraryID, testWatchRelPathB)
		if err != nil {
			t.Fatalf("delete for path: %v", err)
		}

		flags, err = store.ListForPaths(
			ctx,
			testWatchUserID,
			testWatchLibraryID,
			[]string{testWatchRelPathB},
		)
		if err != nil {
			t.Fatalf("list after delete: %v", err)
		}
		if flags[testWatchRelPathB] {
			t.Fatal("expected path cleared by delete")
		}

		position := 42.0
		duration := 100.0
		err = store.Upsert(ctx, testWatchUserID, testWatchLibraryID, testWatchRelPath, watch.State{
			PositionSeconds: &position,
			DurationSeconds: &duration,
		})
		if err != nil {
			t.Fatalf("upsert progress: %v", err)
		}
		progress, err := store.Get(ctx, testWatchUserID, testWatchLibraryID, testWatchRelPath)
		if err != nil || progress.Watched || progress.PositionSeconds == nil ||
			*progress.PositionSeconds != position {
			t.Fatalf("progress get: %#v err=%v", progress, err)
		}
		continueRows, err := store.ListContinue(
			ctx, testWatchUserID, []string{testWatchLibraryID}, 10, 0,
		)
		if err != nil || len(continueRows) != 1 || continueRows[0].RelPath != testWatchRelPath {
			t.Fatalf("list continue: %#v err=%v", continueRows, err)
		}
		continueCount, err := store.CountContinue(
			ctx,
			testWatchUserID,
			[]string{testWatchLibraryID},
		)
		if err != nil || continueCount != 1 {
			t.Fatalf("count continue: %d err=%v", continueCount, err)
		}
		watchedRows, err := store.ListForUser(
			ctx,
			testWatchUserID,
			[]string{testWatchLibraryID},
			10,
			0,
		)
		if err != nil || len(watchedRows) != 0 {
			t.Fatalf("in-progress must not be on watched shelf: %#v err=%v", watchedRows, err)
		}

		err = store.Ping(ctx)
		if err != nil {
			t.Fatalf("ping: %v", err)
		}
	})
}

func TestStore_PingUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil store reports unavailable", func(a *allure.Context) {
		t := a.T()
		var store *Store

		err := store.Ping(context.Background())
		if !errors.Is(err, watch.ErrStoreUnavailable) {
			t.Fatalf("expected ErrStoreUnavailable, got %v", err)
		}

		empty := &Store{}
		err = empty.Ping(context.Background())
		if !errors.Is(err, watch.ErrStoreUnavailable) {
			t.Fatalf("expected ErrStoreUnavailable for nil db, got %v", err)
		}
	})
}

func setupWatchTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	schema := provisionWatchSchema(ctx, t)

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
		VALUES ($1, 'watch-store-test@example.com', 'hash', 'user', TRUE, FALSE)
	`, testWatchUserID)
	if err != nil {
		t.Fatalf("seed watch store test user: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO libraries (id, slug, name, type)
		VALUES ($1, 'movies', 'Movies', 'film')
	`, testWatchLibraryID)
	if err != nil {
		t.Fatalf("seed watch store test library: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO library_roots (library_id, rel_path)
		VALUES ($1, 'movies')
	`, testWatchLibraryID)
	if err != nil {
		t.Fatalf("seed watch store test library root: %v", err)
	}

	return database
}

func provisionWatchSchema(ctx context.Context, t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := "watch_it_" + hex.EncodeToString(buf)

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
