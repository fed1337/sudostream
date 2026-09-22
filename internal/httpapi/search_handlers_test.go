package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sudoStream/internal/metadata"
	"testing"
	"time"

	metadatapostgres "sudoStream/internal/metadata/postgres"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const sceneReleaseName = "[linuxisos.ru].friends.s01.e01.mkv"

//nolint:paralleltest // integration search + ACL
func TestSearch_SceneReleaseAndACL(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"search matches scene-release filename and respects library ACL",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router, root, database := newMetadataTestRouter(t)
			adminToken := adminAccessToken(ctx, t, router)

			err := os.MkdirAll(filepath.Join(root, "friends"), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(
				filepath.Join(root, "friends", sceneReleaseName),
				[]byte("fake"),
				0o600,
			)
			if err != nil {
				t.Fatalf("write video: %v", err)
			}

			library := createLibraryWithRootHTTP(ctx, t, router, "Series", "friends", "series")
			store := metadatapostgres.NewStore(database.GORM)
			relPath := "friends/" + sceneReleaseName
			err = store.UpsertOriginal(
				ctx,
				library.ID,
				relPath,
				metadata.VideoFields{},
				time.Now().UTC(),
				4,
				time.Now().UTC(),
			)
			if err != nil {
				t.Fatalf("seed metadata: %v", err)
			}

			hits := searchHTTP(ctx, t, router, adminToken, "friends")
			if len(hits) != 1 || hits[0].Path != relPath {
				t.Fatalf("admin hits=%+v", hits)
			}

			_, userToken := createUserSession(t, router, "search@hpserver.lan", "search-pass")
			denied := searchHTTP(ctx, t, router, userToken, "friends")
			if len(denied) != 0 {
				t.Fatalf("expected ACL to hide hits, got %+v", denied)
			}

			overrideTitle := "Central Perk"
			err = store.UpdateOverride(
				ctx,
				library.ID,
				relPath,
				metadata.StoredOverride{VideoFields: metadata.VideoFields{Title: &overrideTitle}},
				time.Now().UTC(),
				"admin",
			)
			if err != nil {
				t.Fatalf("override: %v", err)
			}

			titled := searchHTTP(ctx, t, router, adminToken, "Central Perk")
			if len(titled) != 1 {
				t.Fatalf("override title hits=%+v", titled)
			}
		},
	)
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAdmin_ExclusiveLibraryRoots(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"adding a descendant of another library root returns 409",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router, root := newACLTestRouter(t)
			token := adminAccessToken(ctx, t, router)

			err := os.MkdirAll(filepath.Join(root, "series", "nested"), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			_ = createLibraryWithRootHTTP(ctx, t, router, "Series", "series", "series")

			createBody := []byte(`{"name":"Other","type":"other"}`)
			createRecorder := httptest.NewRecorder()
			createRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/admin/libraries",
				bytes.NewReader(createBody),
			)
			createRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(createRequest, token)
			router.ServeHTTP(createRecorder, createRequest)
			if createRecorder.Code != http.StatusCreated {
				t.Fatalf("create: %d %s", createRecorder.Code, createRecorder.Body.String())
			}

			var created LibraryResponse
			err = json.NewDecoder(createRecorder.Body).Decode(&created)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			rootBody := []byte(`{"relPath":"series/nested"}`)
			rootRecorder := httptest.NewRecorder()
			rootRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/admin/libraries/"+created.Library.ID+"/roots",
				bytes.NewReader(rootBody),
			)
			rootRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(rootRequest, token)
			router.ServeHTTP(rootRecorder, rootRequest)
			if rootRecorder.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", rootRecorder.Code, rootRecorder.Body.String())
			}
		},
	)
}

