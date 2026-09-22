package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sudoStream/internal/db"
	"sudoStream/internal/trash"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestStore_TrashItemCRUD(t *testing.T) { //nolint:cyclop,paralleltest
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"postgres trash store inserts, lists, and deletes items",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := setupTrashTestDatabase(ctx, t)
			store := NewStore(database.GORM)

			item := trash.Item{
				ID:              "55555555-5555-5555-5555-555555555555",
				OriginalRelPath: "movies/clip.mp4",
				TrashRelPath:    ".trash/55555555-5555-5555-5555-555555555555/movies/clip.mp4",
				DeletedAt:       time.Now().UTC().Truncate(time.Microsecond),
			}
			err := store.Insert(ctx, item)
			if err != nil {
				t.Fatalf("insert: %v", err)
			}

			got, err := store.Get(ctx, item.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got.OriginalRelPath != item.OriginalRelPath {
				t.Fatalf("got %+v", got)
			}

			listed, err := store.List(ctx)
			if err != nil || len(listed) != 1 {
				t.Fatalf("list: %v %+v", err, listed)
			}
			prefixes, err := store.OriginalPrefixes(ctx)
			if err != nil || len(prefixes) != 1 || prefixes[0] != item.OriginalRelPath {
				t.Fatalf("prefixes: %v %v", prefixes, err)
			}

			err = store.Delete(ctx, item.ID)
			if err != nil {
				t.Fatalf("delete: %v", err)
			}
			_, err = store.Get(ctx, item.ID)
			if !errors.Is(err, trash.ErrNotFound) {
				t.Fatalf("want not found, got %v", err)
			}
		},
	)
}

func setupTrashTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("rand: %v", err)
	}
	schema := "trash_it_" + hex.EncodeToString(buf)

	err = db.CreateSchema(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), schema)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { //nolint:contextcheck // cleanup runs after the test context is cancelled
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = db.DropSchema(dropCtx, os.Getenv("SUDOSTREAM_DATABASE_URL"), schema)
	})

	opts := db.PoolOptionsFromEnv()
	opts.SearchPath = schema
	database, err := db.Open(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(database.Close)

	err = db.Migrate(database.SQLDB())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return database
}
