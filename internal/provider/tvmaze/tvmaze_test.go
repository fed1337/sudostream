package tvmaze_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/tvmaze"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestMatch_SearchUnique(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvmaze matches unique search hits for show and film hints", func(a *allure.Context) {
		t := a.T()
		const wantTitle = "Girls"
		const wantID = "82"
		server := newTVmazeServer(t)
		adapter := tvmaze.New(
			tvmaze.WithBaseURL(server.URL),
			tvmaze.WithMinInterval(0),
			tvmaze.WithHTTPClient(server.Client()),
		)

		showStatus, showFields, showErr := adapter.MatchShow(t.Context(), provider.ShowHint{Show: wantTitle})
		assertMatchOK(t, "show", showStatus, showErr, showFields.Title, showFields.IDs.TvmazeID, wantTitle, wantID)

		filmStatus, filmFields, filmErr := adapter.MatchFilm(t.Context(), provider.FilmHint{Title: wantTitle})
		assertMatchOK(t, "film", filmStatus, filmErr, filmFields.Title, filmFields.IDs.TvmazeID, wantTitle, wantID)
	})
}

func assertMatchOK(
	t *testing.T,
	kind string,
	status provider.MatchStatus,
	err error,
	title, tvmazeID *string,
	wantTitle, wantID string,
) {
	t.Helper()
	if err != nil || status != provider.MatchOK {
		t.Fatalf("%s status=%v err=%v", kind, status, err)
	}
	if title == nil || *title != wantTitle {
		t.Fatalf("%s title: %+v", kind, title)
	}
	if tvmazeID == nil || *tvmazeID != wantID {
		t.Fatalf("%s tvmaze id: %+v", kind, tvmazeID)
	}
}

func TestFetchEpisodes_MapsSeasonNumber(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvmaze episode catalog maps season/number to titles", func(a *allure.Context) {
		t := a.T()
		server := newTVmazeServer(t)
		adapter := tvmaze.New(
			tvmaze.WithBaseURL(server.URL),
			tvmaze.WithMinInterval(0),
			tvmaze.WithHTTPClient(server.Client()),
		)
		id := "82"
		episodes, err := adapter.FetchEpisodes(t.Context(), provider.ShowFields{
			IDs: provider.ExternalIDs{TvmazeID: &id},
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

func TestMatchShow_LookupByImdb(t *testing.T) {
	t.Parallel()

	allure.Test(t, "tvmaze resolves show via imdb lookup", func(a *allure.Context) {
		t := a.T()
		server := newTVmazeServer(t)
		adapter := tvmaze.New(
			tvmaze.WithBaseURL(server.URL),
			tvmaze.WithMinInterval(0),
			tvmaze.WithHTTPClient(server.Client()),
		)
		imdb := "tt0944947"
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{
			Show: "ignored",
			IDs:  provider.ExternalIDs{ImdbID: &imdb},
		})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.Title == nil || *fields.Title != "Game of Thrones" {
			t.Fatalf("title: %+v", fields.Title)
		}
	})
}

func newTVmazeServer(t *testing.T) *httptest.Server {
	t.Helper()

	const jsonName = "name"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/search/shows":
			_ = json.NewEncoder(writer).Encode([]map[string]any{
				{"score": 1, "show": sampleShow(82, "Girls", "2012-04-15", "tt1727824", 220411)},
			})
		case "/lookup/shows":
			_ = json.NewEncoder(writer).Encode(sampleShow(82, "Game of Thrones", "2011-04-17", "tt0944947", 121361))
		case "/shows/82/episodes":
			one := 1
			_ = json.NewEncoder(writer).Encode([]map[string]any{
				{"season": 1, "number": one, jsonName: "Pilot"},
				{"season": 1, "number": 2, jsonName: "Next"},
			})
		case "/shows/82":
			_ = json.NewEncoder(writer).Encode(sampleShow(82, "Girls", "2012-04-15", "tt1727824", 220411))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func sampleShow(id int, name, premiered, imdb string, tvdb int) map[string]any {
	return map[string]any{
		"id":        id,
		"name":      name,
		"summary":   "<p>A show about <b>people</b>.</p>",
		"genres":    []string{"Drama", "Comedy"},
		"premiered": premiered,
		"network":   map[string]any{"name": "HBO"},
		"externals": map[string]any{"thetvdb": tvdb, "imdb": imdb},
		"image":     map[string]any{"original": "http://example.test/poster.jpg"},
	}
}
