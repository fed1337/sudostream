package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const testSeriesRelPath = "series"

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAuth_DisabledUserCannotLogin(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "login blocked when account disabled", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router, _ := newACLTestRouter(t)
		ctx := context.Background()

		createBody := []byte(
			`{"email":"disabled@hpserver.lan","password":"secret-pass","role":"user"}`,
		)
		createRecorder := httptest.NewRecorder()
		createRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/admin/users",
			bytes.NewReader(createBody),
		)
		createRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(createRequest, adminAccessToken(ctx, t, router))
		router.ServeHTTP(createRecorder, createRequest)

		if createRecorder.Code != http.StatusCreated {
			t.Fatalf(
				"create user status: got %d body=%s",
				createRecorder.Code,
				createRecorder.Body.String(),
			)
		}

		var createResponse struct {
			User auth.AdminUser `json:"user"`
		}
		err := json.NewDecoder(createRecorder.Body).Decode(&createResponse)
		if err != nil {
			t.Fatalf("decode create response: %v", err)
		}

		err = disableUserViaAPI(t, router, createResponse.User.ID)
		if err != nil {
			t.Fatalf("disable user: %v", err)
		}

		loginBody := []byte(`{"email":"disabled@hpserver.lan","password":"secret-pass"}`)
		loginRecorder := httptest.NewRecorder()
		loginRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/login",
			bytes.NewReader(loginBody),
		)
		loginRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(loginRecorder, loginRequest)

		if loginRecorder.Code != http.StatusForbidden {
			t.Fatalf("login status: got %d want %d body=%s",
				loginRecorder.Code, http.StatusForbidden, loginRecorder.Body.String())
		}
	})
}

//nolint:paralleltest,cyclop,funlen // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAccess_NonAdminBrowseFilteredByGrants(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "non-admin sees only granted libraries at browse root", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router, mediaRoot := newACLTestRouter(t)
		ctx := context.Background()

		seriesDir := filepath.Join(mediaRoot, "series")
		filmsDir := filepath.Join(mediaRoot, "films")
		err := os.MkdirAll(seriesDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir series: %v", err)
		}
		err = os.MkdirAll(filmsDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir films: %v", err)
		}

		registerMediaLibrariesHTTP(ctx, t, router, mediaRoot)

		userID, userToken := createUserSession(t, router, "reader@hpserver.lan", "reader-pass")

		libraries := listLibraries(t, router)
		var seriesLibraryID string
		for _, library := range libraries {
			if library.RelPath == testSeriesRelPath {
				seriesLibraryID = library.ID
			}
		}
		if seriesLibraryID == "" {
			t.Fatal("series library not found after sync")
		}

		grantBody, err := json.Marshal(map[string]any{
			jsonKeyGrants: []map[string]any{
				{
					jsonKeyLibraryID: seriesLibraryID,
					jsonKeyPermissions: map[string]bool{
						"create":    false,
						jsonKeyRead: true,
						"update":    false,
						"delete":    false,
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal grants: %v", err)
		}

		grantRecorder := httptest.NewRecorder()
		grantRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPut,
			"/api/admin/users/"+userID+"/libraries",
			bytes.NewReader(grantBody),
		)
		grantRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(grantRequest, adminAccessToken(ctx, t, router))
		router.ServeHTTP(grantRecorder, grantRequest)
		if grantRecorder.Code != http.StatusOK {
			t.Fatalf("put grants: %d %s", grantRecorder.Code, grantRecorder.Body.String())
		}

		browseRecorder := httptest.NewRecorder()
		browseRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/browse",
			nil,
		)
		setBearerAuth(browseRequest, userToken)
		router.ServeHTTP(browseRecorder, browseRequest)
		if browseRecorder.Code != http.StatusOK {
			t.Fatalf("browse status: %d %s", browseRecorder.Code, browseRecorder.Body.String())
		}

		var browseResponse mediafs.BrowseResponse
		err = json.NewDecoder(browseRecorder.Body).Decode(&browseResponse)
		if err != nil {
			t.Fatalf("decode browse: %v", err)
		}
		if len(browseResponse.Folder.Children) != 1 {
			t.Fatalf("expected 1 child, got %d", len(browseResponse.Folder.Children))
		}
		if browseResponse.Folder.Children[0].Name != testSeriesRelPath {
			t.Fatalf("unexpected child: %+v", browseResponse.Folder.Children[0])
		}

		deniedRecorder := httptest.NewRecorder()
		deniedRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/browse/films",
			nil,
		)
		setBearerAuth(deniedRequest, userToken)
		router.ServeHTTP(deniedRecorder, deniedRequest)
		if deniedRecorder.Code != http.StatusForbidden {
			t.Fatalf(
				"denied browse status: got %d want %d",
				deniedRecorder.Code,
				http.StatusForbidden,
			)
		}
	})
}