func TestAdmin_LibraryFolderTreeRemoveAndDelete( //nolint:gocognit,cyclop,funlen,paralleltest
	t *testing.T,
) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"folder picker skips dot dirs; remove root and delete library leave files on disk",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router, root := newACLTestRouter(t)
			token := adminAccessToken(ctx, t, router)

			for _, name := range []string{testSeriesRelPath, "friends", "dumps"} {
				err := os.MkdirAll(filepath.Join(root, name), 0o750)
				if err != nil {
					t.Fatalf("mkdir %s: %v", name, err)
				}
			}
			err := os.MkdirAll(filepath.Join(root, ".trash"), 0o750)
			if err != nil {
				t.Fatalf("mkdir trash: %v", err)
			}

			library := createLibraryWithRootHTTP(
				ctx,
				t,
				router,
				"Series",
				testSeriesRelPath,
				testSeriesRelPath,
			)

			treeRecorder := httptest.NewRecorder()
			treeRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/admin/folder-tree",
				nil,
			)
			setBearerAuth(treeRequest, token)
			router.ServeHTTP(treeRecorder, treeRequest)
			if treeRecorder.Code != http.StatusOK {
				t.Fatalf("folder-tree: %d %s", treeRecorder.Code, treeRecorder.Body.String())
			}

			var tree FolderTreeResponse
			err = json.NewDecoder(treeRecorder.Body).Decode(&tree)
			if err != nil {
				t.Fatalf("decode tree: %v", err)
			}

			names := make([]string, 0, len(tree.Entries))
			assigned := ""
			for _, entry := range tree.Entries {
				names = append(names, entry.Name)
				if entry.Name == testSeriesRelPath {
					assigned = entry.AssignedLibraryID
				}
			}
			if assigned != library.ID {
				t.Fatalf("series assignment=%q want %q names=%v", assigned, library.ID, names)
			}
			for _, name := range names {
				if name == ".trash" {
					t.Fatal("folder picker listed .trash")
				}
			}

			removeRecorder := httptest.NewRecorder()
			removeRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodDelete,
				"/api/admin/libraries/"+library.ID+"/roots?path="+testSeriesRelPath,
				nil,
			)
			setBearerAuth(removeRequest, token)
			router.ServeHTTP(removeRecorder, removeRequest)
			if removeRecorder.Code != http.StatusNoContent {
				t.Fatalf(
					"remove last root: %d %s",
					removeRecorder.Code,
					removeRecorder.Body.String(),
				)
			}

			deleteRecorder := httptest.NewRecorder()
			deleteRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodDelete,
				"/api/admin/libraries/"+library.ID,
				nil,
			)
			setBearerAuth(deleteRequest, token)
			router.ServeHTTP(deleteRecorder, deleteRequest)
			if deleteRecorder.Code != http.StatusNotFound {
				t.Fatalf(
					"delete already-gone library: %d %s",
					deleteRecorder.Code,
					deleteRecorder.Body.String(),
				)
			}

			other := createLibraryWithRootHTTP(ctx, t, router, "Dumps", "dumps", "other")
			deleteOther := httptest.NewRecorder()
			deleteOtherReq := httptest.NewRequestWithContext(
				ctx,
				http.MethodDelete,
				"/api/admin/libraries/"+other.ID,
				nil,
			)
			setBearerAuth(deleteOtherReq, token)
			router.ServeHTTP(deleteOther, deleteOtherReq)
			if deleteOther.Code != http.StatusNoContent {
				t.Fatalf("delete library: %d %s", deleteOther.Code, deleteOther.Body.String())
			}

			_, statErr := os.Stat(filepath.Join(root, "dumps"))
			if statErr != nil {
				t.Fatalf("files should remain: %v", statErr)
			}
		},
	)
}

func searchHTTP(
	ctx context.Context,
	t *testing.T,
	router *gin.Engine,
	token, query string,
) []metadata.SearchHit {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/api/search?q="+url.QueryEscape(query),
		nil,
	)
	setBearerAuth(request, token)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var response SearchResponse
	err := json.NewDecoder(recorder.Body).Decode(&response)
	if err != nil {
		t.Fatalf("decode search: %v", err)
	}

	return response.Items
}
