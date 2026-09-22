package tmdb_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/tmdb"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	showTitle     = "Breaking Bad"
	showID        = "1396"
	filmTitle     = "Inception"
	filmID        = "27205"
	jsonName      = "name"
	imdbShow      = "tt0903747"
	imdbFilm      = "tt1375666"
	tvdbShow      = "81189"
	resultsKey    = "results"
	firstAirDate  = "first_air_date"
	imdbIDKey     = "imdb_id"
	episodeNumber = "episode_number"
	tvResultsKey  = "tv_results"
)

var tinyJPEG = []byte{
	0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
	0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xff, 0xdb, 0x00, 0x43,
	0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
	0x09, 0x08, 0x0a, 0x0c, 0x14, 0x0d, 0x0c, 0x0b, 0x0b, 0x0c, 0x19, 0x12,
	0x13, 0x0f, 0x14, 0x1d, 0x1a, 0x1f, 0x1e, 0x1d, 0x1a, 0x1c, 0x1c, 0x20,
	0x24, 0x2e, 0x27, 0x20, 0x22, 0x2c, 0x23, 0x1c, 0x1c, 0x28, 0x37, 0x29,
	0x2c, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1f, 0x27, 0x39, 0x3d, 0x38, 0x32,
	0x3c, 0x2e, 0x33, 0x34, 0x32, 0xff, 0xc0, 0x00, 0x0b, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xff, 0xc4, 0x00, 0x14, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xff, 0xc4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xda, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3f, 0x00,
	0x7f, 0xff, 0xd9,
}

func TestMatch_SearchUnique(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb matches unique search hits for show and film", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)

		showStatus, showFields, showErr := adapter.MatchShow(t.Context(), provider.ShowHint{Show: showTitle})
		assertMatchOK(t, "show", showStatus, showErr, showFields.Title, showFields.IDs.TmdbID, showTitle, showID)

		filmStatus, filmFields, filmErr := adapter.MatchFilm(t.Context(), provider.FilmHint{Title: filmTitle})
		assertMatchOK(t, "film", filmStatus, filmErr, filmFields.Title, filmFields.IDs.TmdbID, filmTitle, filmID)
	})
}

func assertMatchOK(
	t *testing.T,
	kind string,
	status provider.MatchStatus,
	err error,
	title, tmdbID *string,
	wantTitle, wantID string,
) {
	t.Helper()
	if err != nil || status != provider.MatchOK {
		t.Fatalf("%s status=%v err=%v", kind, status, err)
	}
	if title == nil || *title != wantTitle {
		t.Fatalf("%s title: %+v", kind, title)
	}
	if tmdbID == nil || *tmdbID != wantID {
		t.Fatalf("%s tmdb id: %+v", kind, tmdbID)
	}
}

func TestFetchEpisodes_MapsSeasonNumber(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb episode catalog maps season/number to titles", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		id := showID
		episodes, err := adapter.FetchEpisodes(t.Context(), provider.ShowFields{
			IDs: provider.ExternalIDs{TmdbID: &id},
		})
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		info, ok := episodes[provider.EpisodeKey{Season: 1, Episode: 1}]
		if !ok || info.Title != "Pilot" {
			t.Fatalf("episode map: %+v", episodes)
		}
	})
}

func TestMatchShow_ByStoredTmdbID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb resolves show via stored tmdb id", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		id := showID
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{
			Show: "ignored",
			IDs:  provider.ExternalIDs{TmdbID: &id},
		})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.Title == nil || *fields.Title != showTitle {
			t.Fatalf("title: %+v", fields.Title)
		}
	})
}

func TestFetchPoster_OK(t *testing.T) { //nolint:cyclop // show+film poster assertions
	t.Parallel()

	allure.Test(t, "tmdb downloads show and film posters via image base URL", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		id := showID
		showStatus, showArt, showErr := adapter.FetchShowPoster(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{TmdbID: &id},
		})
		if showErr != nil || showStatus != provider.MatchOK {
			t.Fatalf("show poster status=%v err=%v", showStatus, showErr)
		}
		if len(showArt.Bytes) == 0 || showArt.ExternalID == nil || *showArt.ExternalID != showID {
			t.Fatalf("show art: %+v", showArt)
		}

		filmIDVal := filmID
		filmStatus, filmArt, filmErr := adapter.FetchFilmPoster(t.Context(), provider.FilmHint{
			IDs: provider.ExternalIDs{TmdbID: &filmIDVal},
		})
		if filmErr != nil || filmStatus != provider.MatchOK {
			t.Fatalf("film poster status=%v err=%v", filmStatus, filmErr)
		}
		if len(filmArt.Bytes) == 0 || filmArt.ExternalID == nil || *filmArt.ExternalID != filmID {
			t.Fatalf("film art: %+v", filmArt)
		}
	})
}

