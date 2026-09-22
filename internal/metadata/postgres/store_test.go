package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sudoStream/internal/access"
	accesspostgres "sudoStream/internal/access/postgres"
	"sudoStream/internal/db"
	"sudoStream/internal/metadata"
	"sudoStream/internal/watch"
	watchpostgres "sudoStream/internal/watch/postgres"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testStorePilotTitle    = "Pilot"
	testStoreOverrideTitle = "Override"
	testStoreLibrarySlug   = "series"
	testStoreUserID        = "00000000-0000-0000-0000-000000000001"
	testStoreContinuePath  = "films/mid.mp4"
)

//nolint:paralleltest,gocognit,cyclop,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestStore_MetadataCRUD(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"postgres metadata store upserts probes and deletes paths",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := setupMetadataTestDatabase(ctx, t)
			accessStore := accesspostgres.NewStore(database.GORM)
			store := NewStore(database.GORM)

			library, err := accessStore.UpsertLibrary(ctx, access.Library{
				Slug:    testStoreLibrarySlug,
				RelPath: testStoreLibrarySlug,
				Name:    "Series",
				Type:    access.LibraryTypeSeries,
			})
			if err != nil {
				t.Fatalf("upsert library: %v", err)
			}

			relPath := "series/Demo/S01E01.mkv"
			title := testStorePilotTitle
			ed2kHash := "d41d8cd98f00b204e9800998ecf8427e"
			mtime := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
			size := int64(1024)
			probedAt := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)

			err = store.UpsertOriginal(
				ctx,
				library.ID,
				relPath,
				metadata.VideoFields{Title: &title, Ed2kHash: &ed2kHash, HashFileSize: &size},
				mtime,
				size,
				probedAt,
			)
			if err != nil {
				t.Fatalf("upsert original: %v", err)
			}

			row, err := store.Get(ctx, library.ID, relPath)
			if err != nil {
				t.Fatalf("get metadata: %v", err)
			}
			if row.Original.Title == nil || *row.Original.Title != testStorePilotTitle {
				t.Fatalf("unexpected original title: %#v", row.Original.Title)
			}
			if row.ProbedAt == nil || !row.ProbedAt.Equal(probedAt) {
				t.Fatalf("unexpected probed_at: %#v", row.ProbedAt)
			}

			needsProbe, err := store.NeedsProbe(ctx, library.ID, relPath, mtime, size)
			if err != nil {
				t.Fatalf("needs probe unchanged: %v", err)
			}
			if needsProbe {
				t.Fatal("expected cached stat to skip probe")
			}

			needsProbe, err = store.NeedsProbe(ctx, library.ID, relPath, mtime.Add(time.Hour), size)
			if err != nil {
				t.Fatalf("needs probe changed mtime: %v", err)
			}
			if !needsProbe {
				t.Fatal("expected mtime change to require probe")
			}

			overrideTitle := testStoreOverrideTitle
			updatedAt := time.Now().UTC()
			err = store.UpdateOverride(
				ctx,
				library.ID,
				relPath,
				metadata.StoredOverride{VideoFields: metadata.VideoFields{Title: &overrideTitle}},
				updatedAt,
				testStoreUserID,
			)
			if err != nil {
				t.Fatalf("update override: %v", err)
			}

			row, err = store.Get(ctx, library.ID, relPath)
			if err != nil {
				t.Fatalf("get after override: %v", err)
			}
			if row.Override.Title == nil || *row.Override.Title != testStoreOverrideTitle {
				t.Fatalf("unexpected override title: %#v", row.Override.Title)
			}
			if row.OverriddenBy == nil || *row.OverriddenBy != testStoreUserID {
				t.Fatalf("unexpected overridden_by: %#v", row.OverriddenBy)
			}

			paths, err := store.ListIndexedPaths(ctx, library.ID)
			if err != nil {
				t.Fatalf("list indexed paths: %v", err)
			}
			if len(paths) != 1 || paths[0] != relPath {
				t.Fatalf("unexpected indexed paths: %#v", paths)
			}

			active, err := store.ListActiveIndexedPaths(ctx, library.ID)
			if err != nil {
				t.Fatalf("list active indexed paths: %v", err)
			}
			if len(active) != 1 || active[0] != relPath {
				t.Fatalf("unexpected active indexed paths: %#v", active)
			}

			err = store.DeletePath(ctx, library.ID, relPath)
			if err != nil {
				t.Fatalf("delete path: %v", err)
			}

			_, err = store.Get(ctx, library.ID, relPath)
			if !errors.Is(err, metadata.ErrNotFound) {
				t.Fatalf("expected not found after delete, got %v", err)
			}

			needsProbe, err = store.NeedsProbe(ctx, library.ID, relPath, mtime, size)
			if err != nil {
				t.Fatalf("needs probe missing row: %v", err)
			}
			if !needsProbe {
				t.Fatal("expected missing row to require probe")
			}
		},
	)
}