//nolint:paralleltest,cyclop,funlen // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAccess_EncodedParentCannotBypassLibraryGrant(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "encoded ..%2f under granted library cannot reach ungranted library", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router, mediaRoot := newACLTestRouter(t)
		ctx := context.Background()

		seriesDir := filepath.Join(mediaRoot, "series")
		filmsDir := filepath.Join(mediaRoot, "films")
		err := os.MkdirAll(seriesDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir series: %v", err)
		}
		err = os.MkdirAll(filmsDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir films: %v", err)
		}
		secret := []byte("ungranted-film-bytes")
		err = os.WriteFile(filepath.Join(filmsDir, "secret.mp4"), secret, 0o600)
		if err != nil {
			t.Fatalf("write film: %v", err)
		}
		err = os.WriteFile(filepath.Join(seriesDir, "ok.mp4"), []byte("series-ok"), 0o600)
		if err != nil {
			t.Fatalf("write series: %v", err)
		}

		registerMediaLibrariesHTTP(ctx, t, router, mediaRoot)

		userID, userToken := createUserSession(t, router, "enc-bypass@hpserver.lan", "reader-pass")
		libraries := listLibraries(t, router)
		seriesLibraryID := libraryIDByRelPath(t, libraries, testSeriesRelPath)

		grantBody, err := json.Marshal(map[string]any{
			jsonKeyGrants: []map[string]any{
				{
					jsonKeyLibraryID: seriesLibraryID,
					jsonKeyPermissions: map[string]bool{
						"create":    false,
						jsonKeyRead: true,
						"update":    false,
						"delete":    false,
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("marshal grants: %v", err)
		}
		grantRecorder := httptest.NewRecorder()
		grantRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPut,
			"/api/admin/users/"+userID+"/libraries",
			bytes.NewReader(grantBody),
		)
		grantRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(grantRequest, adminAccessToken(ctx, t, router))
		router.ServeHTTP(grantRecorder, grantRequest)
		if grantRecorder.Code != http.StatusOK {
			t.Fatalf("put grants: %d %s", grantRecorder.Code, grantRecorder.Body.String())
		}

		// Control: direct ungranted path is forbidden.
		direct := httptest.NewRecorder()
		directReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/download/films/secret.mp4", nil)
		setBearerAuth(directReq, userToken)
		router.ServeHTTP(direct, directReq)
		if direct.Code != http.StatusForbidden {
			t.Fatalf("direct download: got %d want 403 body=%s", direct.Code, direct.Body.String())
		}

		// Granted series path still works.
		okRec := httptest.NewRecorder()
		okReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/download/series/ok.mp4", nil)
		setBearerAuth(okReq, userToken)
		router.ServeHTTP(okRec, okReq)
		if okRec.Code != http.StatusOK {
			t.Fatalf("granted download: got %d want 200 body=%s", okRec.Code, okRec.Body.String())
		}

		bypassPaths := []string{
			"/api/download/series/..%2ffilms/secret.mp4",
			"/api/download/series/%2e%2e/films/secret.mp4",
			"/api/download/series/%2e%2e%2ffilms/secret.mp4",
			"/api/stream/series/..%2ffilms/secret.mp4",
			"/api/browse/series/..%2ffilms",
		}
		for _, path := range bypassPaths {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
			setBearerAuth(req, userToken)
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf(
					"bypass %s: got %d want 403 body=%s",
					path,
					rec.Code,
					rec.Body.String(),
				)
			}
			if bytes.Contains(rec.Body.Bytes(), secret) {
				t.Fatalf("bypass %s leaked ungranted file body", path)
			}
		}

		delRec := httptest.NewRecorder()
		delReq := httptest.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			"/api/media/series/..%2ffilms/secret.mp4",
			nil,
		)
		setBearerAuth(delReq, userToken)
		router.ServeHTTP(delRec, delReq)
		if delRec.Code != http.StatusForbidden {
			t.Fatalf("delete bypass: got %d want 403", delRec.Code)
		}
		_, statErr := os.Stat(filepath.Join(filmsDir, "secret.mp4"))
		if statErr != nil {
			t.Fatalf("ungranted file must remain after delete bypass: %v", statErr)
		}
	})
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAccess_NoReadGrantsReturnsEmptyRootBrowse(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "non-admin without can_read sees empty browse root", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router, mediaRoot := newACLTestRouter(t)
		ctx := context.Background()

		err := os.MkdirAll(filepath.Join(mediaRoot, "series"), 0o750)
		if err != nil {
			t.Fatalf("mkdir series: %v", err)
		}

		registerMediaLibrariesHTTP(ctx, t, router, mediaRoot)

		_, userToken := createUserSession(t, router, "empty@hpserver.lan", "empty-pass")

		browseRecorder := httptest.NewRecorder()
		browseRequest := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/browse", nil)
		setBearerAuth(browseRequest, userToken)
		router.ServeHTTP(browseRecorder, browseRequest)

		if browseRecorder.Code != http.StatusOK {
			t.Fatalf("browse status: got %d want %d body=%s",
				browseRecorder.Code, http.StatusOK, browseRecorder.Body.String())
		}

		var browseResponse mediafs.BrowseResponse
		err = json.NewDecoder(browseRecorder.Body).Decode(&browseResponse)
		if err != nil {
			t.Fatalf("decode browse: %v", err)
		}
		if len(browseResponse.Folder.Children) != 0 {
			t.Fatalf("expected empty children, got %d", len(browseResponse.Folder.Children))
		}
	})
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAccess_SyncPreservesLibraryType(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"library sync preserves existing type and name on upsert",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			router, mediaRoot := newACLTestRouter(t)
			ctx := context.Background()

			err := os.MkdirAll(filepath.Join(mediaRoot, "series"), 0o750)
			if err != nil {
				t.Fatalf("mkdir series: %v", err)
			}

			registerMediaLibrariesHTTP(ctx, t, router, mediaRoot)

			libraries := listLibraries(t, router)
			seriesLibraryID := libraryIDByRelPath(t, libraries, testSeriesRelPath)

			patchBody := []byte(`{"type":"film","name":"TV Shows"}`)
			patchRecorder := httptest.NewRecorder()
			patchRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPatch,
				"/api/admin/libraries/"+seriesLibraryID,
				bytes.NewReader(patchBody),
			)
			patchRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(patchRequest, adminAccessToken(ctx, t, router))
			router.ServeHTTP(patchRecorder, patchRequest)
			if patchRecorder.Code != http.StatusOK {
				t.Fatalf("patch library: %d %s", patchRecorder.Code, patchRecorder.Body.String())
			}

			syncLibrariesHTTP(ctx, t, router)

			libraries = listLibraries(t, router)
			for _, library := range libraries {
				if library.ID == seriesLibraryID &&
					(library.Type != access.LibraryTypeFilm || library.Name != "TV Shows") {
					t.Fatalf("expected film/TV Shows after resync, got type=%q name=%q",
						library.Type, library.Name)
				}
			}
		},
	)
}

