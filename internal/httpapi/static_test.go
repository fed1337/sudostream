package httpapi

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

//nolint:paralleltest // shares SUDOSTREAM_DATABASE_URL integration fixture
func TestRegisterFrontend_ServesAssetAndSPAFallback(t *testing.T) {
	allure.Test(t, "frontend serves static asset and SPA fallback", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		frontend := newFrontendFixture(t)
		router := newFrontendTestRouter(t, frontend)

		assetRecorder := httptest.NewRecorder()
		assetRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/assets/app.js",
			nil,
		)
		router.ServeHTTP(assetRecorder, assetRequest)

		if assetRecorder.Code != http.StatusOK {
			t.Fatalf("asset status: got %d want %d", assetRecorder.Code, http.StatusOK)
		}
		if assetRecorder.Body.String() != "console.log('ok')" {
			t.Fatalf("unexpected asset body: %q", assetRecorder.Body.String())
		}

		spaRecorder := httptest.NewRecorder()
		spaRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/browse/movies",
			nil,
		)
		router.ServeHTTP(spaRecorder, spaRequest)

		if spaRecorder.Code != http.StatusOK {
			t.Fatalf("spa status: got %d want %d", spaRecorder.Code, http.StatusOK)
		}
		if spaRecorder.Body.String() != "<html>spa</html>" {
			t.Fatalf("unexpected spa body: %q", spaRecorder.Body.String())
		}
	})
}

//nolint:paralleltest // shares SUDOSTREAM_DATABASE_URL integration fixture
func TestRegisterFrontend_APIPathsStayNotFoundOnNoRoute(t *testing.T) {
	allure.Test(t, "unknown API path does not return SPA index", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		frontend := newFrontendFixture(t)
		router := newFrontendTestRouter(t, frontend)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/unknown",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status: got %d want %d", recorder.Code, http.StatusNotFound)
		}
	})
}

func TestRegisterFrontend_HeadAndPostMethods(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HEAD serves assets and POST returns 404", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		frontend := newFrontendFixture(t)
		router := gin.New()
		RegisterFrontend(router, frontend)

		headRecorder := httptest.NewRecorder()
		headRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodHead,
			"/assets/app.js",
			nil,
		)
		router.ServeHTTP(headRecorder, headRequest)
		if headRecorder.Code != http.StatusOK {
			t.Fatalf("HEAD status: got %d", headRecorder.Code)
		}

		postRecorder := httptest.NewRecorder()
		postRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/browse/movies",
			nil,
		)
		router.ServeHTTP(postRecorder, postRequest)
		if postRecorder.Code != http.StatusNotFound {
			t.Fatalf("POST status: got %d", postRecorder.Code)
		}
	})
}

func TestRegisterFrontend_ServesRootIndex(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET / returns index.html", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		frontend := newFrontendFixture(t)
		router := gin.New()
		RegisterFrontend(router, frontend)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d", recorder.Code)
		}
		if recorder.Body.String() != "<html>spa</html>" {
			t.Fatalf("body: got %q", recorder.Body.String())
		}
	})
}

func TestRegisterFrontend_MissingIndexReturns404(t *testing.T) {
	t.Parallel()

	allure.Test(t, "SPA fallback returns 404 when index.html is missing", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		dir := t.TempDir()
		router := gin.New()
		RegisterFrontend(router, os.DirFS(dir))

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/unknown-route",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func newFrontendFixture(t *testing.T) fs.FS {
	t.Helper()

	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>spa</html>"), 0o600)
	if err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	err = os.MkdirAll(filepath.Join(dir, "assets"), 0o750)
	if err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(dir, "assets", "app.js"),
		[]byte("console.log('ok')"),
		0o600,
	)
	if err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	return os.DirFS(dir)
}

func newFrontendTestRouter(t *testing.T, frontend fs.FS) *gin.Engine {
	t.Helper()

	router := newTestRouter(t, t.TempDir())
	RegisterFrontend(router, frontend)

	return router
}
