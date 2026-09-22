package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sudoStream/internal/db"
	"sudoStream/internal/maintenance"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	maintTestUserID    = "00000000-0000-0000-0000-000000000301"
	maintTestLibraryID = "00000000-0000-0000-0000-000000000302"
)

//nolint:paralleltest,gocognit,cyclop,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestStore_MaintenanceCRUD(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "postgres maintenance store schedules and runs", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		database := setupMaintTestDatabase(ctx, t)
		store := NewStore(database.GORM)

		if (scheduleModel{}).TableName() != "maintenance_schedules" {
			t.Fatal("schedule table name")
		}
		if (runModel{}).TableName() != "maintenance_runs" {
			t.Fatal("run table name")
		}

		global, err := store.UpsertSchedule(ctx, maintenance.Schedule{
			Action:  maintenance.ActionLibrariesScan,
			Cron:    "0 * * * *",
			Enabled: true,
			Config:  map[string]any{"note": "global"},
		})
		if err != nil {
			t.Fatalf("upsert global: %v", err)
		}
		if global.ID == "" || !global.Enabled {
			t.Fatalf("global=%+v", global)
		}

		libID := maintTestLibraryID
		librarySched, err := store.UpsertSchedule(ctx, maintenance.Schedule{
			Action:    maintenance.ActionMetadataScan,
			LibraryID: &libID,
			Cron:      "15 * * * *",
			Enabled:   true,
			Config:    nil,
		})
		if err != nil {
			t.Fatalf("upsert library: %v", err)
		}

		global.Cron = "30 * * * *"
		global.Enabled = false
		updated, err := store.UpsertSchedule(ctx, global)
		if err != nil {
			t.Fatalf("update global: %v", err)
		}
		if updated.Cron != "30 * * * *" || updated.Enabled {
			t.Fatalf("updated=%+v", updated)
		}

		all, err := store.ListSchedules(ctx)
		if err != nil || len(all) < 2 {
			t.Fatalf("list all: %v %+v", err, all)
		}
		globals, err := store.ListGlobalSchedules(ctx)
		if err != nil || len(globals) < 1 {
			t.Fatalf("list global: %v %+v", err, globals)
		}
		forLib, err := store.ListSchedulesForLibrary(ctx, libID)
		if err != nil || len(forLib) != 1 || forLib[0].ID != librarySched.ID {
			t.Fatalf("list library: %v %+v", err, forLib)
		}

		run, err := store.InsertRun(ctx, maintenance.Run{
			Action:    maintenance.ActionLibrariesScan,
			Trigger:   maintenance.TriggerManual,
			Status:    maintenance.StatusRunning,
			StartedAt: time.Now().UTC(),
			Summary:   nil,
		})
		if err != nil {
			t.Fatalf("insert run: %v", err)
		}

		err = store.FinishRun(ctx, run.ID, maintenance.StatusSuccess, map[string]any{
			"ok": true,
		})
		if err != nil {
			t.Fatalf("finish run: %v", err)
		}

		latest, err := store.LatestRun(ctx, maintenance.ActionLibrariesScan, nil)
		if err != nil || latest.Status != maintenance.StatusSuccess {
			t.Fatalf("latest: %v %+v", err, latest)
		}

		libRun, err := store.InsertRun(ctx, maintenance.Run{
			Action:    maintenance.ActionMetadataScan,
			LibraryID: &libID,
			Trigger:   maintenance.TriggerSchedule,
			Status:    maintenance.StatusRunning,
			StartedAt: time.Now().UTC(),
			Summary:   map[string]any{},
		})
		if err != nil {
			t.Fatalf("insert lib run: %v", err)
		}
		_ = libRun

		n, err := store.MarkInterruptedRuns(ctx)
		if err != nil || n < 1 {
			t.Fatalf("mark interrupted: n=%d err=%v", n, err)
		}

		runs, err := store.ListRuns(ctx, 10)
		if err != nil || len(runs) < 2 {
			t.Fatalf("list runs: %v %+v", err, runs)
		}
	})
}

func setupMaintTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	schema := provisionMaintSchema(ctx, t)
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
		VALUES ($1, 'maint-store-test@example.com', 'hash', 'admin', TRUE, FALSE)
	`, maintTestUserID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO libraries (id, slug, name, type)
		VALUES ($1, 'movies', 'Movies', 'film')
	`, maintTestLibraryID)
	if err != nil {
		t.Fatalf("seed library: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO library_roots (library_id, rel_path)
		VALUES ($1, 'movies')
	`, maintTestLibraryID)
	if err != nil {
		t.Fatalf("seed library root: %v", err)
	}

	return database
}

func provisionMaintSchema(ctx context.Context, t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := "maint_it_" + hex.EncodeToString(buf)

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
