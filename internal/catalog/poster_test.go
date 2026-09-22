package catalog

import (
	"context"
	"errors"
	"path/filepath"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

var errPosterIndexBoom = errors.New("poster index failed")

type stubPosterIndex struct {
	paths map[string]struct{}
	err   error
}

func (s stubPosterIndex) PosterPaths(
	_ context.Context,
	_ string,
) (map[string]struct{}, error) {
	return s.paths, s.err
}

func newPosterCatalog(t *testing.T, libraryType access.LibraryType, files []string) *Service {
	t.Helper()

	slug := catalogMoviesSlug
	if libraryType == access.LibraryTypeSeries {
		slug = catalogSeriesSlug
	}

	indexed := make([]string, 0, len(files))
	for _, rel := range files {
		indexed = append(indexed, filepath.ToSlash(filepath.Join(slug, rel)))
	}

	return NewService(
		stubPaths{byLibrary: map[string][]string{"lib-1": indexed}},
		stubAccess{libraries: []access.Library{
			{ID: "lib-1", Slug: slug, RelPath: slug, Type: libraryType},
		}},
		stubMetadata{},
	)
}

func TestCatalog_PosterURLPrefersCachedProviderPoster(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cards expose posterUrl only for locally cached provider posters",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			admin := auth.PublicUser{Role: auth.RoleAdmin}

			films := newPosterCatalog(
				t,
				access.LibraryTypeFilm,
				[]string{"Casino 1995.mkv", "Blow 2001.mkv"},
			)
			films.SetPosterIndex(stubPosterIndex{paths: map[string]struct{}{
				"movies/Casino 1995.mkv": {},
			}})

			result, err := films.GetLibraryCatalog(ctx, admin, catalogMoviesSlug, mediafs.PageOpts{})
			if err != nil {
				t.Fatalf("film catalog: %v", err)
			}

			withPoster := 0
			for _, movie := range result.Movies {
				if movie.PosterURL == "" {
					continue
				}
				withPoster++
				if movie.PosterURL != "/api/provider-poster/movies/Casino%201995.mkv" {
					t.Fatalf("poster url: %s", movie.PosterURL)
				}
			}
			if withPoster != 1 {
				t.Fatalf("want exactly one card with a provider poster, got %d", withPoster)
			}

			shows := newPosterCatalog(
				t,
				access.LibraryTypeSeries,
				[]string{"Show A/S01E01.mkv", "Show A/S01E02.mkv"},
			)
			shows.SetPosterIndex(stubPosterIndex{paths: map[string]struct{}{
				"series/Show A/S01E01.mkv": {},
			}})

			result, err = shows.GetLibraryCatalog(ctx, admin, catalogSeriesSlug, mediafs.PageOpts{})
			if err != nil || len(result.Shows) != 1 {
				t.Fatalf("series catalog: %+v %v", result.Shows, err)
			}
			if result.Shows[0].PosterURL == "" {
				t.Fatal("want show poster keyed by the representative episode")
			}
		})
}

func TestCatalog_ShowAndEpisodePosterURL(t *testing.T) {
	t.Parallel()

	allure.Test(t, "show detail and episodes expose provider posterUrl",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			admin := auth.PublicUser{Role: auth.RoleAdmin}

			shows := newPosterCatalog(
				t,
				access.LibraryTypeSeries,
				[]string{"Show A/S01E01.mkv", "Show A/S01E02.mkv"},
			)
			shows.SetPosterIndex(stubPosterIndex{paths: map[string]struct{}{
				"series/Show A/S01E01.mkv": {},
			}})

			result, err := shows.GetLibraryCatalog(ctx, admin, catalogSeriesSlug, mediafs.PageOpts{})
			if err != nil || len(result.Shows) != 1 {
				t.Fatalf("series catalog: %+v %v", result.Shows, err)
			}
			detail, err := shows.GetShow(ctx, admin, catalogSeriesSlug, result.Shows[0].ShowKey)
			if err != nil {
				t.Fatalf("show detail: %v", err)
			}
			if detail.PosterURL == "" || detail.SeasonCount != 1 || detail.EpisodeCount != 2 {
				t.Fatalf("want show hero poster and counts, got %+v", detail)
			}
			episodes, err := shows.ListSeasonEpisodes(
				ctx, admin, catalogSeriesSlug, result.Shows[0].ShowKey, 1, mediafs.PageOpts{},
			)
			if err != nil || len(episodes.Episodes) != 2 {
				t.Fatalf("episodes: %+v %v", episodes, err)
			}
			if episodes.Episodes[0].PosterURL == "" {
				t.Fatal("want episode posterUrl when artifact exists for path")
			}
		})
}

func TestCatalog_PosterIndexFailureFallsBackToThumbnails(t *testing.T) {
	t.Parallel()

	allure.Test(t, "poster index errors and nil index degrade to thumbnails",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			admin := auth.PublicUser{Role: auth.RoleAdmin}

			films := newPosterCatalog(t, access.LibraryTypeFilm, []string{"Casino 1995.mkv"})
			films.SetPosterIndex(stubPosterIndex{err: errPosterIndexBoom})

			result, err := films.GetLibraryCatalog(ctx, admin, catalogMoviesSlug, mediafs.PageOpts{})
			if err != nil || len(result.Movies) != 1 {
				t.Fatalf("catalog: %+v %v", result.Movies, err)
			}
			if result.Movies[0].PosterURL != "" {
				t.Fatal("want no poster url when the index fails")
			}
			if result.Movies[0].Actions.Thumbnail == "" {
				t.Fatal("want thumbnail fallback preserved")
			}

			var nilService *Service
			nilService.SetPosterIndex(stubPosterIndex{})
		})
}