//nolint:paralleltest,cyclop // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAccess_ListReadableLibrariesForUser(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"user libraries endpoint returns only readable libraries",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			router, mediaRoot := newACLTestRouter(t)
			ctx := context.Background()

			err := os.MkdirAll(filepath.Join(mediaRoot, "series"), 0o750)
			if err != nil {
				t.Fatalf("mkdir series: %v", err)
			}
			err = os.MkdirAll(filepath.Join(mediaRoot, "films"), 0o750)
			if err != nil {
				t.Fatalf("mkdir films: %v", err)
			}

			registerMediaLibrariesHTTP(ctx, t, router, mediaRoot)

			userID, userToken := createUserSession(t, router, "libs@hpserver.lan", "libs-pass")

			libraries := listLibraries(t, router)
			var seriesLibraryID string
			for _, library := range libraries {
				if library.RelPath == testSeriesRelPath {
					seriesLibraryID = library.ID
				}
			}
			if seriesLibraryID == "" {
				t.Fatal("series library not found after sync")
			}

			grantBody, err := json.Marshal(map[string]any{
				jsonKeyGrants: []map[string]any{
					{
						jsonKeyLibraryID: seriesLibraryID,
						jsonKeyPermissions: map[string]bool{
							jsonKeyRead: true,
						},
					},
				},
			})
			if err != nil {
				t.Fatalf("marshal grants: %v", err)
			}

			grantRecorder := httptest.NewRecorder()
			grantRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPut,
				"/api/admin/users/"+userID+"/libraries",
				bytes.NewReader(grantBody),
			)
			grantRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(grantRequest, adminAccessToken(ctx, t, router))
			router.ServeHTTP(grantRecorder, grantRequest)
			if grantRecorder.Code != http.StatusOK {
				t.Fatalf("put grants: %d %s", grantRecorder.Code, grantRecorder.Body.String())
			}

			listRecorder := httptest.NewRecorder()
			listRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/libraries",
				nil,
			)
			setBearerAuth(listRequest, userToken)
			router.ServeHTTP(listRecorder, listRequest)
			if listRecorder.Code != http.StatusOK {
				t.Fatalf("list libraries: %d %s", listRecorder.Code, listRecorder.Body.String())
			}

			var listResponse struct {
				Libraries []access.Library `json:"libraries"`
			}
			err = json.NewDecoder(listRecorder.Body).Decode(&listResponse)
			if err != nil {
				t.Fatalf("decode libraries: %v", err)
			}
			if len(listResponse.Libraries) != 1 {
				t.Fatalf("expected 1 library, got %d", len(listResponse.Libraries))
			}
			if listResponse.Libraries[0].RelPath != testSeriesRelPath {
				t.Fatalf("unexpected library: %+v", listResponse.Libraries[0])
			}
			if listResponse.Libraries[0].Type != access.LibraryTypeOther {
				t.Fatalf("expected default type other, got %q", listResponse.Libraries[0].Type)
			}
		},
	)
}

