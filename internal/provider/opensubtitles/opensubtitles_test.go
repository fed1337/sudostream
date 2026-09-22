package opensubtitles_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/opensubtitles"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestFetchSubtitle_HashMatch(t *testing.T) {
	t.Parallel()

	allure.Test(t, "opensubtitles fetches vtt for a unique hash match", func(a *allure.Context) {
		t := a.T()
		server := newOpenSubtitlesTestServer(t)
		adapter := opensubtitles.New(
			opensubtitles.WithBaseURL(server.URL),
			opensubtitles.WithHTTPClient(server.Client()),
			opensubtitles.WithMinInterval(0),
			opensubtitles.WithCredentials("key", "user", "pass"),
		)

		status, file, err := adapter.FetchSubtitle(t.Context(), provider.SubtitleHint{
			RelPath: "movies/a.mkv",
			Title:   "Sample",
		}, "en")
		if err != nil {
			t.Fatal(err)
		}
		if status != provider.MatchOK {
			t.Fatalf("status %s", status)
		}
		if file.Lang != "en" || len(file.Bytes) == 0 {
			t.Fatalf("file %+v", file)
		}
		prefixLen := min(20, len(file.Bytes))
		if string(file.Bytes[:6]) != "WEBVTT" {
			t.Fatalf("want WEBVTT conversion, got %q", file.Bytes[:prefixLen])
		}
	})
}

func newOpenSubtitlesTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/login" && request.Method == http.MethodPost:
			_ = json.NewEncoder(writer).Encode(map[string]string{"token": "tok"})
		case request.URL.Path == "/subtitles":
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"data": []map[string]any{
					{
						"attributes": map[string]any{
							"files": []map[string]any{{"file_id": 42}},
						},
					},
				},
			})
		case request.URL.Path == "/download" && request.Method == http.MethodPost:
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"link": "http://" + request.Host + "/file.srt",
			})
		case request.URL.Path == "/file.srt":
			_, _ = writer.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	return server
}
