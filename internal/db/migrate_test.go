package db_test

import (
	"context"
	"os"
	"sudoStream/internal/db"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestMigrate_AppliesAndIsIdempotent(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "migrations apply and re-running is a no-op", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		database, err := db.Open(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), db.PoolOptionsFromEnv())
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		t.Cleanup(database.Close)

		err = db.Migrate(database.SQLDB())
		if err != nil {
			t.Fatalf("first migrate: %v", err)
		}

		err = db.Migrate(database.SQLDB())
		if err != nil {
			t.Fatalf("second migrate should be idempotent: %v", err)
		}

		var version int64
		err = database.SQLDB().QueryRowContext(
			ctx,
			`SELECT max(version_id) FROM goose_db_version`,
		).Scan(&version)
		if err != nil {
			t.Fatalf("read goose version: %v", err)
		}
		if version <= 0 {
			t.Fatalf("expected applied migration version, got %d", version)
		}
	})
}
