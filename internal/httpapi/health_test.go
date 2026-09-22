package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/version"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

var errHealthTestDBDown = errors.New("health test db down")

//nolint:paralleltest // shares SUDOSTREAM_DATABASE_URL integration fixture
func TestHealth_ReturnsVersionAndOK(t *testing.T) {
	allure.Test(t, "Health returns version and ok", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		router := newTestRouter(t, root)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/health",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("unexpected status: got %d want %d", recorder.Code, http.StatusOK)
		}

		var response HealthResponse
		err := json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Status != "ok" {
			t.Fatalf("unexpected status field: %q", response.Status)
		}
		if response.Version != version.Version {
			t.Fatalf("unexpected version: got %q want %q", response.Version, version.Version)
		}
	})
}

//nolint:paralleltest // setupTestCacheEnv mutates package testCacheRoot
func TestHealth_ReturnsDegradedWhenMediaRootMissing(t *testing.T) {
	allure.Test(t, "Health returns degraded when media root is missing", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new media service: %v", err)
		}

		err = os.Remove(root)
		if err != nil {
			t.Fatalf("remove media root: %v", err)
		}

		setupTestCacheEnv(t)

		router := gin.New()
		RegisterRoutes(router, media, nil, nil, nil, nil, testRouteConfig())

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/health",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf(
				"unexpected status: got %d want %d",
				recorder.Code,
				http.StatusServiceUnavailable,
			)
		}

		var response HealthResponse
		err = json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Status != healthStatusDegraded {
			t.Fatalf("unexpected status field: %q", response.Status)
		}
		if response.Version != version.Version {
			t.Fatalf("unexpected version: got %q want %q", response.Version, version.Version)
		}
	})
}

//nolint:paralleltest // direct handler call; no shared env mutation but kept serial with sibling health tests
func TestHealth_ReturnsDegradedWhenDBPingFails(t *testing.T) {
	allure.Test(t, "Health returns degraded when db ping fails", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("new media service: %v", err)
		}

		mediaHandler := &handler{
			media: media,
			dbPing: func(context.Context) error {
				return errHealthTestDBDown
			},
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/health",
			nil,
		)

		mediaHandler.health(ctx)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d want %d", recorder.Code, http.StatusServiceUnavailable)
		}

		var response HealthResponse
		err = json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Status != healthStatusDegraded {
			t.Fatalf("status field: got %q want %q", response.Status, healthStatusDegraded)
		}
	})
}
