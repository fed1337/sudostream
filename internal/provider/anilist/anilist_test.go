package anilist

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sudoStream/internal/provider"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	steinsGate     = "Steins;Gate"
	steinsGateSlug = "steins-gate"
)

// mediaJSON builds one AniList media node for the search payload.
func mediaJSON(id int, english, romaji string, year int, cover string) map[string]any {
	return map[string]any{
		"id":          id,
		"title":       map[string]any{"english": english, "romaji": romaji, "native": ""},
		"description": "A <b>lab</b> story.<br>Second line.",
		"genres":      []string{"Sci-Fi", "Thriller"},
		"seasonYear":  year,
		"startDate":   map[string]any{"year": year},
		"studios":     map[string]any{"nodes": []map[string]any{{"name": "White Fox"}}},
		"coverImage":  map[string]any{"extraLarge": cover, "large": cover},
	}
}

func searchServer(t *testing.T, nodes ...map[string]any) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "search") {
			t.Errorf("missing search variable in %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"Page": map[string]any{"media": nodes}},
		})
	})
	mux.HandleFunc("/cover.jpg", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg-bytes"))
	})

	return httptest.NewServer(mux)
}

func testProvider(server *httptest.Server) *Provider {
	return New(
		WithEndpoint(server.URL+"/graphql"),
		WithHTTPClient(server.Client()),
		WithMinInterval(0),
	)
}

func TestProvider_MatchShow_AppliesExactSingleHit(t *testing.T) { //nolint:cyclop // field assertions
	t.Parallel()

	allure.Test(t, "AniList exact single title hit maps show fields", func(a *allure.Context) {
		t := a.T()
		server := searchServer(t, mediaJSON(9253, steinsGate, steinsGateSlug, 2011, ""))
		defer server.Close()

		status, fields, err := testProvider(server).MatchShow(
			context.Background(),
			provider.ShowHint{Show: steinsGate},
		)
		if err != nil {
			t.Fatalf("match show: %v", err)
		}
		if status != provider.MatchOK {
			t.Fatalf("want MatchOK, got %s", status)
		}
		if fields.Title == nil || *fields.Title != steinsGate {
			t.Fatalf("title: %+v", fields.Title)
		}
		if fields.Description == nil ||
			!strings.HasPrefix(*fields.Description, "A lab story.") {
			t.Fatalf("want HTML stripped description, got %+v", fields.Description)
		}
		if fields.Studio == nil || *fields.Studio != "White Fox" {
			t.Fatalf("studio: %+v", fields.Studio)
		}
		if fields.Year == nil || *fields.Year != 2011 {
			t.Fatalf("year: %+v", fields.Year)
		}
		if len(fields.Genres) != 2 || fields.ExternalID == nil || *fields.ExternalID != "9253" {
			t.Fatalf("genres/id: %v %+v", fields.Genres, fields.ExternalID)
		}
	})
}

func TestProvider_Match_SkipsWhenNotConfident(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ambiguous, fuzzy, and empty results never apply", func(a *allure.Context) {
		t := a.T()

		cases := []struct {
			name  string
			nodes []map[string]any
			hint  provider.FilmHint
			want  provider.MatchStatus
		}{
			{
				name:  "no results",
				nodes: nil,
				hint:  provider.FilmHint{Title: "Unknown Movie"},
				want:  provider.MatchNone,
			},
			{
				name:  "fuzzy only",
				nodes: []map[string]any{mediaJSON(1, "Steins;Gate 0", "steins-gate-0", 2018, "")},
				hint:  provider.FilmHint{Title: steinsGate},
				want:  provider.MatchUncertain,
			},
			{
				name: "ambiguous exact",
				nodes: []map[string]any{
					mediaJSON(1, steinsGate, steinsGateSlug, 2011, ""),
					mediaJSON(2, steinsGate, steinsGateSlug, 2018, ""),
				},
				hint: provider.FilmHint{Title: steinsGate},
				want: provider.MatchUncertain,
			},
			{
				name:  "year mismatch",
				nodes: []map[string]any{mediaJSON(1, steinsGate, steinsGateSlug, 2011, "")},
				hint:  provider.FilmHint{Title: steinsGate, Year: new(1999)},
				want:  provider.MatchUncertain,
			},
			{
				name:  "empty title never calls api",
				nodes: nil,
				hint:  provider.FilmHint{Title: "   "},
				want:  provider.MatchNone,
			},
		}

		for _, testCase := range cases {
			server := searchServer(t, testCase.nodes...)
			status, fields, err := testProvider(server).MatchFilm(
				context.Background(),
				testCase.hint,
			)
			server.Close()

			if err != nil {
				t.Fatalf("%s: %v", testCase.name, err)
			}
			if status != testCase.want {
				t.Fatalf("%s: want %s, got %s", testCase.name, testCase.want, status)
			}
			if fields.Title != nil {
				t.Fatalf("%s: want no fields on non-OK match", testCase.name)
			}
		}
	})
}

