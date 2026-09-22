package tvdb_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/tvdb"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testShowTitle = "Game of Thrones"
	testFilmTitle = "Inception"
	testShowID    = "121361"
	testFilmID    = "145"
	loginPath     = "/login"
	searchPath    = "/search"
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

func TestMatch_SearchUnique(t *testing.T) { //nolint:cyclop // show+film field assertions
	t.Parallel()

	allure.Test(t, "tvdb matches unique search hits for show and film", func(a *allure.Context) {
		t := a.T()
		server := newTVDBServer(t)
		adapter := newTestAdapter(server)

		showStatus, showFields, showErr := adapter.MatchShow(t.Context(), provider.ShowHint{Show: testShowTitle})
		if showErr != nil || showStatus != provider.MatchOK {
			t.Fatalf("show status=%v err=%v", showStatus, showErr)
		}
		if showFields.Title == nil || *showFields.Title != testShowTitle {
			t.Fatalf("show title: %+v", showFields.Title)
		}
		if showFields.IDs.TvdbID == nil || *showFields.IDs.TvdbID != testShowID {
			t.Fatalf("show tvdb id: %+v", showFields.IDs.TvdbID)
		}
		if showFields.IDs.ImdbID == nil || *showFields.IDs.ImdbID != "tt0944947" {
			t.Fatalf("show imdb id: %+v", showFields.IDs.ImdbID)
		}
		if showFields.IDs.TmdbID == nil || *showFields.IDs.TmdbID != "1399" {
			t.Fatalf("show tmdb id: %+v", showFields.IDs.TmdbID)
		}
		if showFields.Studio == nil || *showFields.Studio != "HBO" {
			t.Fatalf("show studio: %+v", showFields.Studio)
		}

		filmStatus, filmFields, filmErr := adapter.MatchFilm(t.Context(), provider.FilmHint{Title: testFilmTitle})
		if filmErr != nil || filmStatus != provider.MatchOK {
			t.Fatalf("film status=%v err=%v", filmStatus, filmErr)
		}
		if filmFields.IDs.TvdbID == nil || *filmFields.IDs.TvdbID != testFilmID {
			t.Fatalf("film tvdb id: %+v", filmFields.IDs.TvdbID)
		}
		if filmFields.Studio == nil || *filmFields.Studio != "Warner Bros." {
			t.Fatalf("film studio: %+v", filmFields.Studio)
		}
	})
}

func TestMatchShow_ByStoredTvdbID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb resolves show via stored tvdb id", func(a *allure.Context) {
		t := a.T()
		server := newTVDBServer(t)
		adapter := newTestAdapter(server)
		id := testShowID
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{
			Show: "ignored",
			IDs:  provider.ExternalIDs{TvdbID: &id},
		})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.Title == nil || *fields.Title != testShowTitle {
			t.Fatalf("title: %+v", fields.Title)
		}
	})
}

func TestFetchEpisodes_MapsSeasonNumber(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb episode catalog maps season/number to titles", func(a *allure.Context) {
		t := a.T()
		server := newTVDBServer(t)
		adapter := newTestAdapter(server)
		id := testShowID
		episodes, err := adapter.FetchEpisodes(t.Context(), provider.ShowFields{
			IDs: provider.ExternalIDs{TvdbID: &id},
		})
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		info, ok := episodes[provider.EpisodeKey{Season: 1, Episode: 1}]
		if !ok || info.Title != "Winter Is Coming" {
			t.Fatalf("episode map: %+v", episodes)
		}
		info2, ok2 := episodes[provider.EpisodeKey{Season: 1, Episode: 3}]
		if !ok2 || info2.Title != "Lord Snow" {
			t.Fatalf("paginated episode: %+v", episodes)
		}
	})
}

func TestRequiredEnv_APIKeyOnly(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb RequiredEnv lists API key only", func(a *allure.Context) {
		t := a.T()
		adapter := tvdb.New()
		keys := adapter.RequiredEnv()
		if len(keys) != 1 || keys[0] != tvdb.EnvAPIKey {
			t.Fatalf("RequiredEnv: %v", keys)
		}
		if adapter.Key() != tvdb.Key {
			t.Fatalf("Key: %q", adapter.Key())
		}
	})
}

