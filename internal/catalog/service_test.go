package catalog

import (
	"context"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type stubAccess struct {
	libraries []access.Library
}

func (s stubAccess) ListReadableLibraries(
	_ context.Context,
	_ auth.PublicUser,
) ([]access.Library, error) {
	return s.libraries, nil
}

func (s stubAccess) CanRead(_ context.Context, _ auth.PublicUser, _ string) (bool, error) {
	return true, nil
}

type stubMetadata struct{}

func (stubMetadata) Get(_ context.Context, _ string) (metadata.MetadataResponse, error) {
	return metadata.MetadataResponse{}, metadata.ErrNotFound
}

func TestService_ListMovies(t *testing.T) {
	t.Parallel()

	allure.Test(t, "lists one movie card per indexed video path", func(a *allure.Context) {
		t := a.T()
		svc := NewService(
			indexedFilmPaths(catalogMoviesSlug,
				"Casino.1995.BDRip.1080p.mkv",
				"Blow.2001.BDRip.720p.mkv",
			),
			stubAccess{libraries: []access.Library{
				{
					ID:      "1",
					Slug:    catalogMoviesSlug,
					RelPath: catalogMoviesSlug,
					Type:    access.LibraryTypeFilm,
				},
			}},
			stubMetadata{},
		)

		catalog, err := svc.GetLibraryCatalog(
			context.Background(),
			auth.PublicUser{Role: auth.RoleAdmin},
			catalogMoviesSlug,
			mediafs.PageOpts{},
		)
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
		if len(catalog.Movies) != 2 || catalog.Total != 2 {
			t.Fatalf("movies=%d total=%d", len(catalog.Movies), catalog.Total)
		}
	})
}

func TestService_SeriesGroupAndShow(t *testing.T) { //nolint:cyclop // multi-assert series flow
	t.Parallel()

	allure.Test(t, "groups season folders into one show from indexed paths", func(a *allure.Context) {
		t := a.T()
		index := indexedSeriesPaths(
			"series/90210 2008 Season 1 Complete/90210 S01E01 Pilot.mkv",
			"series/90210 2008 Season 2 Complete/90210 S02E01 Return.mkv",
		)
		svc := NewService(index, stubAccess{libraries: []access.Library{{
			ID: "1", Slug: "series", RelPath: "series", Type: access.LibraryTypeSeries,
		}}}, stubMetadata{})

		user := auth.PublicUser{Role: auth.RoleAdmin}
		catalog, err := svc.GetLibraryCatalog(
			context.Background(),
			user,
			"series",
			mediafs.PageOpts{},
		)
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
		if len(catalog.Shows) != 1 {
			t.Fatalf("shows=%d %+v", len(catalog.Shows), catalog.Shows)
		}
		showKey := catalog.Shows[0].ShowKey
		detail, err := svc.GetShow(context.Background(), user, "series", showKey)
		if err != nil {
			t.Fatalf("show: %v", err)
		}
		if len(detail.Seasons) != 2 {
			t.Fatalf("seasons=%d", len(detail.Seasons))
		}
		if detail.Seasons[0].EpisodeCount < 1 {
			t.Fatalf("expected episode counts, got %+v", detail.Seasons)
		}
		episodes, err := svc.ListSeasonEpisodes(
			context.Background(),
			user,
			"series",
			showKey,
			detail.Seasons[0].Season,
			mediafs.PageOpts{},
		)
		if err != nil {
			t.Fatalf("episodes: %v", err)
		}
		if episodes.Total < 1 || len(episodes.Episodes) < 1 {
			t.Fatalf("episodes=%+v", episodes)
		}
		if episodes.Limit != episodes.Total || len(episodes.Episodes) != episodes.Total {
			t.Fatalf("want full season when limit unset, got %+v", episodes)
		}

		identity, ok := svc.ResolveSeriesEpisode(
			context.Background(),
			access.LibraryTypeSeries,
			"series/90210 2008 Season 1 Complete/90210 S01E01 Pilot.mkv",
		)
		if !ok || identity.Season != 1 || identity.Episode != 1 || identity.ShowKey == "" {
			t.Fatalf("ResolveSeriesEpisode: ok=%v %+v", ok, identity)
		}
		_, filmOK := svc.ResolveSeriesEpisode(
			context.Background(),
			access.LibraryTypeFilm,
			"series/90210 2008 Season 1 Complete/90210 S01E01 Pilot.mkv",
		)
		if filmOK {
			t.Fatal("film library must not resolve series identity")
		}

		special, specialOK := svc.ResolveSeriesEpisode(
			context.Background(),
			access.LibraryTypeSeries,
			"series/Andy Panda/Andy Panda - Andy Panda's Pop [SATRip-Rus].avi",
		)
		if !specialOK || special.ShowKey == "" {
			t.Fatalf("special without S/E must still resolve show: ok=%v %+v", specialOK, special)
		}
		if special.Season != 0 || special.Episode != 0 {
			t.Fatalf("special without S/E should default to S0E0, got %+v", special)
		}
	})
}

func TestService_CatalogPagination(t *testing.T) {
	t.Parallel()

	allure.Test(t, "film catalog pages with limit offset", func(a *allure.Context) {
		t := a.T()
		svc := NewService(
			indexedFilmPaths(catalogMoviesSlug, "A.mkv", "B.mkv", "C.mkv"),
			stubAccess{libraries: []access.Library{
				{
					ID:      "1",
					Slug:    catalogMoviesSlug,
					RelPath: catalogMoviesSlug,
					Type:    access.LibraryTypeFilm,
				},
			}},
			stubMetadata{},
		)

		page, err := svc.GetLibraryCatalog(
			context.Background(),
			auth.PublicUser{Role: auth.RoleAdmin},
			catalogMoviesSlug,
			mediafs.PageOpts{Limit: 2, Offset: 0},
		)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 3 || len(page.Movies) != 2 {
			t.Fatalf("page=%+v", page)
		}
	})
}

func TestService_EmptyUntilIndexed(t *testing.T) {
	t.Parallel()

	allure.Test(t, "catalog is empty when index has no rows", func(a *allure.Context) {
		t := a.T()
		svc := NewService(
			PathCatalogIndex{
				LibraryType: access.LibraryTypeFilm,
				ByLibrary:   map[string][]string{"1": {}},
			},
			stubAccess{libraries: []access.Library{{
				ID: "1", Slug: catalogMoviesSlug, RelPath: catalogMoviesSlug, Type: access.LibraryTypeFilm,
			}}},
			stubMetadata{},
		)
		catalog, err := svc.GetLibraryCatalog(
			context.Background(),
			auth.PublicUser{Role: auth.RoleAdmin},
			catalogMoviesSlug,
			mediafs.PageOpts{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if catalog.Total != 0 || len(catalog.Movies) != 0 {
			t.Fatalf("want empty catalog, got %+v", catalog)
		}
	})
}

func TestService_ListMoviesFromAllRoots(t *testing.T) {
	t.Parallel()

	allure.Test(t, "film catalog includes indexed videos from every library root", func(a *allure.Context) {
		t := a.T()
		svc := NewService(
			PathCatalogIndex{
				LibraryType: access.LibraryTypeFilm,
				ByLibrary: map[string][]string{
					"1": {"movies/One.mkv", "extra/Two.mkv"},
				},
			},
			stubAccess{libraries: []access.Library{
				{
					ID:    "1",
					Slug:  catalogMoviesSlug,
					Type:  access.LibraryTypeFilm,
					Roots: []string{"movies", "extra"},
				},
			}},
			stubMetadata{},
		)

		catalog, err := svc.GetLibraryCatalog(
			context.Background(),
			auth.PublicUser{Role: auth.RoleAdmin},
			catalogMoviesSlug,
			mediafs.PageOpts{},
		)
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
		if catalog.Total != 2 || len(catalog.Movies) != 2 {
			t.Fatalf("movies=%d total=%d", len(catalog.Movies), catalog.Total)
		}
	})
}