func TestProvider_FetchPoster_DownloadsCoverForConfidentMatch(t *testing.T) {
	t.Parallel()

	allure.Test(t, "poster fetch downloads cover art after an exact match", func(a *allure.Context) {
		t := a.T()
		server := searchServer(t)
		defer server.Close()

		cover := server.URL + "/cover.jpg"
		hitServer := searchServer(t, mediaJSON(9253, steinsGate, steinsGateSlug, 2011, cover))
		defer hitServer.Close()

		status, art, err := testProvider(hitServer).FetchShowPoster(
			context.Background(),
			provider.ShowHint{Show: steinsGate},
		)
		if err != nil {
			t.Fatalf("fetch poster: %v", err)
		}
		if status != provider.MatchOK || string(art.Bytes) != "jpeg-bytes" {
			t.Fatalf("status=%s bytes=%q", status, art.Bytes)
		}
		if art.ContentType != "image/jpeg" || art.ExternalID == nil {
			t.Fatalf("content type/id: %q %+v", art.ContentType, art.ExternalID)
		}

		// A confident match without cover art is an error, not a silent empty poster.
		noCover := searchServer(t, mediaJSON(1, steinsGate, steinsGateSlug, 2011, ""))
		defer noCover.Close()

		_, _, err = testProvider(noCover).FetchFilmPoster(
			context.Background(),
			provider.FilmHint{Title: steinsGate},
		)
		if !errors.Is(err, ErrPosterEmpty) {
			t.Fatalf("want ErrPosterEmpty, got %v", err)
		}
	})
}

func TestProvider_Post_SurfacesTransportFailures(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HTTP and GraphQL errors are reported, 429 is retried", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()

		graphqlErr := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Too Many Requests."}]}`))
			}))
		defer graphqlErr.Close()

		_, _, err := testProvider(graphqlErr).MatchFilm(ctx, provider.FilmHint{Title: "x"})
		if !errors.Is(err, ErrGraphQLError) {
			t.Fatalf("want ErrGraphQLError, got %v", err)
		}

		serverErr := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
		defer serverErr.Close()

		_, _, err = testProvider(serverErr).MatchFilm(ctx, provider.FilmHint{Title: "x"})
		if !errors.Is(err, ErrRequestFailed) {
			t.Fatalf("want ErrRequestFailed, got %v", err)
		}

		forbidden := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, request *http.Request) {
				if request.Header.Get("User-Agent") == "" {
					t.Fatal("anilist requests must send User-Agent")
				}
				w.WriteHeader(http.StatusForbidden)
			}))
		defer forbidden.Close()

		_, _, err = testProvider(forbidden).MatchFilm(ctx, provider.FilmHint{Title: "x"})
		if !errors.Is(err, provider.ErrProviderUnavailable) {
			t.Fatalf("want ErrProviderUnavailable for 403, got %v", err)
		}

		attempts := 0
		throttled := httptest.NewServer(http.HandlerFunc(
			func(writer http.ResponseWriter, _ *http.Request) {
				attempts++
				if attempts == 1 {
					writer.Header().Set("Retry-After", "0")
					writer.WriteHeader(http.StatusTooManyRequests)

					return
				}
				_, _ = writer.Write([]byte(`{"data":{"Page":{"media":[]}}}`))
			}))
		defer throttled.Close()

		status, _, err := testProvider(throttled).MatchFilm(ctx, provider.FilmHint{Title: "x"})
		if err != nil || status != provider.MatchNone {
			t.Fatalf("want retry to succeed: status=%s err=%v", status, err)
		}
		if attempts != 2 {
			t.Fatalf("want one retry after 429, got %d attempts", attempts)
		}
	})
}

func TestRetryAfterDuration_ClampsHeader(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Retry-After parsing falls back and clamps", func(a *allure.Context) {
		t := a.T()

		if got := retryAfterDuration("bogus"); got != time.Second {
			t.Fatalf("bogus header: %v", got)
		}
		if got := retryAfterDuration("30"); got != 30*time.Second {
			t.Fatalf("valid header: %v", got)
		}
		if got := retryAfterDuration("100000"); got != maxRetryAfter {
			t.Fatalf("clamp: %v", got)
		}
	})
}

func TestProvider_Throttle_SpacesCalls(t *testing.T) {
	t.Parallel()

	allure.Test(t, "client-side throttle delays the second AniList call", func(a *allure.Context) {
		t := a.T()
		instance := New(WithMinInterval(40 * time.Millisecond))

		started := time.Now()
		instance.throttle(context.Background())
		instance.throttle(context.Background())

		if time.Since(started) < 30*time.Millisecond {
			t.Fatal("want second call delayed by the throttle")
		}
	})
}
