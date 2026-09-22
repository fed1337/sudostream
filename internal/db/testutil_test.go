package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sudoStream/internal/db"
	"testing"
	"time"
)

func requireDatabaseURL(t *testing.T) string {
	t.Helper()

	databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	return databaseURL
}

func uniqueSchemaPrefix(prefix string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	return prefix + hex.EncodeToString(buf)
}

func openIsolatedDatabase(
	ctx context.Context,
	t *testing.T,
	prefix string,
) (*db.Database, string) {
	t.Helper()

	databaseURL := requireDatabaseURL(t)
	schema := uniqueSchemaPrefix(prefix)

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

	return database, schema
}