func TestStore_ListUnwatchedHidesCompletedAndContinue( //nolint:cyclop,paralleltest
	t *testing.T,
) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"unwatched shelf hides completed and in-progress titles",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := setupMetadataTestDatabase(ctx, t)
			accessStore := accesspostgres.NewStore(database.GORM)
			store := NewStore(database.GORM)
			watchStore := watchpostgres.NewStore(database.GORM)

			library, err := accessStore.UpsertLibrary(ctx, access.Library{
				Slug:    "films",
				RelPath: "films",
				Name:    "Films",
				Type:    access.LibraryTypeFilm,
			})
			if err != nil {
				t.Fatalf("upsert library: %v", err)
			}

			probe := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
			for _, relPath := range []string{
				"films/plain.mp4",
				"films/done.mp4",
				testStoreContinuePath,
			} {
				title := relPath
				err = store.UpsertOriginal(
					ctx,
					library.ID,
					relPath,
					metadata.VideoFields{Title: &title},
					probe,
					1024,
					probe,
				)
				if err != nil {
					t.Fatalf("upsert %s: %v", relPath, err)
				}
			}

			err = watchStore.Upsert(ctx, testStoreUserID, library.ID, "films/done.mp4", watch.State{
				Watched: true,
			})
			if err != nil {
				t.Fatalf("mark watched: %v", err)
			}
			position := 40.0
			duration := 100.0
			err = watchStore.Upsert(
				ctx,
				testStoreUserID,
				library.ID,
				testStoreContinuePath,
				watch.State{PositionSeconds: &position, DurationSeconds: &duration},
			)
			if err != nil {
				t.Fatalf("mark continue: %v", err)
			}

			rows, err := store.ListUnwatched(ctx, testStoreUserID, []string{library.ID}, 10, 0)
			if err != nil || len(rows) != 1 || rows[0].RelPath != "films/plain.mp4" {
				t.Fatalf("list unwatched: %#v err=%v", rows, err)
			}
			total, err := store.CountUnwatched(ctx, testStoreUserID, []string{library.ID})
			if err != nil || total != 1 {
				t.Fatalf("count unwatched: %d err=%v", total, err)
			}

			counts, err := store.CountByLibraries(ctx, []string{library.ID})
			if err != nil || counts[library.ID] != 3 {
				t.Fatalf("count by libraries: %#v err=%v", counts, err)
			}

			shelf, err := store.ListShelfItems(
				ctx,
				[]string{library.ID},
				[]string{testStoreContinuePath},
			)
			if err != nil || len(shelf) != 1 || shelf[0].Title != testStoreContinuePath {
				t.Fatalf("list shelf items: %#v err=%v", shelf, err)
			}
		},
	)
}

func TestStore_Search_SceneRelease( //nolint:cyclop,paralleltest // integration search + trash filter
	t *testing.T,
) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"search_document matches scene-release friends via ILIKE tokens",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := setupMetadataTestDatabase(ctx, t)
			accessStore := accesspostgres.NewStore(database.GORM)
			store := NewStore(database.GORM)

			library, err := accessStore.UpsertLibrary(ctx, access.Library{
				Slug:    testStoreLibrarySlug,
				RelPath: testStoreLibrarySlug,
				Name:    "Series",
				Type:    access.LibraryTypeSeries,
			})
			if err != nil {
				t.Fatalf("upsert library: %v", err)
			}

			relPath := "friends/[linuxisos.ru].friends.s01.e01.mkv"
			err = store.UpsertOriginal(
				ctx,
				library.ID,
				relPath,
				metadata.VideoFields{},
				time.Now().UTC(),
				4,
				time.Now().UTC(),
			)
			if err != nil {
				t.Fatalf("upsert: %v", err)
			}

			tokens := metadata.NormalizeSearchQuery("friends")
			rows, total, err := store.Search(ctx, []string{library.ID}, tokens, "friends", 20, 0)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if total != 1 || len(rows) != 1 || rows[0].RelPath != relPath {
				t.Fatalf("rows=%+v total=%d", rows, total)
			}

			hidden := ".trash/item/friends.mkv"
			err = store.UpsertOriginal(
				ctx,
				library.ID,
				hidden,
				metadata.VideoFields{},
				time.Now().UTC(),
				4,
				time.Now().UTC(),
			)
			if err != nil {
				t.Fatalf("upsert trash: %v", err)
			}

			rows, total, err = store.Search(ctx, []string{library.ID}, tokens, "friends", 20, 0)
			if err != nil {
				t.Fatalf("search after trash: %v", err)
			}
			if total != 1 || rows[0].RelPath != relPath {
				t.Fatalf("trash leaked: rows=%+v total=%d", rows, total)
			}

			_, err = database.SQLDB().ExecContext(ctx, `
				INSERT INTO trash_items (id, original_rel_path, trash_rel_path, deleted_at)
				VALUES ($1, $2, $3, NOW())
			`, "44444444-4444-4444-4444-444444444444", relPath, ".trash/id/"+relPath)
			if err != nil {
				t.Fatalf("insert trash_items: %v", err)
			}

			rows, total, err = store.Search(ctx, []string{library.ID}, tokens, "friends", 20, 0)
			if err != nil {
				t.Fatalf("search after trash_items: %v", err)
			}
			if total != 0 || len(rows) != 0 {
				t.Fatalf("frozen original leaked: rows=%+v total=%d", rows, total)
			}
		},
	)
}

// setupMetadataTestDatabase provisions an isolated Postgres schema per test.
func setupMetadataTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	schema := provisionMetadataSchema(ctx, t)

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
		VALUES ($1, 'metadata-store-test@example.com', 'hash', 'admin', TRUE, FALSE)
	`, testStoreUserID)
	if err != nil {
		t.Fatalf("seed metadata store test user: %v", err)
	}

	return database
}

func provisionMetadataSchema(ctx context.Context, t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := "metadata_it_" + hex.EncodeToString(buf)

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
