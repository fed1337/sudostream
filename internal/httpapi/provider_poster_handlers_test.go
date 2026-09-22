package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sudoStream/internal/provider"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const posterTestPath = "movies/sample.mkv"

var errArtifactsBoom = errors.New("artifacts boom")

// stubArtifacts serves one canned artifact row.
type stubArtifacts struct {
	rows map[string]provider.Artifact
	err  error
}

func (s stubArtifacts) GetArtifactByPath(
	_ context.Context,
	relPath, _ string,
	_ *string,
) (provider.Artifact, error) {
	if s.err != nil {
		return provider.Artifact{}, s.err
	}

	row, ok := s.rows[relPath]
	if !ok {
		return provider.Artifact{}, provider.ErrNotFound
	}

	return row, nil
}

func (s stubArtifacts) ListArtifacts(
	_ context.Context,
	libraryID, _ string,
) ([]provider.Artifact, error) {
	if s.err != nil {
		return nil, s.err
	}

	rows := make([]provider.Artifact, 0, len(s.rows))
	for _, row := range s.rows {
		if row.LibraryID == libraryID {
			rows = append(rows, row)
		}
	}

	return rows, nil
}

func (s stubArtifacts) ListArtifactsByPath(
	_ context.Context,
	relPath, kind string,
) ([]provider.Artifact, error) {
	if s.err != nil {
		return nil, s.err
	}

	rows := make([]provider.Artifact, 0, len(s.rows))
	for _, row := range s.rows {
		if row.RelPath == relPath && row.Kind == kind {
			rows = append(rows, row)
		}
	}

	return rows, nil
}

func (s stubArtifacts) UpsertArtifact(
	_ context.Context,
	artifact provider.Artifact,
) (provider.Artifact, error) {
	return artifact, nil
}

func (s stubArtifacts) DeleteArtifact(_ context.Context, _ string) error { return nil }

func posterRequest(recorder *httptest.ResponseRecorder, relPath string) *gin.Context {
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/provider-poster/"+relPath,
		nil,
	)
	ctx.Params = gin.Params{{Key: "path", Value: "/" + relPath}}

	return ctx
}

func TestProviderPoster_ServesCachedFile(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cached provider poster is served from the provider cache", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cache, err := provider.NewCache(filepath.Join(t.TempDir(), "providers"))
		if err != nil {
			t.Fatalf("cache: %v", err)
		}
		cachePath := provider.PosterCachePath("lib-1", posterTestPath, "image/webp")
		err = cache.Write(cachePath, []byte("webp-poster"))
		if err != nil {
			t.Fatalf("write cache: %v", err)
		}

		mediaHandler := &handler{posters: &posterCache{
			artifacts: stubArtifacts{rows: map[string]provider.Artifact{
				posterTestPath: {
					ID:        "artifact-1",
					LibraryID: providerTestLibraryID,
					RelPath:   posterTestPath,
					Kind:      provider.ArtifactKindPoster,
					CachePath: cachePath,
				},
			}},
			cache: cache,
		}}

		recorder := httptest.NewRecorder()
		mediaHandler.providerPoster(posterRequest(recorder, posterTestPath))

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
		if recorder.Body.String() != "webp-poster" {
			t.Fatalf("body: got %q", recorder.Body.String())
		}
		if got := recorder.Header().Get("Content-Type"); got != "image/webp" {
			t.Fatalf("content-type: got %q", got)
		}
	})
}

func TestProviderPoster_MissingArtifactReturns404(t *testing.T) {
	t.Parallel()

	allure.Test(t, "uncached paths and unwired caches return 404 so clients fall back",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			cache, err := provider.NewCache(filepath.Join(t.TempDir(), "providers"))
			if err != nil {
				t.Fatalf("cache: %v", err)
			}

			cases := []*handler{
				{},
				{posters: &posterCache{artifacts: stubArtifacts{}, cache: cache}},
			}
			for _, mediaHandler := range cases {
				recorder := httptest.NewRecorder()
				mediaHandler.providerPoster(posterRequest(recorder, posterTestPath))

				if recorder.Code != http.StatusNotFound {
					t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
				}
			}
		})
}

func TestProviderPoster_StoreFailureReturns500(t *testing.T) {
	t.Parallel()

	allure.Test(t, "artifact lookup failures return 500", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cache, err := provider.NewCache(filepath.Join(t.TempDir(), "providers"))
		if err != nil {
			t.Fatalf("cache: %v", err)
		}

		mediaHandler := &handler{posters: &posterCache{
			artifacts: stubArtifacts{err: errArtifactsBoom},
			cache:     cache,
		}}

		recorder := httptest.NewRecorder()
		mediaHandler.providerPoster(posterRequest(recorder, posterTestPath))

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPosterCache_PosterPaths(t *testing.T) {
	t.Parallel()

	allure.Test(t, "poster path index reports cached paths for a library", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()

		index := &posterCache{artifacts: stubArtifacts{rows: map[string]provider.Artifact{
			posterTestPath: {LibraryID: providerTestLibraryID, RelPath: posterTestPath},
			"movies/b.mkv": {LibraryID: "lib-2", RelPath: "movies/b.mkv"},
		}}}

		paths, err := index.PosterPaths(ctx, "lib-1")
		if err != nil {
			t.Fatalf("poster paths: %v", err)
		}
		if _, ok := paths[posterTestPath]; !ok || len(paths) != 1 {
			t.Fatalf("want only lib-1 paths, got %v", paths)
		}

		_, err = (&posterCache{artifacts: stubArtifacts{err: errArtifactsBoom}}).
			PosterPaths(ctx, "lib-1")
		if !errors.Is(err, errArtifactsBoom) {
			t.Fatalf("want store error surfaced, got %v", err)
		}

		var nilIndex *posterCache
		paths, err = nilIndex.PosterPaths(ctx, "lib-1")
		if len(paths) != 0 || err != nil {
			t.Fatalf("nil index: %v %v", paths, err)
		}
	})
}
