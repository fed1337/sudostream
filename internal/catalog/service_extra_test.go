package catalog

import (
	"context"
	"errors"
	"strconv"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

var errStubAccessList = errors.New("list libraries failed")

const (
	catalogMoviesSlug = "movies"
	catalogSeriesSlug = "series"
)

type denyAccess struct {
	libraries []access.Library
}

func (d denyAccess) ListReadableLibraries(
	_ context.Context,
	user auth.PublicUser,
) ([]access.Library, error) {
	if user.Role != auth.RoleAdmin {
		return nil, nil
	}

	return d.libraries, nil
}

func (denyAccess) CanRead(_ context.Context, _ auth.PublicUser, _ string) (bool, error) {
	return false, nil
}

type failListAccess struct{}

func (failListAccess) ListReadableLibraries(
	_ context.Context,
	_ auth.PublicUser,
) ([]access.Library, error) {
	return nil, errStubAccessList
}

func (failListAccess) CanRead(_ context.Context, _ auth.PublicUser, _ string) (bool, error) {
	return true, nil
}

type metaStub struct {
	byPath map[string]metadata.MetadataResponse
}

func (m metaStub) Get(_ context.Context, rawPath string) (metadata.MetadataResponse, error) {
	if response, ok := m.byPath[rawPath]; ok {
		return response, nil
	}

	return metadata.MetadataResponse{}, metadata.ErrNotFound
}

// fixedCatalogIndex returns predetermined denorm rows (simulates write-time override identity).
type fixedCatalogIndex struct {
	movies   []metadata.CatalogMovieRow
	shows    []metadata.CatalogShowRow
	showAggs map[string]metadata.CatalogShowAgg
	episodes map[string][]metadata.CatalogEpisodeRow
}

func (f fixedCatalogIndex) ListCatalogMoviesPage(
	_ context.Context,
	_ string,
	limit, offset int,
) ([]metadata.CatalogMovieRow, int, error) {
	total := len(f.movies)
	if offset > total {
		return nil, total, nil
	}
	end := total
	if limit > 0 {
		end = min(offset+limit, total)
	}

	return f.movies[offset:end], total, nil
}

func (f fixedCatalogIndex) ListCatalogShowsPage(
	_ context.Context,
	_ string,
	limit, offset int,
) ([]metadata.CatalogShowRow, int, error) {
	total := len(f.shows)
	if offset > total {
		return nil, total, nil
	}
	end := total
	if limit > 0 {
		end = min(offset+limit, total)
	}

	return f.shows[offset:end], total, nil
}

func (f fixedCatalogIndex) GetCatalogShowAgg(
	_ context.Context,
	_, showKey string,
) (metadata.CatalogShowAgg, bool, error) {
	agg, ok := f.showAggs[showKey]

	return agg, ok, nil
}

func (f fixedCatalogIndex) CatalogShowExists(
	_ context.Context,
	_, showKey string,
) (bool, error) {
	_, ok := f.showAggs[showKey]

	return ok, nil
}

func (f fixedCatalogIndex) ListCatalogSeasonEpisodes(
	_ context.Context,
	_, showKey string,
	season, limit, offset int,
) ([]metadata.CatalogEpisodeRow, int, error) {
	key := showKey + "|" + strconv.Itoa(season)
	rows := f.episodes[key]
	total := len(rows)
	if offset > total {
		return nil, total, nil
	}
	end := total
	if limit > 0 {
		end = min(offset+limit, total)
	}

	return rows[offset:end], total, nil
}

//nolint:gocognit,cyclop,funlen,maintidx // multi-scenario catalog service coverage
func TestService_ErrorAndMetadataPaths(t *testing.T) {
	t.Parallel()

	allure.Test(t, "unsupported library type and missing slug", func(a *allure.Context) {
		t := a.T()
		svc := NewService(PathCatalogIndex{}, stubAccess{libraries: []access.Library{{
			ID: "1", Slug: "photos", RelPath: "photos", Type: access.LibraryTypePhotos,
		}}}, stubMetadata{})

		user := auth.PublicUser{Role: auth.RoleAdmin}
		_, err := svc.GetLibraryCatalog(context.Background(), user, "photos", mediafs.PageOpts{})
		if !errors.Is(err, ErrUnsupportedType) {
			t.Fatalf("want ErrUnsupportedType, got %v", err)
		}

		_, err = svc.GetLibraryCatalog(context.Background(), user, "missing", mediafs.PageOpts{})
		if !errors.Is(err, ErrLibraryNotFound) {
			t.Fatalf("want ErrLibraryNotFound, got %v", err)
		}

		_, err = NewService(nil, nil, nil).GetLibraryCatalog(
			context.Background(),
			user,
			"x",
			mediafs.PageOpts{},
		)
		if !errors.Is(err, ErrLibraryNotFound) {
			t.Fatalf("nil access: want ErrLibraryNotFound, got %v", err)
		}
	})

	allure.Test(t, "non-admin without can_read is not found", func(a *allure.Context) {
		t := a.T()
		svc := NewService(PathCatalogIndex{}, denyAccess{libraries: []access.Library{
			{
				ID:      "1",
				Slug:    catalogMoviesSlug,
				RelPath: catalogMoviesSlug,
				Type:    access.LibraryTypeFilm,
			},
		}}, stubMetadata{})

		_, err := svc.GetLibraryCatalog(
			context.Background(),
			auth.PublicUser{Role: auth.RoleUser},
			catalogMoviesSlug,
			mediafs.PageOpts{},
		)
		if !errors.Is(err, ErrLibraryNotFound) {
			t.Fatalf("want ErrLibraryNotFound, got %v", err)
		}
	})

	allure.Test(t, "list libraries error wraps", func(a *allure.Context) {
		t := a.T()
		svc := NewService(PathCatalogIndex{}, failListAccess{}, stubMetadata{})
		_, err := svc.GetLibraryCatalog(
			context.Background(),
			auth.PublicUser{Role: auth.RoleAdmin},
			catalogMoviesSlug,
			mediafs.PageOpts{},
		)
		if err == nil || !errors.Is(err, errStubAccessList) {
			t.Fatalf("want wrapped list error, got %v", err)
		}
	})

	allure.Test(t, "film denorm title and year preferred", func(a *allure.Context) {
		t := a.T()
		rel := catalogMoviesSlug + "/opaque.mkv"
		title := "Casino"
		year := 1995
		svc := NewService(
			fixedCatalogIndex{movies: []metadata.CatalogMovieRow{{
				RelPath: rel, Title: title, Year: &year,
			}}},
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
			t.Fatal(err)
		}
		if len(catalog.Movies) != 1 || catalog.Movies[0].Title != title {
			t.Fatalf("movies=%+v", catalog.Movies)
		}
		if catalog.Movies[0].Year == nil || *catalog.Movies[0].Year != year {
			t.Fatalf("year=%v", catalog.Movies[0].Year)
		}
	})

	allure.Test(
		t,
		"series denorm fills show fields; GetShow missing key",
		func(a *allure.Context) {
			t := a.T()
			rel := catalogSeriesSlug + "/show/ep.mkv"
			show := "Melrose Place"
			showKey := metadata.NormalizeShowKey(show)
			epTitle := "Pilot"
			season, episode := 1, 1
			svc := NewService(
				fixedCatalogIndex{
					shows: []metadata.CatalogShowRow{{
						ShowKey: showKey, Name: show, SeasonCount: 1, EpisodeCount: 1, PosterPath: rel,
					}},
					showAggs: map[string]metadata.CatalogShowAgg{
						showKey: {
							ShowKey: showKey, Name: show, PosterPath: rel,
							Seasons: []metadata.CatalogSeasonCount{{Season: 1, EpisodeCount: 1}},
						},
					},
					episodes: map[string][]metadata.CatalogEpisodeRow{
						showKey + "|1": {{
							RelPath: rel, Title: "S01E01", Season: &season, Episode: &episode,
							EpisodeTitle: epTitle,
						}},
					},
				},
				stubAccess{libraries: []access.Library{
					{
						ID:      "1",
						Slug:    catalogSeriesSlug,
						RelPath: catalogSeriesSlug,
						Type:    access.LibraryTypeSeries,
					},
				}},
				metaStub{byPath: map[string]metadata.MetadataResponse{
					rel: {
						DisplayName: "S01E01",
						Effective: metadata.VideoFields{
							Show:         &show,
							EpisodeTitle: &epTitle,
							Season:       &season,
							Episode:      &episode,
						},
					},
				}},
			)

			user := auth.PublicUser{Role: auth.RoleAdmin}
			catalog, err := svc.GetLibraryCatalog(
				context.Background(),
				user,
				catalogSeriesSlug,
				mediafs.PageOpts{},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(catalog.Shows) != 1 || catalog.Shows[0].Name != show {
				t.Fatalf("shows=%+v", catalog.Shows)
			}
			detail, err := svc.GetShow(
				context.Background(),
				user,
				catalogSeriesSlug,
				catalog.Shows[0].ShowKey,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(detail.Seasons) != 1 || detail.Seasons[0].EpisodeCount != 1 {
				t.Fatalf("detail=%+v", detail)
			}
			episodes, err := svc.ListSeasonEpisodes(
				context.Background(),
				user,
				catalogSeriesSlug,
				catalog.Shows[0].ShowKey,
				1,
				mediafs.PageOpts{},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(episodes.Episodes) != 1 || episodes.Episodes[0].EpisodeTitle != epTitle {
				t.Fatalf("episodes=%+v", episodes)
			}

			_, err = svc.GetShow(context.Background(), user, catalogSeriesSlug, "no-such-show")
			if !errors.Is(err, ErrShowNotFound) {
				t.Fatalf("want ErrShowNotFound, got %v", err)
			}
		},
	)

	allure.Test(t, "catalog only sees indexed paths (not unindexed disk files)", func(a *allure.Context) {
		t := a.T()
		svc := NewService(
			PathCatalogIndex{
				LibraryType: access.LibraryTypeFilm,
				ByLibrary: map[string][]string{
					"1": {catalogMoviesSlug + "/Keep.mkv"},
				},
			},
			stubAccess{libraries: []access.Library{
				{
					ID:      "1",
					Slug:    catalogMoviesSlug,
					RelPath: catalogMoviesSlug,
					Type:    access.LibraryTypeFilm,
				},
			}},
			nil,
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
		if len(catalog.Movies) != 1 {
			t.Fatalf("movies=%d %+v", len(catalog.Movies), catalog.Movies)
		}
	})
}
