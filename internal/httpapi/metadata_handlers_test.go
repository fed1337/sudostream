package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sudoStream/internal/access"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestMetadata_ServiceUnavailable(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"metadata routes report 503 when the service is not wired",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router := newAuthTestRouter(t)
			adminToken := adminAccessToken(ctx, t, router)

			getRecorder := httptest.NewRecorder()
			getRequest := httptest.NewRequestWithContext(
				ctx, http.MethodGet, "/api/metadata/series/S01E01.mkv", nil,
			)
			setBearerAuth(getRequest, adminToken)
			router.ServeHTTP(getRecorder, getRequest)
			if getRecorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("get metadata: %d %s", getRecorder.Code, getRecorder.Body.String())
			}

			patchRecorder := httptest.NewRecorder()
			patchRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPatch,
				"/api/metadata/series/S01E01.mkv",
				bytes.NewReader([]byte(`{"target":"override","fields":{}}`)),
			)
			patchRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(patchRequest, adminToken)
			router.ServeHTTP(patchRecorder, patchRequest)
			if patchRecorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("patch metadata: %d %s", patchRecorder.Code, patchRecorder.Body.String())
			}
		},
	)
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestMetadata_PatchBadRequests(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"patch metadata rejects malformed body, target, and values",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router, root, _ := newMetadataTestRouter(t)
			adminToken := adminAccessToken(ctx, t, router)

			err := os.MkdirAll(filepath.Join(root, testSeriesRelPath), 0o750)
			if err != nil {
				t.Fatalf("mkdir library: %v", err)
			}
			videoPath := filepath.Join(root, testSeriesRelPath, "S01E01.mkv")
			err = os.WriteFile(videoPath, []byte("fake-video"), 0o600)
			if err != nil {
				t.Fatalf("write video: %v", err)
			}

			registerMediaLibrariesHTTP(ctx, t, router, root)
			libraries := listLibraries(t, router)
			if len(libraries) == 0 {
				t.Fatal("expected library after sync")
			}
			patchTypeHTTP(
				ctx,
				t,
				router,
				adminToken,
				libraries[0].ID,
				string(access.LibraryTypeSeries),
			)

			relPath := testSeriesRelPath + "/S01E01.mkv"
			cases := []struct {
				name string
				body string
			}{
				{name: "malformed json", body: `{"target":`},
				{name: "invalid target", body: `{"target":"bogus","fields":{}}`},
				{
					name: "invalid patch value",
					body: `{"target":"override","fields":{"season":"nope"}}`,
				},
			}

			for _, testCase := range cases {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequestWithContext(
					ctx,
					http.MethodPatch,
					"/api/metadata/"+relPath,
					bytes.NewReader([]byte(testCase.body)),
				)
				request.Header.Set("Content-Type", "application/json")
				setBearerAuth(request, adminToken)
				router.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusBadRequest {
					t.Fatalf("%s: expected 400, got %d %s",
						testCase.name, recorder.Code, recorder.Body.String())
				}
			}
		},
	)
}
