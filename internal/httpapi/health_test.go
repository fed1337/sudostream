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

func TestHealth_StatusMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name       string
		serve      func(t *testing.T) *httptest.ResponseRecorder
		wantStatus int
		wantField  string
	}{
		{
			name: "ok",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()

				return serveHealthRouter(newTestRouter(t, t.TempDir()))
			},
			wantStatus: http.StatusOK,
			wantField:  "ok",
		},
		{
			name: "degraded when media root missing",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()

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

				return serveHealthRouter(router)
			},
			wantStatus: http.StatusServiceUnavailable,
			wantField:  healthStatusDegraded,
		},
		{
			name: "degraded when db ping fails",
			serve: func(t *testing.T) *httptest.ResponseRecorder {
				t.Helper()

				media, err := mediafs.New(t.TempDir())
				if err != nil {
					t.Fatalf("new media service: %v", err)
				}

				mediaHandler := &handler{
					media: media,
					dbPing: func(context.Context) error {
						return errHealthTestDBDown
					},
				}

				router := gin.New()
				router.GET("/api/health", mediaHandler.health)

				return serveHealthRouter(router)
			},
			wantStatus: http.StatusServiceUnavailable,
			wantField:  healthStatusDegraded,
		},
	} {
		allure.Test(t, "health status matrix — "+tc.name, func(a *allure.Context) {
			t := a.T()
			recorder := tc.serve(t)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d", recorder.Code, tc.wantStatus)
			}

			var response HealthResponse
			err := json.NewDecoder(recorder.Body).Decode(&response)
			if err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Status != tc.wantField {
				t.Fatalf("status field: got %q want %q", response.Status, tc.wantField)
			}
			if response.Version != version.Version {
				t.Fatalf("version: got %q want %q", response.Version, version.Version)
			}
		})
	}
}

func serveHealthRouter(router *gin.Engine) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/health",
		nil,
	)
	router.ServeHTTP(recorder, request)

	return recorder
}