func syncLibrariesHTTP(ctx context.Context, t *testing.T, router *gin.Engine) {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/admin/libraries/sync",
		nil,
	)
	setBearerAuth(request, adminAccessToken(ctx, t, router))
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("sync libraries: %d %s", recorder.Code, recorder.Body.String())
	}
}

func registerMediaLibrariesHTTP(
	ctx context.Context,
	t *testing.T,
	router *gin.Engine,
	mediaRoot string,
) {
	t.Helper()

	entries, err := os.ReadDir(mediaRoot)
	if err != nil {
		t.Fatalf("read media root: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		createLibraryWithRootHTTP(ctx, t, router, entry.Name(), entry.Name(), "other")
	}
}

func createLibraryWithRootHTTP(
	ctx context.Context,
	t *testing.T,
	router *gin.Engine,
	name, relPath, libraryType string,
) access.Library {
	t.Helper()

	token := adminAccessToken(ctx, t, router)
	if libraryType == "" {
		libraryType = "other"
	}
	body, err := json.Marshal(map[string]string{
		"name": name,
		"type": libraryType,
		"slug": relPath,
	})
	if err != nil {
		t.Fatalf("marshal create library: %v", err)
	}

	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/admin/libraries",
		bytes.NewReader(body),
	)
	createRequest.Header.Set("Content-Type", "application/json")
	setBearerAuth(createRequest, token)
	router.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create library: %d %s", createRecorder.Code, createRecorder.Body.String())
	}

	var created LibraryResponse
	err = json.NewDecoder(createRecorder.Body).Decode(&created)
	if err != nil {
		t.Fatalf("decode create library: %v", err)
	}

	rootBody, err := json.Marshal(map[string]string{"relPath": relPath})
	if err != nil {
		t.Fatalf("marshal add root: %v", err)
	}

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
	if rootRecorder.Code != http.StatusOK {
		t.Fatalf("add library root: %d %s", rootRecorder.Code, rootRecorder.Body.String())
	}

	var updated LibraryResponse
	err = json.NewDecoder(rootRecorder.Body).Decode(&updated)
	if err != nil {
		t.Fatalf("decode add root: %v", err)
	}

	return updated.Library
}

