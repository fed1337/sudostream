package fanart_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/fanart"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	likesKey = "likes"
	urlKey   = "url"
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

func TestFetchPoster_MatchNoneWithoutIDs(t *testing.T) {
	t.Parallel()

	allure.Test(t, "fanart returns MatchNone when TMDB/TVDB ids are missing", func(a *allure.Context) {
		t := a.T()
		adapter := fanart.New(
			fanart.WithMinInterval(0),
			fanart.WithCredentials("test-key", ""),
		)

		showStatus, _, showErr := adapter.FetchShowPoster(t.Context(), provider.ShowHint{Show: "Anything"})
		if showErr != nil || showStatus != provider.MatchNone {
			t.Fatalf("show status=%v err=%v", showStatus, showErr)
		}

		filmStatus, _, filmErr := adapter.FetchFilmPoster(t.Context(), provider.FilmHint{Title: "Anything"})
		if filmErr != nil || filmStatus != provider.MatchNone {
			t.Fatalf("film status=%v err=%v", filmStatus, filmErr)
		}
	})
}

func TestFetchPoster_DownloadsWhenIDsPresent(t *testing.T) { //nolint:cyclop // show+film assertions
	t.Parallel()

	allure.Test(t, "fanart downloads posters when TVDB/TMDB ids are present", func(a *allure.Context) {
		t := a.T()
		server := newFanartServer(t)
		adapter := fanart.New(
			fanart.WithBaseURL(server.URL),
			fanart.WithMinInterval(0),
			fanart.WithHTTPClient(server.Client()),
			fanart.WithCredentials("test-key", "client-key"),
		)

		tvdb := "121361"
		showStatus, showArt, showErr := adapter.FetchShowPoster(t.Context(), provider.ShowHint{
			Show: "Game of Thrones",
			IDs:  provider.ExternalIDs{TvdbID: &tvdb},
		})
		if showErr != nil || showStatus != provider.MatchOK {
			t.Fatalf("show status=%v err=%v", showStatus, showErr)
		}
		if len(showArt.Bytes) == 0 || showArt.ExternalID == nil || *showArt.ExternalID != tvdb {
			t.Fatalf("show art: %+v", showArt)
		}

		tmdb := "27205"
		filmStatus, filmArt, filmErr := adapter.FetchFilmPoster(t.Context(), provider.FilmHint{
			Title: "Inception",
			IDs:   provider.ExternalIDs{TmdbID: &tmdb},
		})
		if filmErr != nil || filmStatus != provider.MatchOK {
			t.Fatalf("film status=%v err=%v", filmStatus, filmErr)
		}
		if len(filmArt.Bytes) == 0 || filmArt.ExternalID == nil || *filmArt.ExternalID != tmdb {
			t.Fatalf("film art: %+v", filmArt)
		}
	})
}

func newFanartServer(t *testing.T) *httptest.Server {
	t.Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/tv/121361":
			if request.URL.Query().Get("api_key") == "" {
				writer.WriteHeader(http.StatusUnauthorized)

				return
			}
			if request.Header.Get("Client-Key") != "client-key" {
				writer.WriteHeader(http.StatusForbidden)

				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"name":       "Show",
				"thetvdb_id": "121361",
				"tvposter": []map[string]any{
					{"id": "1", urlKey: server.URL + "/poster.jpg", likesKey: "10"},
					{"id": "2", urlKey: server.URL + "/low.jpg", likesKey: "1"},
				},
			})
		case "/movies/27205":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"name":    "Film",
				"tmdb_id": "27205",
				"movieposter": []map[string]any{
					{"id": "1", urlKey: server.URL + "/m.jpg", likesKey: "5"},
				},
			})
		case "/poster.jpg", "/m.jpg", "/low.jpg":
			writer.Header().Set("Content-Type", "image/jpeg")
			_, _ = writer.Write(tinyJPEG)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return server
}
