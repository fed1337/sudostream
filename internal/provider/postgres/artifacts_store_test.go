package postgres

import (
	"context"
	"errors"
	"os"
	"sudoStream/internal/provider"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	artifactTestPath    = "movies/a.mkv"
	artifactProviderKey = "anilist"
)

//nolint:paralleltest,cyclop,gocognit,funlen // integration test shares SUDOSTREAM_DATABASE_URL fixture
func TestArtifactStore_CRUD(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "postgres artifact store upserts, lists, and deletes cache rows",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := setupProviderTestDatabase(ctx, t)
			store := NewArtifactStore(database.GORM)

			if (artifactModel{}).TableName() != "provider_artifacts" {
				t.Fatal("artifact table name")
			}

			_, err := store.GetArtifactByPath(
				ctx,
				artifactTestPath,
				provider.ArtifactKindPoster,
				nil,
			)
			if !errors.Is(err, provider.ErrNotFound) {
				t.Fatalf("want ErrNotFound before any row, got %v", err)
			}

			externalID := "9253"
			saved, err := store.UpsertArtifact(ctx, provider.Artifact{
				LibraryID:   providerTestLibraryID,
				RelPath:     artifactTestPath,
				Kind:        provider.ArtifactKindPoster,
				ProviderKey: artifactProviderKey,
				CachePath:   "posters/lib/a.webp",
				ExternalID:  &externalID,
			})
			if err != nil {
				t.Fatalf("upsert: %v", err)
			}
			if saved.ID == "" || saved.FetchedAt.IsZero() {
				t.Fatalf("want generated id and fetched_at, got %+v", saved)
			}

			// Same lookup tuple must update in place, not insert a duplicate.
			updated, err := store.UpsertArtifact(ctx, provider.Artifact{
				LibraryID:   providerTestLibraryID,
				RelPath:     artifactTestPath,
				Kind:        provider.ArtifactKindPoster,
				ProviderKey: artifactProviderKey,
				CachePath:   "posters/lib/a.jpg",
				FetchedAt:   time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("re-upsert: %v", err)
			}
			if updated.ID != saved.ID || updated.CachePath != "posters/lib/a.jpg" {
				t.Fatalf("want in-place update, got %+v", updated)
			}

			lang := "en"
			_, err = store.UpsertArtifact(ctx, provider.Artifact{
				LibraryID:   providerTestLibraryID,
				RelPath:     artifactTestPath,
				Kind:        provider.ArtifactKindSubtitle,
				Lang:        &lang,
				ProviderKey: "opensubtitles",
				CachePath:   "subtitles/lib/a/en.vtt",
			})
			if err != nil {
				t.Fatalf("upsert subtitle: %v", err)
			}

			posters, err := store.ListArtifacts(
				ctx,
				providerTestLibraryID,
				provider.ArtifactKindPoster,
			)
			if err != nil || len(posters) != 1 {
				t.Fatalf("list posters: %+v %v", posters, err)
			}

			subtitles, err := store.ListArtifacts(
				ctx,
				providerTestLibraryID,
				provider.ArtifactKindSubtitle,
			)
			if err != nil || len(subtitles) != 1 || subtitles[0].Lang == nil {
				t.Fatalf("list subtitles: %+v %v", subtitles, err)
			}

			fetched, err := store.GetArtifactByPath(
				ctx,
				artifactTestPath,
				provider.ArtifactKindSubtitle,
				&lang,
			)
			if err != nil || fetched.ProviderKey != "opensubtitles" {
				t.Fatalf("get subtitle: %+v %v", fetched, err)
			}

			err = store.DeleteArtifact(ctx, saved.ID)
			if err != nil {
				t.Fatalf("delete: %v", err)
			}
			err = store.DeleteArtifact(ctx, saved.ID)
			if err != nil {
				t.Fatalf("delete missing row must be a no-op, got %v", err)
			}

			posters, err = store.ListArtifacts(
				ctx,
				providerTestLibraryID,
				provider.ArtifactKindPoster,
			)
			if err != nil || len(posters) != 0 {
				t.Fatalf("list after delete: %+v %v", posters, err)
			}
		})
}
