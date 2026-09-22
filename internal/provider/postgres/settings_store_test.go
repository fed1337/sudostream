package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sudoStream/internal/db"
	"sudoStream/internal/provider"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const providerTestLibraryID = "00000000-0000-0000-0000-000000000401"

//nolint:paralleltest,cyclop // integration test shares SUDOSTREAM_DATABASE_URL fixture
func TestSettingsStore_CRUD(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "postgres settings store round-trips and reports not found", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		database := setupProviderTestDatabase(ctx, t)
		store := NewSettingsStore(database.GORM)

		if (settingsModel{}).TableName() != "library_provider_settings" {
			t.Fatal("settings table name")
		}

		_, err := store.GetSettings(ctx, providerTestLibraryID)
		if !errors.Is(err, provider.ErrNotFound) {
			t.Fatalf("want ErrNotFound before any row exists, got %v", err)
		}

		metadataKey := "anilist"
		saved, err := store.UpsertSettings(ctx, provider.Settings{
			LibraryID:                 providerTestLibraryID,
			MetadataProvider:          &metadataKey,
			SubtitleLanguages:         []string{"en", "ja"},
			AllowOverrideUserMetadata: true,
			MetadataApplyMode:         provider.ApplyModeFullRewrite,
			MetadataWriteTarget:       provider.WriteTargetFile,
		})
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
		if saved.MetadataProvider == nil || *saved.MetadataProvider != metadataKey {
			t.Fatalf("saved metadata provider: %+v", saved)
		}
		if len(saved.SubtitleLanguages) != 2 {
			t.Fatalf("saved languages: %+v", saved.SubtitleLanguages)
		}

		fetched, err := store.GetSettings(ctx, providerTestLibraryID)
		if err != nil {
			t.Fatalf("get after upsert: %v", err)
		}
		if fetched.MetadataApplyMode != provider.ApplyModeFullRewrite ||
			fetched.MetadataWriteTarget != provider.WriteTargetFile ||
			!fetched.AllowOverrideUserMetadata {
			t.Fatalf("fetched mismatch: %+v", fetched)
		}

		cleared, err := store.UpsertSettings(ctx, provider.Settings{
			LibraryID:           providerTestLibraryID,
			SubtitleLanguages:   []string{},
			MetadataApplyMode:   provider.ApplyModeFillMissing,
			MetadataWriteTarget: provider.WriteTargetDB,
		})
		if err != nil {
			t.Fatalf("re-upsert to clear: %v", err)
		}
		if cleared.MetadataProvider != nil {
			t.Fatalf("want metadata provider cleared, got %+v", cleared.MetadataProvider)
		}
	})
}

func setupProviderTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	schema := provisionProviderSchema(ctx, t)
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
		INSERT INTO libraries (id, slug, name, type)
		VALUES ($1, 'movies', 'Movies', 'film')
	`, providerTestLibraryID)
	if err != nil {
		t.Fatalf("seed library: %v", err)
	}

	return database
}

func provisionProviderSchema(ctx context.Context, t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := "provider_it_" + hex.EncodeToString(buf)

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
