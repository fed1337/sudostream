package anidb_test

import (
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/anidb"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	cowboyBebop   = "Cowboy Bebop"
	titlesXMLPath = "/titles.xml"
	titlesXML     = `<?xml version="1.0"?>
<animetitles>
  <anime aid="1">
    <title xml:lang="en" type="main">Cowboy Bebop</title>
  </anime>
</animetitles>`
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

func TestMatchShow_AndFetchEpisodes(t *testing.T) { //nolint:cyclop // field + episode assertions
	t.Parallel()

	allure.Test(t, "anidb MatchShow OK and FetchEpisodes maps episode 1 title", func(a *allure.Context) {
		t := a.T()
		server := newAniDBServer(t)
		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/anime-titles.xml"),
			anidb.WithCacheDir(t.TempDir()),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)

		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: cowboyBebop})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.Title == nil || *fields.Title != cowboyBebop {
			t.Fatalf("title: %+v", fields.Title)
		}
		if fields.IDs.AnidbID == nil || *fields.IDs.AnidbID != "1" {
			t.Fatalf("anidb id: %+v", fields.IDs.AnidbID)
		}
		if fields.Year == nil || *fields.Year != 1998 {
			t.Fatalf("year: %+v", fields.Year)
		}
		if len(fields.Genres) != 1 || fields.Genres[0] != "Action" {
			t.Fatalf("genres: %+v", fields.Genres)
		}

		episodes, epErr := adapter.FetchEpisodes(t.Context(), fields)
		if epErr != nil {
			t.Fatalf("episodes: %v", epErr)
		}
		info, ok := episodes[provider.EpisodeKey{Season: 1, Episode: 1}]
		if !ok || info.Title != "Asteroid Blues" {
			t.Fatalf("episode map: %+v", episodes)
		}
	})
}

func TestMatchShow_ByStoredAnidbID(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb resolves show via stored anidb id without titles search", func(a *allure.Context) {
		t := a.T()
		server := newAniDBServer(t)
		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/missing-titles.xml"),
			anidb.WithCacheDir(t.TempDir()),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		aid := "1"
		status, fields, err := adapter.MatchShow(t.Context(), provider.ShowHint{
			Show: "ignored",
			IDs:  provider.ExternalIDs{AnidbID: &aid},
		})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.ExternalID == nil || *fields.ExternalID != "1" {
			t.Fatalf("external id: %+v", fields.ExternalID)
		}
	})
}

func TestFetchShowPoster_DownloadsPicture(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb downloads poster from picture element", func(a *allure.Context) {
		t := a.T()
		server := newAniDBServer(t)
		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/anime-titles.xml"),
			anidb.WithCacheDir(filepath.Join(t.TempDir(), "cache")),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		status, art, err := adapter.FetchShowPoster(t.Context(), provider.ShowHint{Show: cowboyBebop})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if len(art.Bytes) == 0 || art.ExternalID == nil || *art.ExternalID != "1" {
			t.Fatalf("art: %+v", art)
		}
	})
}

func TestMatchFilm_AndPoster(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb MatchFilm and FetchFilmPoster succeed for unique title", func(a *allure.Context) {
		t := a.T()
		server := newAniDBServer(t)
		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/anime-titles.xml"),
			anidb.WithCacheDir(t.TempDir()),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		if adapter.Key() != "anidb" {
			t.Fatalf("key: %s", adapter.Key())
		}
		status, fields, err := adapter.MatchFilm(t.Context(), provider.FilmHint{Title: cowboyBebop})
		if err != nil || status != provider.MatchOK {
			t.Fatalf("status=%v err=%v", status, err)
		}
		if fields.Title == nil || *fields.Title != cowboyBebop {
			t.Fatalf("title: %+v", fields.Title)
		}
		filmStatus, art, filmErr := adapter.FetchFilmPoster(t.Context(), provider.FilmHint{Title: cowboyBebop})
		if filmErr != nil || filmStatus != provider.MatchOK || len(art.Bytes) == 0 {
			t.Fatalf("film poster status=%v err=%v art=%+v", filmStatus, filmErr, art)
		}
	})
}

func TestRequiredEnv(t *testing.T) {
	allure.Test(t, "anidb RequiredEnv lists client when unset", func(a *allure.Context) {
		t := a.T()
		t.Setenv(anidb.EnvClient, "")
		empty := anidb.New(anidb.WithMinInterval(0), anidb.WithClient(""))
		if got := empty.RequiredEnv(); len(got) != 1 || got[0] != anidb.EnvClient {
			t.Fatalf("RequiredEnv: %v", got)
		}
		filled := anidb.New(anidb.WithMinInterval(0), anidb.WithClient("sudostream"))
		if got := filled.RequiredEnv(); got != nil {
			t.Fatalf("RequiredEnv with client: %v", got)
		}
	})
}

func TestMatchShow_UncertainAndNone(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb titles search returns uncertain/none for ambiguous or missing titles", func(a *allure.Context) {
		t := a.T()
		dump := `<?xml version="1.0"?>
<animetitles>
  <anime aid="1"><title xml:lang="en" type="main">Cowboy Bebop</title></anime>
  <anime aid="2"><title xml:lang="en" type="main">Cowboy Bebop Knockin on Heavens Door</title></anime>
</animetitles>`
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == titlesXMLPath {
				_, _ = writer.Write([]byte(dump))

				return
			}
			writer.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+titlesXMLPath),
			anidb.WithCacheDir(t.TempDir()),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		uncertain, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: "Cowboy"})
		if err != nil || uncertain != provider.MatchUncertain {
			t.Fatalf("uncertain status=%v err=%v", uncertain, err)
		}
		none, _, noneErr := adapter.MatchShow(t.Context(), provider.ShowHint{Show: "Totally Unknown"})
		if noneErr != nil || none != provider.MatchNone {
			t.Fatalf("none status=%v err=%v", none, noneErr)
		}
	})
}