func TestFetchPoster_OK(t *testing.T) { //nolint:cyclop // show+film poster assertions
	t.Parallel()

	allure.Test(t, "tvdb downloads show and film posters", func(a *allure.Context) {
		t := a.T()
		server := newTVDBServer(t)
		adapter := newTestAdapter(server)

		id := testShowID
		showStatus, showArt, showErr := adapter.FetchShowPoster(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{TvdbID: &id},
		})
		if showErr != nil || showStatus != provider.MatchOK {
			t.Fatalf("show poster status=%v err=%v", showStatus, showErr)
		}
		if len(showArt.Bytes) == 0 || showArt.ExternalID == nil || *showArt.ExternalID != testShowID {
			t.Fatalf("show art: %+v", showArt)
		}

		filmID := testFilmID
		filmStatus, filmArt, filmErr := adapter.FetchFilmPoster(t.Context(), provider.FilmHint{
			IDs: provider.ExternalIDs{TvdbID: &filmID},
		})
		if filmErr != nil || filmStatus != provider.MatchOK {
			t.Fatalf("film poster status=%v err=%v", filmStatus, filmErr)
		}
		if len(filmArt.Bytes) == 0 || filmArt.ExternalID == nil || *filmArt.ExternalID != testFilmID {
			t.Fatalf("film art: %+v", filmArt)
		}
	})
}

func TestMatch_UncertainAmbiguousSearch(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb returns MatchUncertain for ambiguous search results", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch {
			case request.URL.Path == loginPath && request.Method == http.MethodPost:
				writeBody(writer, `{"data":{"token":"test-token"}}`)
			case request.URL.Path == searchPath:
				writeBody(writer, `{
					"data":[
						{"tvdb_id":"1","name":"A","type":"series","year":"2011"},
						{"tvdb_id":"2","name":"B","type":"series","year":"2011"}
					]
				}`)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)

		adapter := newTestAdapter(server)
		year := 2011
		status, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: "Dup", Year: &year})
		if err != nil || status != provider.MatchUncertain {
			t.Fatalf("status=%v err=%v", status, err)
		}
	})
}

func TestMatch_NoneEmptyQuery(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb returns MatchNone for empty show and film queries", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(newTVDBServer(t))
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

func TestLogin_FailureUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb login 401 maps to ErrProviderUnavailable", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == loginPath {
				writer.WriteHeader(http.StatusUnauthorized)

				return
			}
			writer.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		adapter := newTestAdapter(server)
		_, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: testShowTitle})
		if err == nil || !errors.Is(err, provider.ErrProviderUnavailable) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestMatch_APIForbidden(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb API 403 maps to ErrProviderUnavailable", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch {
			case request.URL.Path == loginPath && request.Method == http.MethodPost:
				writeBody(writer, `{"data":{"token":"test-token"}}`)
			default:
				writer.WriteHeader(http.StatusForbidden)
			}
		}))
		t.Cleanup(server.Close)

		adapter := newTestAdapter(server)
		_, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: testShowTitle})
		if err == nil || !errors.Is(err, provider.ErrProviderUnavailable) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestMatch_MissingCredentials(t *testing.T) {
	// No t.Parallel: clears credential env via t.Setenv.
	allure.Test(t, "tvdb returns ErrMissingCredentials without API key", func(a *allure.Context) {
		t := a.T()
		t.Setenv(tvdb.EnvAPIKey, "")
		t.Setenv(tvdb.EnvPIN, "")
		adapter := tvdb.New(tvdb.WithMinInterval(0), tvdb.WithCredentials("", ""))
		_, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: testShowTitle})
		if err == nil || !errors.Is(err, tvdb.ErrMissingCredentials) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestFetchEpisodes_EmptyAndExternalID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb FetchEpisodes empty page and external id fallback", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch {
			case request.URL.Path == loginPath && request.Method == http.MethodPost:
				writeBody(writer, `{"data":{"token":"test-token"}}`)
			case strings.HasPrefix(request.URL.Path, "/series/empty/episodes/default"):
				writeBody(writer, `{"data":{"episodes":[]},"links":{"next":null}}`)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)

		adapter := newTestAdapter(server)
		ext := "empty"
		episodes, err := adapter.FetchEpisodes(t.Context(), provider.ShowFields{ExternalID: &ext})
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if len(episodes) != 0 {
			t.Fatalf("want empty map, got %+v", episodes)
		}

		_, missErr := adapter.FetchEpisodes(t.Context(), provider.ShowFields{})
		if missErr == nil || !errors.Is(missErr, tvdb.ErrMissingShowID) {
			t.Fatalf("missing id err=%v", missErr)
		}
	})
}

