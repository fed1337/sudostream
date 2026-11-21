package db_test

import (
	"context"
	"os"
	"sudoStream/internal/db"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestPoolOptionsFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       map[string]string
		wantMax   int
		wantMin   int
		wantLife  time.Duration
		wantIdle  time.Duration
		checkIdle bool
	}{
		{
			name:     "defaults when unset",
			env:      map[string]string{"SUDOSTREAM_DB_MAX_CONNS": "", "SUDOSTREAM_DB_MIN_CONNS": ""},
			wantMax:  10,
			wantMin:  2,
			wantLife: 30 * time.Minute,
		},
		{
			name: "custom values",
			env: map[string]string{
				"SUDOSTREAM_DB_MAX_CONNS":          "25",
				"SUDOSTREAM_DB_MIN_CONNS":          "5",
				"SUDOSTREAM_DB_MAX_CONN_LIFETIME":  "2h",
				"SUDOSTREAM_DB_MAX_CONN_IDLE_TIME": "15m",
			},
			wantMax:   25,
			wantMin:   5,
			wantLife:  2 * time.Hour,
			wantIdle:  15 * time.Minute,
			checkIdle: true,
		},
	} {
		allure.Test(t, "pool options from env — "+tc.name, func(a *allure.Context) {
			t := a.T()
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			opts := db.PoolOptionsFromEnv()
			if opts.MaxOpenConns != tc.wantMax {
				t.Fatalf("MaxOpenConns: got %d want %d", opts.MaxOpenConns, tc.wantMax)
			}
			if opts.MaxIdleConns != tc.wantMin {
				t.Fatalf("MaxIdleConns: got %d want %d", opts.MaxIdleConns, tc.wantMin)
			}
			if opts.ConnMaxLifetime != tc.wantLife {
				t.Fatalf("ConnMaxLifetime: got %v want %v", opts.ConnMaxLifetime, tc.wantLife)
			}
			if tc.checkIdle && opts.ConnMaxIdleTime != tc.wantIdle {
				t.Fatalf("ConnMaxIdleTime: got %v want %v", opts.ConnMaxIdleTime, tc.wantIdle)
			}
		})
	}
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