func TestTitles_CacheReuse(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb titles dump is reused from cache on second match", func(a *allure.Context) {
		t := a.T()
		hits := 0
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch {
			case request.URL.Path == "/anime-titles.xml":
				hits++
				_, _ = writer.Write([]byte(titlesXML))
			case request.URL.Query().Get("request") == "anime":
				writeAnime(writer, request.Host)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)

		cacheDir := t.TempDir()
		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/anime-titles.xml"),
			anidb.WithCacheDir(cacheDir),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		_, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: cowboyBebop})
		if err != nil {
			t.Fatalf("first match: %v", err)
		}
		// New provider instance shares disk cache.
		adapter2 := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithTitlesURL(server.URL+"/anime-titles.xml"),
			anidb.WithCacheDir(cacheDir),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		_, _, err = adapter2.MatchShow(t.Context(), provider.ShowHint{Show: cowboyBebop})
		if err != nil {
			t.Fatalf("second match: %v", err)
		}
		if hits != 1 {
			t.Fatalf("titles fetch hits=%d want 1", hits)
		}
	})
}

func TestGetAnime_NotFoundAndBanned(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb maps not-found and banned responses", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			aid := request.URL.Query().Get("aid")
			switch aid {
			case "404":
				_, _ = writer.Write([]byte(`<error>Anime not found</error>`))
			case "ban":
				_, _ = writer.Write([]byte(`<error>Banned</error>`))
			default:
				writer.WriteHeader(http.StatusForbidden)
			}
		}))
		t.Cleanup(server.Close)

		adapter := anidb.New(
			anidb.WithBaseURL(server.URL),
			anidb.WithMinInterval(0),
			anidb.WithHTTPClient(server.Client()),
			anidb.WithClient("testclient"),
		)
		missing := "404"
		status, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{AnidbID: &missing},
		})
		if err != nil || status != provider.MatchNone {
			t.Fatalf("not found status=%v err=%v", status, err)
		}
		banned := "ban"
		_, _, banErr := adapter.MatchShow(t.Context(), provider.ShowHint{
			IDs: provider.ExternalIDs{AnidbID: &banned},
		})
		if banErr == nil || !errors.Is(banErr, provider.ErrProviderUnavailable) {
			t.Fatalf("banned err=%v", banErr)
		}
	})
}

func TestTitles_GzipAndPlain(t *testing.T) {
	t.Parallel()

	allure.Test(t, "anidb titles dump accepts gzip and plain XML", func(a *allure.Context) {
		t := a.T()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/titles.gz":
				gz := gzip.NewWriter(writer)
				_, _ = gz.Write([]byte(titlesXML))
				_ = gz.Close()
			case titlesXMLPath:
				_, _ = writer.Write([]byte(titlesXML))
			case "/":
				writeAnime(writer, request.Host)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
		t.Cleanup(server.Close)

		// DisableCompression so gzip magic bytes reach decodeTitlesBody (production dump is .gz).
		rawClient := server.Client()
		if transport, ok := rawClient.Transport.(*http.Transport); ok {
			clone := transport.Clone()
			clone.DisableCompression = true
			rawClient = &http.Client{Transport: clone}
		} else {
			rawClient = &http.Client{
				Transport: &http.Transport{DisableCompression: true},
			}
		}

		for _, titlesPath := range []string{titlesXMLPath, "/titles.gz"} {
			adapter := anidb.New(
				anidb.WithBaseURL(server.URL),
				anidb.WithTitlesURL(server.URL+titlesPath),
				anidb.WithCacheDir(t.TempDir()),
				anidb.WithMinInterval(0),
				anidb.WithHTTPClient(rawClient),
				anidb.WithClient("testclient"),
			)
			status, _, err := adapter.MatchShow(t.Context(), provider.ShowHint{Show: cowboyBebop})
			if err != nil || status != provider.MatchOK {
				t.Fatalf("%s status=%v err=%v", titlesPath, status, err)
			}
		}
	})
}

func newAniDBServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/anime-titles.xml":
			_, _ = writer.Write([]byte(titlesXML))
		case request.URL.Path == "/poster.jpg":
			writer.Header().Set("Content-Type", "image/jpeg")
			_, _ = writer.Write(tinyJPEG)
		case request.URL.Query().Get("request") == "anime":
			writeAnime(writer, request.Host)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func writeAnime(writer http.ResponseWriter, host string) {
	pictureURL := "http://" + host + "/poster.jpg"
	body := `<?xml version="1.0"?>
<anime id="1" restricted="false">
  <titles><title xml:lang="en" type="main">Cowboy Bebop</title></titles>
  <description>Space bounty hunters.</description>
  <startdate>1998-04-03</startdate>
  <picture>` + pictureURL + `</picture>
  <categories><category><name>Action</name></category></categories>
  <episodes>
    <episode><epno type="1">1</epno><title xml:lang="en">Asteroid Blues</title></episode>
  </episodes>
</anime>`
	_, _ = writer.Write([]byte(body)) //nolint:gosec // G705: test stub XML, not user HTML
}