func TestMatchFilm_ByStoredIDAndYear(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb resolves film by id and year-filtered search", func(a *allure.Context) {
		t := a.T()
		server := newTVDBServer(t)
		adapter := newTestAdapter(server)

		id := testFilmID
		status, fields, err := adapter.MatchFilm(t.Context(), provider.FilmHint{
			IDs: provider.ExternalIDs{TvdbID: &id},
		})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("by-id status=%v err=%v", status, err)
		}
		if fields.Title == nil || *fields.Title != testFilmTitle {
			t.Fatalf("title: %+v", fields.Title)
		}

		year := 2011
		searchStatus, searchFields, searchErr := adapter.MatchShow(t.Context(), provider.ShowHint{
			Show: testShowTitle,
			Year: &year,
		})
		if searchErr != nil || searchStatus != provider.MatchOK {
			t.Fatalf("year search status=%v err=%v", searchStatus, searchErr)
		}
		if searchFields.IDs.TvdbID == nil || *searchFields.IDs.TvdbID != testShowID {
			t.Fatalf("year search id: %+v", searchFields.IDs.TvdbID)
		}
	})
}

func TestMatch_NotFoundID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvdb returns MatchNone for unknown stored ids", func(a *allure.Context) {
		t := a.T()
		adapter := newTestAdapter(newTVDBServer(t))
		missing := "999999"
		showStatus, _, showErr := adapter.MatchShow(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{TvdbID: &missing},
		})
		if showErr != nil || showStatus != provider.MatchNone {
			t.Fatalf("show status=%v err=%v", showStatus, showErr)
		}
	})
}

func TestMatch_NumericSearchID(t *testing.T) { //nolint:cyclop // artwork + remote id assertions
	t.Parallel()

	allure.Test(t, "tvdb accepts numeric tvdb_id in search hits", func(a *allure.Context) {
		t := a.T()
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			handleNumericSearch(writer, request, server)
		}))
		t.Cleanup(server.Close)

		adapter := tvdb.New(
			tvdb.WithBaseURL(server.URL),
			tvdb.WithMinInterval(0),
			tvdb.WithHTTPClient(server.Client()),
			tvdb.WithCredentials("test-key", "pin"),
		)
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: testShowTitle})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.IDs.ImdbID == nil || *fields.IDs.ImdbID != "tt0944947" {
			t.Fatalf("imdb: %+v", fields.IDs.ImdbID)
		}
		if fields.IDs.TmdbID == nil || *fields.IDs.TmdbID != "1399" {
			t.Fatalf("tmdb: %+v", fields.IDs.TmdbID)
		}
		if fields.Studio == nil || *fields.Studio != "HBO" {
			t.Fatalf("studio: %+v", fields.Studio)
		}

		showID := testShowID
		artStatus, art, artErr := adapter.FetchShowPoster(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{TvdbID: &showID},
		})
		if artErr != nil || artStatus != provider.MatchOK || len(art.Bytes) == 0 {
			t.Fatalf("artwork poster status=%v err=%v", artStatus, artErr)
		}
	})
}

