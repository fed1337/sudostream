package db_test

import (
	"context"
	"os"
	"sudoStream/internal/db"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestPoolOptionsFromEnv_Defaults(t *testing.T) {
	allure.Test(t, "pool options use defaults when env unset", func(a *allure.Context) {
		t := a.T()
		t.Setenv("SUDOSTREAM_DB_MAX_CONNS", "")
		t.Setenv("SUDOSTREAM_DB_MIN_CONNS", "")

		opts := db.PoolOptionsFromEnv()
		if opts.MaxOpenConns != 10 {
			t.Fatalf("MaxOpenConns: got %d want 10", opts.MaxOpenConns)
		}
		if opts.MaxIdleConns != 2 {
			t.Fatalf("MaxIdleConns: got %d want 2", opts.MaxIdleConns)
		}
		if opts.ConnMaxLifetime != 30*time.Minute {
			t.Fatalf("ConnMaxLifetime: got %v", opts.ConnMaxLifetime)
		}
	})
}

func TestPoolOptionsFromEnv_CustomValues(t *testing.T) {
	allure.Test(t, "pool options read custom env values", func(a *allure.Context) {
		t := a.T()
		t.Setenv("SUDOSTREAM_DB_MAX_CONNS", "25")
		t.Setenv("SUDOSTREAM_DB_MIN_CONNS", "5")
		t.Setenv("SUDOSTREAM_DB_MAX_CONN_LIFETIME", "2h")
		t.Setenv("SUDOSTREAM_DB_MAX_CONN_IDLE_TIME", "15m")

		opts := db.PoolOptionsFromEnv()
		if opts.MaxOpenConns != 25 {
			t.Fatalf("MaxOpenConns: got %d want 25", opts.MaxOpenConns)
		}
		if opts.MaxIdleConns != 5 {
			t.Fatalf("MaxIdleConns: got %d want 5", opts.MaxIdleConns)
		}
		if opts.ConnMaxLifetime != 2*time.Hour {
			t.Fatalf("ConnMaxLifetime: got %v", opts.ConnMaxLifetime)
		}
		if opts.ConnMaxIdleTime != 15*time.Minute {
			t.Fatalf("ConnMaxIdleTime: got %v", opts.ConnMaxIdleTime)
		}
	})
}

func TestDatabase_PingRequiresConnection(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ping reports missing database handle", func(a *allure.Context) {
		t := a.T()
		var database *db.Database

		err := database.Ping(context.Background())
		if err == nil {
			t.Fatal("expected ping error for nil database")
		}
	})
}

//nolint:paralleltest // integration test uses isolated schema fixture
func TestOpen_ScopedSearchPath(t *testing.T) {
	allure.Test(t, "open honors isolated schema search_path", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		database, schema := openIsolatedDatabase(ctx, t, "db_it_")

		var currentSchema string
		err := database.SQLDB().QueryRowContext(
			ctx,
			`SELECT current_schema()`,
		).Scan(&currentSchema)
		if err != nil {
			t.Fatalf("read current schema: %v", err)
		}
		if currentSchema != schema {
			t.Fatalf("current_schema: got %q want %q", currentSchema, schema)
		}
	})
}

//nolint:paralleltest // integration test uses isolated schema fixture
func TestSchema_CreateAndDrop(t *testing.T) {
	allure.Test(t, "create and drop schema via database/sql pool", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		databaseURL := requireDatabaseURL(t)
		schema := uniqueSchemaPrefix("db_schema_")

		err := db.CreateSchema(ctx, databaseURL, schema)
		if err != nil {
			t.Fatalf("create schema: %v", err)
		}

		err = db.DropSchema(ctx, databaseURL, schema)
		if err != nil {
			t.Fatalf("drop schema: %v", err)
		}
	})
}

func TestOpen_Ping(t *testing.T) {
	t.Parallel()

	allure.Test(t, "open connects and pings postgres", func(a *allure.Context) {
		t := a.T()
		databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
		if databaseURL == "" {
			t.Skip("SUDOSTREAM_DATABASE_URL not set")
		}

		database, err := db.Open(context.Background(), databaseURL, db.PoolOptionsFromEnv())
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		defer database.Close()

		err = database.Ping(context.Background())
		if err != nil {
			t.Fatalf("ping database: %v", err)
		}
	})
}
