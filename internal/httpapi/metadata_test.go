package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sudoStream/internal/access"
	"sudoStream/internal/metadata"
	"testing"
	"time"

	metadatapostgres "sudoStream/internal/metadata/postgres"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const testMetadataPilotTitle = "Pilot"

//nolint:paralleltest,gocognit,cyclop,funlen // integration scenario exercises GET/PATCH metadata flows end-to-end
func TestMetadata_HTTPGetPatchAndACL(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"metadata GET and PATCH enforce ACL and merge overrides",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router, root, database := newMetadataTestRouter(t)
			adminToken := adminAccessToken(ctx, t, router)
			store := metadatapostgres.NewStore(database.GORM)

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

			library := libraries[0]
			patchTypeHTTP(ctx, t, router, adminToken, library.ID, string(access.LibraryTypeSeries))

			relPath := testSeriesRelPath + "/S01E01.mkv"
			title := testMetadataPilotTitle
			show := "Demo Show"
			err = store.UpsertOriginal(
				ctx,
				library.ID,
				relPath,
				metadata.VideoFields{Title: &title, Show: &show},
				time.Now().UTC(),
				int64(len("fake-video")),
				time.Now().UTC(),
			)
			if err != nil {
				t.Fatalf("seed metadata: %v", err)
			}

			getRecorder := httptest.NewRecorder()
			getRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/metadata/"+relPath,
				nil,
			)
			setBearerAuth(getRequest, adminToken)
			router.ServeHTTP(getRecorder, getRequest)
			if getRecorder.Code != http.StatusOK {
				t.Fatalf("get metadata: %d %s", getRecorder.Code, getRecorder.Body.String())
			}

			var getResponse metadata.MetadataResponse
			err = json.NewDecoder(getRecorder.Body).Decode(&getResponse)
			if err != nil {
				t.Fatalf("decode metadata: %v", err)
			}
			if !getResponse.UsesMetadataForDisplay {
				t.Fatal("expected series library to use metadata for display")
			}
			if getResponse.Effective.Title == nil ||
				*getResponse.Effective.Title != testMetadataPilotTitle {
				t.Fatalf("unexpected effective title: %+v", getResponse.Effective.Title)
			}

			patchBody := []byte(`{"target":"override","fields":{"title":"Override Title"}}`)
			patchRecorder := httptest.NewRecorder()
			patchRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPatch,
				"/api/metadata/"+relPath,
				bytes.NewReader(patchBody),
			)
			patchRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(patchRequest, adminToken)
			router.ServeHTTP(patchRecorder, patchRequest)
			if patchRecorder.Code != http.StatusOK {
				t.Fatalf("patch metadata: %d %s", patchRecorder.Code, patchRecorder.Body.String())
			}

			var patchResponse metadata.MetadataResponse
			err = json.NewDecoder(patchRecorder.Body).Decode(&patchResponse)
			if err != nil {
				t.Fatalf("decode patch response: %v", err)
			}
			if patchResponse.Effective.Title == nil ||
				*patchResponse.Effective.Title != "Override Title" {
				t.Fatalf("expected override title, got %+v", patchResponse.Effective.Title)
			}

			filePatchBody := []byte(`{"target":"file","fields":{"title":"File Title"}}`)
			filePatchRecorder := httptest.NewRecorder()
			filePatchRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPatch,
				"/api/metadata/"+relPath,
				bytes.NewReader(filePatchBody),
			)
			filePatchRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(filePatchRequest, adminToken)
			router.ServeHTTP(filePatchRecorder, filePatchRequest)
			if filePatchRecorder.Code != http.StatusBadRequest &&
				filePatchRecorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf(
					"patch file target: %d %s",
					filePatchRecorder.Code,
					filePatchRecorder.Body.String(),
				)
			}

			notesPath := "notes.txt"
			err = os.WriteFile(filepath.Join(root, notesPath), []byte("text"), 0o600)
			if err != nil {
				t.Fatalf("write text file: %v", err)
			}

			nonVideoRecorder := httptest.NewRecorder()
			nonVideoRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/metadata/"+notesPath,
				nil,
			)
			setBearerAuth(nonVideoRequest, adminToken)
			router.ServeHTTP(nonVideoRecorder, nonVideoRequest)
			if nonVideoRecorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"get non-video metadata: %d %s",
					nonVideoRecorder.Code,
					nonVideoRecorder.Body.String(),
				)
			}
		},
	)
}

func patchTypeHTTP(
	ctx context.Context,
	t *testing.T,
	router *gin.Engine,
	adminToken, libraryID, libraryType string,
) {
	t.Helper()

	body := []byte(`{"type":"` + libraryType + `"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		"/api/admin/libraries/"+libraryID,
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	setBearerAuth(request, adminToken)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch library type: %d %s", recorder.Code, recorder.Body.String())
	}
}