func handleNumericSearch(writer http.ResponseWriter, request *http.Request, server *httptest.Server) {
	switch {
	case request.URL.Path == loginPath && request.Method == http.MethodPost:
		writeBody(writer, `{"data":{"token":"t"}}`)
	case request.URL.Path == searchPath:
		writeBody(writer, `{
			"data":[{"tvdb_id":121361,"id":null,"name":"Game of Thrones","type":"series","year":"2011"}]
		}`)
	case request.URL.Path == "/series/"+testShowID+"/extended":
		writer.WriteHeader(http.StatusNotFound)
	case request.URL.Path == "/series/"+testShowID:
		writeBody(writer, `{
			"data":{
				"id":121361,
				"name":"Game of Thrones",
				"overview":"Winter is coming.",
				"year":"2011",
				"artworks":[{"image":"`+server.URL+`/poster.jpg","type":2}],
				"remoteIds":[
					{"id":"tt0944947","type":2,"sourceName":"IMDb"},
					{"id":"","type":4,"sourceName":"TheMovieDB"},
					{"id":"1399","type":4,"sourceName":"TMDB"}
				],
				"genres":[{"name":"Fantasy"}],
				"companies":[{"name":"HBO"}]
			}
		}`)
	case request.URL.Path == "/poster.jpg":
		writer.Header().Set("Content-Type", "image/jpeg")
		_, _ = writer.Write(tinyJPEG)
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func newTestAdapter(server *httptest.Server) *tvdb.Provider {
	return tvdb.New(
		tvdb.WithBaseURL(server.URL),
		tvdb.WithMinInterval(0),
		tvdb.WithHTTPClient(server.Client()),
		tvdb.WithCredentials("test-key", ""),
	)
}

func newTVDBServer(t *testing.T) *httptest.Server { //nolint:cyclop // multi-route stub
	t.Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == loginPath && request.Method == http.MethodPost:
			writeBody(writer, `{"data":{"token":"test-token"}}`)
		case request.URL.Path == searchPath:
			writeSearch(writer, request.URL.Query().Get("type"))
		case request.URL.Path == "/series/"+testShowID+"/extended" ||
			request.URL.Path == "/series/"+testShowID:
			writeBody(writer, `{
				"data":{
					"id":121361,
					"name":"Game of Thrones",
					"overview":"Winter is coming.",
					"year":"2011",
					"image":"`+server.URL+`/poster.jpg",
					"remoteIds":[
						{"id":"tt0944947","type":2,"sourceName":"IMDB"},
						{"id":"1399","type":4,"sourceName":"TheMovieDB"}
					],
					"genres":[{"name":"Fantasy"}],
					"originalNetwork":{"name":"HBO"}
				}
			}`)
		case request.URL.Path == "/series/"+testShowID+"/episodes/default":
			writeEpisodePages(writer, server, request.URL.Query().Get("page"))
		case request.URL.Path == "/movies/"+testFilmID+"/extended" ||
			request.URL.Path == "/movies/"+testFilmID:
			writeBody(writer, `{
				"data":{
					"id":145,
					"name":"Inception",
					"overview":"Dreams within dreams.",
					"year":"2010",
					"image":"`+server.URL+`/film.jpg",
					"remoteIds":[{"id":"tt1375666","type":2,"sourceName":"IMDB"}],
					"companies":[{"name":"Warner Bros."}]
				}
			}`)
		case request.URL.Path == "/poster.jpg", request.URL.Path == "/film.jpg":
			writer.Header().Set("Content-Type", "image/jpeg")
			_, _ = writer.Write(tinyJPEG)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func writeEpisodePages(writer http.ResponseWriter, server *httptest.Server, page string) {
	if page == "0" || page == "" {
		next := server.URL + "/series/" + testShowID + "/episodes/default?page=1"
		writeBody(writer, `{
			"data":{"episodes":[
				{"seasonNumber":1,"number":1,"name":"Winter Is Coming"},
				{"seasonNumber":1,"number":2,"name":"The Kingsroad"},
				{"seasonNumber":-1,"number":1,"name":"skip"},
				{"seasonNumber":1,"number":0,"name":"skip2"},
				{"seasonNumber":1,"number":99,"name":""}
			]},
			"links":{"next":"`+next+`"}
		}`)

		return
	}
	writeBody(writer, `{
		"data":{"episodes":[
			{"seasonNumber":1,"number":3,"name":"Lord Snow"}
		]},
		"links":{"next":null}
	}`)
}

func writeSearch(writer http.ResponseWriter, entityType string) {
	if entityType == "movie" {
		writeBody(writer, `{
			"data":[{
				"tvdb_id":"145",
				"name":"Inception",
				"type":"movie",
				"year":"2010",
				"image_url":"http://example/inc.jpg"
			}]
		}`)

		return
	}
	writeBody(writer, `{
		"data":[{
			"tvdb_id":"121361",
			"name":"Game of Thrones",
			"type":"series",
			"year":"2011",
			"image_url":"http://example/got.jpg"
		}]
	}`)
}

func writeBody(writer http.ResponseWriter, payload string) {
	_, _ = writer.Write([]byte(payload))
}