func TestMatch_UncertainAmbiguousSearch(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb returns MatchUncertain for ambiguous search results", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/search/tv" {
				_ = json.NewEncoder(writer).Encode(map[string]any{
					resultsKey: []map[string]any{
						{"id": 1, firstAirDate: "2008-01-01", jsonName: "A"},
						{"id": 2, firstAirDate: "2008-01-01", jsonName: "B"},
					},
				})

				return
			}
			writer.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		adapter := tmdb.New(
			tmdb.WithBaseURL(server.URL),
			tmdb.WithMinInterval(0),
			tmdb.WithHTTPClient(server.Client()),
			tmdb.WithCredentials("test-key", ""),
		)
		year := 2008
		status, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: "Dup", Year: &year})
		if err != nil || status != provider.MatchUncertain {
			t.Fatalf("status=%v err=%v", status, err)
		}
	})
}

func TestMatch_NoneEmptyQuery(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb returns MatchNone for empty show and film queries", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		showStatus, _, showErr := adapter.MatchShow(t.Context(), provider.ShowHint{})
		if showErr != nil || showStatus != provider.MatchNone {
			t.Fatalf("show status=%v err=%v", showStatus, showErr)
		}
		filmStatus, _, filmErr := adapter.MatchFilm(t.Context(), provider.FilmHint{})
		if filmErr != nil || filmStatus != provider.MatchNone {
			t.Fatalf("film status=%v err=%v", filmStatus, filmErr)
		}
	})
}

func TestMatch_FindByExternalIDs(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb finds show via tvdb id and film via imdb id", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)

		tvdbID := tvdbShow
		showStatus, showFields, showErr := adapter.MatchShow(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{TvdbID: &tvdbID},
		})
		assertMatchOK(t, "show-tvdb", showStatus, showErr, showFields.Title, showFields.IDs.TmdbID, showTitle, showID)

		imdbID := imdbFilm
		filmStatus, filmFields, filmErr := adapter.MatchFilm(t.Context(), provider.FilmHint{
			IDs: provider.ExternalIDs{ImdbID: &imdbID},
		})
		assertMatchOK(t, "film-imdb", filmStatus, filmErr, filmFields.Title, filmFields.IDs.TmdbID, filmTitle, filmID)
	})
}

func TestMatchShow_FindByImdbID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb finds show via imdb id find endpoint", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		imdbID := imdbShow
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{ImdbID: &imdbID},
		})
		assertMatchOK(t, "show-imdb", status, err, fields.Title, fields.IDs.TmdbID, showTitle, showID)
	})
}

func TestMatch_MissingCredentials(t *testing.T) {
	// No t.Parallel: clears credential env via t.Setenv.
	allure.Test(t, "tmdb returns ErrMissingCredentials without key or token", func(a *allure.Context) {
		t := a.T()
		t.Setenv(tmdb.EnvAPIKey, "")
		t.Setenv(tmdb.EnvAccessToken, "")
		adapter := tmdb.New(tmdb.WithMinInterval(0), tmdb.WithCredentials("", ""))
		_, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: showTitle})
		if err == nil || !errors.Is(err, tmdb.ErrMissingCredentials) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestMatch_ForbiddenUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb maps 403 to ErrProviderUnavailable", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusForbidden)
		}))
		t.Cleanup(server.Close)

		adapter := tmdb.New(
			tmdb.WithBaseURL(server.URL),
			tmdb.WithMinInterval(0),
			tmdb.WithHTTPClient(server.Client()),
			tmdb.WithCredentials("test-key", ""),
		)
		_, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: showTitle})
		if err == nil || !errors.Is(err, provider.ErrProviderUnavailable) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestRequiredEnvAndKey(t *testing.T) {
	allure.Test(t, "tmdb RequiredEnv and Key behavior", func(a *allure.Context) {
		t := a.T()
		t.Setenv(tmdb.EnvAPIKey, "")
		t.Setenv(tmdb.EnvAccessToken, "")
		empty := tmdb.New(tmdb.WithCredentials("", ""))
		if empty.Key() != tmdb.Key {
			t.Fatalf("Key: %q", empty.Key())
		}
		if got := empty.RequiredEnv(); len(got) != 1 || got[0] != tmdb.EnvAPIKey {
			t.Fatalf("RequiredEnv empty: %v", got)
		}
		filled := tmdb.New(tmdb.WithCredentials("key", ""))
		if got := filled.RequiredEnv(); got != nil {
			t.Fatalf("RequiredEnv with key: %v", got)
		}
		tokenOnly := tmdb.New(tmdb.WithCredentials("", "token"))
		if got := tokenOnly.RequiredEnv(); got != nil {
			t.Fatalf("RequiredEnv with token: %v", got)
		}
	})
}

func TestMatch_YearFilterAndExternalIDEpisodes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb year filter picks unique hit; episodes accept external id", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		year := 2008
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: showTitle, Year: &year})
		assertMatchOK(t, "show-year", status, err, fields.Title, fields.IDs.TmdbID, showTitle, showID)

		filmYear := 2010
		filmStatus, filmFields, filmErr := adapter.MatchFilm(t.Context(), provider.FilmHint{
			Title: filmTitle,
			Year:  &filmYear,
		})
		assertMatchOK(t, "film-year", filmStatus, filmErr, filmFields.Title, filmFields.IDs.TmdbID, filmTitle, filmID)

		ext := showID
		episodes, epErr := adapter.FetchEpisodes(t.Context(), provider.ShowFields{ExternalID: &ext})
		if epErr != nil {
			t.Fatalf("episodes: %v", epErr)
		}
		if _, ok := episodes[provider.EpisodeKey{Season: 1, Episode: 1}]; !ok {
			t.Fatalf("episodes: %+v", episodes)
		}
	})
}

