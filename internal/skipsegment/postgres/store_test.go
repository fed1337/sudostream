package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sudoStream/internal/db"
	"sudoStream/internal/skipsegment"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testSkipLibraryID = "00000000-0000-0000-0000-000000000311"
	testSkipRelPath   = "show/S01E01.mkv"
)

func TestSkipIntroModel_TableName(t *testing.T) {
	t.Parallel()

	allure.Test(t, "skip intro model maps to skip_intro_segments table", func(a *allure.Context) {
		t := a.T()
		if (skipIntroSegmentModel{}).TableName() != "skip_intro_segments" {
			t.Fatalf("unexpected table: %q", (skipIntroSegmentModel{}).TableName())
		}
	})
}

func TestStore_SkipIntroCRUD(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "postgres skip intro store upsert get delete", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		database := setupSkipIntroTestDatabase(ctx, t)
		store := NewStore(database.GORM)

		_, found, err := store.Get(ctx, testSkipLibraryID, testSkipRelPath)
		if err != nil {
			t.Fatalf("get empty: %v", err)
		}
		if found {
			t.Fatal("expected missing")
		}

		err = store.Upsert(ctx, skipsegment.StoredSegment{
			LibraryID:     testSkipLibraryID,
			RelPath:       testSkipRelPath,
			Kind:          skipsegment.KindIntro,
			StartMs:       30_000,
			EndMs:         90_000,
			Source:        skipsegment.SourceAudio,
			Confidence:    0.91,
			EngineVersion: skipsegment.EngineVersion,
			ShowKey:       "demo",
			Season:        1,
			DetectedAt:    time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}

		got, found, err := store.Get(ctx, testSkipLibraryID, testSkipRelPath)
		if err != nil || !found {
			t.Fatalf("get after upsert: found=%v err=%v", found, err)
		}
		if got.StartMs != 30_000 || got.Source != skipsegment.SourceAudio {
			t.Fatalf("%+v", got)
		}

		err = store.Upsert(ctx, skipsegment.StoredSegment{
			LibraryID:     testSkipLibraryID,
			RelPath:       testSkipRelPath,
			Kind:          skipsegment.KindIntro,
			StartMs:       32_000,
			EndMs:         95_000,
			Source:        skipsegment.SourceAudio,
			Confidence:    0.95,
			EngineVersion: skipsegment.EngineVersion,
			ShowKey:       "demo",
			Season:        1,
		})
		if err != nil {
			t.Fatalf("upsert replace: %v", err)
		}
		got, _, err = store.Get(ctx, testSkipLibraryID, testSkipRelPath)
		if err != nil || got.StartMs != 32_000 {
			t.Fatalf("after replace: %+v err=%v", got, err)
		}

		err = store.DeleteForPath(ctx, testSkipLibraryID, testSkipRelPath)
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		_, found, err = store.Get(ctx, testSkipLibraryID, testSkipRelPath)
		if err != nil || found {
			t.Fatalf("after delete found=%v err=%v", found, err)
		}
	})
}

func setupSkipIntroTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	schema := provisionSkipIntroSchema(ctx, t)
	opts := db.PoolOptionsFromEnv()
	opts.SearchPath = schema

	database, err := db.Open(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	err = db.Migrate(database.SQLDB())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err = database.SQLDB().ExecContext(ctx, `
		INSERT INTO libraries (id, slug, name, type)
		VALUES ($1, 'series', 'Series', 'series')
	`, testSkipLibraryID)
	if err != nil {
		t.Fatalf("seed library: %v", err)
	}

	return database
}

func provisionSkipIntroSchema(ctx context.Context, t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("rand: %v", err)
	}
	schema := "skip_it_" + hex.EncodeToString(buf)
	err = db.CreateSchema(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), schema)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = db.DropSchema(dropCtx, os.Getenv("SUDOSTREAM_DATABASE_URL"), schema)
	})

	return schema
}