func adminAccessToken(ctx context.Context, t *testing.T, router *gin.Engine) string {
	t.Helper()

	return loginTestAuth(ctx, t, router).AccessToken
}

func createUserSession(
	t *testing.T,
	router *gin.Engine,
	email, password string,
) (string, string) {
	t.Helper()

	ctx := context.Background()
	body := []byte(`{"email":"` + email + `","password":"` + password + `","role":"user"}`)
	createRecorder := httptest.NewRecorder()
	createRequest := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/admin/users",
		bytes.NewReader(body),
	)
	createRequest.Header.Set("Content-Type", "application/json")
	setBearerAuth(createRequest, adminAccessToken(ctx, t, router))
	router.ServeHTTP(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", createRecorder.Code, createRecorder.Body.String())
	}

	var createResponse struct {
		User auth.AdminUser `json:"user"`
	}
	err := json.NewDecoder(createRecorder.Body).Decode(&createResponse)
	if err != nil {
		t.Fatalf("decode create user: %v", err)
	}

	loginBody := []byte(`{"email":"` + email + `","password":"` + password + `"}`)
	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/auth/login",
		bytes.NewReader(loginBody),
	)
	loginRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login user: %d %s", loginRecorder.Code, loginRecorder.Body.String())
	}

	var loginResponse struct {
		AccessToken string `json:"accessToken"`
	}
	err = json.NewDecoder(loginRecorder.Body).Decode(&loginResponse)
	if err != nil {
		t.Fatalf("decode login user: %v", err)
	}
	if loginResponse.AccessToken == "" {
		t.Fatal("expected user access token")
	}

	return createResponse.User.ID, loginResponse.AccessToken
}

func libraryIDByRelPath(t *testing.T, libraries []access.Library, relPath string) string {
	t.Helper()

	for _, library := range libraries {
		if library.RelPath == relPath {
			return library.ID
		}
		if slices.Contains(library.Roots, relPath) {
			return library.ID
		}
	}

	t.Fatalf("library %q not found", relPath)

	return ""
}

func listLibraries(t *testing.T, router *gin.Engine) []access.Library {
	t.Helper()

	ctx := context.Background()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"/api/admin/libraries",
		nil,
	)
	setBearerAuth(request, adminAccessToken(ctx, t, router))
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list libraries: %d %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Libraries []access.Library `json:"libraries"`
	}
	err := json.NewDecoder(recorder.Body).Decode(&response)
	if err != nil {
		t.Fatalf("decode libraries: %v", err)
	}

	return response.Libraries
}

func disableUserViaAPI(t *testing.T, router *gin.Engine, userID string) error {
	t.Helper()

	ctx := context.Background()
	body := []byte(`{"enabled":false}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		"/api/admin/users/"+userID,
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	setBearerAuth(request, adminAccessToken(ctx, t, router))
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return errDisableUserFailed
	}

	return nil
}

var errDisableUserFailed = errors.New("disable user failed")