func TestFetchEpisodes_MissingShowID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb FetchEpisodes requires show id", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		_, err := adapter.FetchEpisodes(t.Context(), provider.ShowFields{})
		if err == nil || !errors.Is(err, tmdb.ErrMissingShowID) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestMatch_BearerTokenAuth(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb authenticates with bearer access token only", func(a *allure.Context) {
		t := a.T()
		server := newTMDBServer(t)
		adapter := tmdb.New(
			tmdb.WithBaseURL(server.URL),
			tmdb.WithImageBaseURL(server.URL),
			tmdb.WithMinInterval(0),
			tmdb.WithHTTPClient(server.Client()),
			tmdb.WithCredentials("", "test-token"),
		)
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: showTitle})
		assertMatchOK(t, "bearer-show", status, err, fields.Title, fields.IDs.TmdbID, showTitle, showID)
	})
}

func TestMatch_NotFoundID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tmdb returns MatchNone for unknown stored ids", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(t)
		missing := "999999"
		showStatus, _, showErr := adapter.MatchShow(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{TmdbID: &missing},
		})
		if showErr != nil || showStatus != provider.MatchNone {
			t.Fatalf("show status=%v err=%v", showStatus, showErr)
		}
		filmStatus, _, filmErr := adapter.MatchFilm(t.Context(), provider.FilmHint{
			IDs: provider.ExternalIDs{TmdbID: &missing},
		})
		if filmErr != nil || filmStatus != provider.MatchNone {
			t.Fatalf("film status=%v err=%v", filmStatus, filmErr)
		}
	})
}

func newTestAdapter(t *testing.T) *tmdb.Provider {
	t.Helper()
	server := newTMDBServer(t)

	return tmdb.New(
		tmdb.WithBaseURL(server.URL),
		tmdb.WithImageBaseURL(server.URL),
		tmdb.WithMinInterval(0),
		tmdb.WithHTTPClient(server.Client()),
		tmdb.WithCredentials("test-key", ""),
	)
}

func newTMDBServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/search/tv":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				resultsKey: []map[string]any{
					{"id": 1396, firstAirDate: "2008-01-20", jsonName: showTitle},
				},
			})
		case request.URL.Path == "/search/movie":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				resultsKey: []map[string]any{
					{"id": 27205, "release_date": "2010-07-16", "title": filmTitle},
				},
			})
		case request.URL.Path == "/tv/1396":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id":           1396,
				jsonName:       showTitle,
				"overview":     "A chemistry teacher.",
				firstAirDate:   "2008-01-20",
				"poster_path":  "/bb.jpg",
				"genres":       []map[string]any{{jsonName: "Drama"}},
				"networks":     []map[string]any{{jsonName: "AMC"}},
				"seasons":      []map[string]any{{"season_number": -1}, {"season_number": 1}},
				"external_ids": map[string]any{imdbIDKey: imdbShow, "tvdb_id": 81189},
			})
		case request.URL.Path == "/tv/1396/season/1":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"episodes": []map[string]any{
					{episodeNumber: 1, jsonName: "Pilot"},
					{episodeNumber: 2, jsonName: "Cat's in the Bag..."},
					{episodeNumber: 0, jsonName: "skip"},
				},
			})
		case request.URL.Path == "/movie/27205":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"id":           27205,
				"title":        filmTitle,
				"overview":     "Dreams within dreams.",
				"release_date": "2010-07-16",
				"poster_path":  "/inc.jpg",
				"genres":       []map[string]any{{jsonName: "Action"}},
				"production_companies": []map[string]any{
					{jsonName: "Warner Bros."},
				},
				"external_ids": map[string]any{imdbIDKey: imdbFilm},
			})
		case strings.HasPrefix(request.URL.Path, "/find/"):
			writeFind(writer, request)
		case request.URL.Path == "/w780/bb.jpg", request.URL.Path == "/w780/inc.jpg":
			writer.Header().Set("Content-Type", "image/jpeg")
			_, _ = writer.Write(tinyJPEG)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func writeFind(writer http.ResponseWriter, request *http.Request) {
	source := request.URL.Query().Get("external_source")
	externalID := strings.TrimPrefix(request.URL.Path, "/find/")
	switch {
	case source == imdbIDKey && externalID == imdbShow:
		_, _ = writer.Write([]byte(`{"tv_results":[{"id":1396}],"movie_results":[]}`))
	case source == "tvdb_id" && externalID == tvdbShow:
		_, _ = writer.Write([]byte(`{"` + tvResultsKey + `":[{"id":1396}]}`))
	case source == imdbIDKey && externalID == imdbFilm:
		_, _ = writer.Write([]byte(`{"movie_results":[{"id":27205}],"tv_results":[]}`))
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}
