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
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/watch"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const (
	watchTestLibraryID = "lib-movies"
	watchTestUserID    = "user-1"
	watchTestUserEmail = "a@example.com"
)

var errTestWatchHandler = errors.New("watch handler error")

type watchHandlerMemoryStore struct {
	rows map[string]watch.State
}

func (m *watchHandlerMemoryStore) Get(
	_ context.Context,
	userID, libraryID, relPath string,
) (watch.State, error) {
	return m.rows[userID+"|"+libraryID+"|"+relPath], nil
}

func (m *watchHandlerMemoryStore) Upsert(
	_ context.Context,
	userID, libraryID, relPath string,
	state watch.State,
) error {
	m.rows[userID+"|"+libraryID+"|"+relPath] = state

	return nil
}

func (m *watchHandlerMemoryStore) Clear(
	_ context.Context,
	userID, libraryID, relPath string,
) error {
	delete(m.rows, userID+"|"+libraryID+"|"+relPath)

	return nil
}

func (m *watchHandlerMemoryStore) ListForPaths(
	_ context.Context,
	userID, libraryID string,
	relPaths []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(relPaths))
	for _, relPath := range relPaths {
		if m.rows[userID+"|"+libraryID+"|"+relPath].Watched {
			result[relPath] = true
		}
	}

	return result, nil
}

func (m *watchHandlerMemoryStore) DeleteForPath(
	_ context.Context,
	libraryID, relPath string,
) error {
	suffix := "|" + libraryID + "|" + relPath
	for key := range m.rows {
		if len(key) >= len(suffix) && key[len(key)-len(suffix):] == suffix {
			delete(m.rows, key)
		}
	}

	return nil
}

type flagMemoryStore struct {
	rows map[string]bool
}

func (m *flagMemoryStore) Get(
	_ context.Context,
	userID, libraryID, relPath string,
) (bool, error) {
	return m.rows[userID+"|"+libraryID+"|"+relPath], nil
}

func (m *flagMemoryStore) Set(
	_ context.Context,
	userID, libraryID, relPath string,
	on bool,
) error {
	key := userID + "|" + libraryID + "|" + relPath
	if on {
		m.rows[key] = true
	} else {
		delete(m.rows, key)
	}

	return nil
}

func (m *flagMemoryStore) ListForPaths(
	_ context.Context,
	userID, libraryID string,
	relPaths []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(relPaths))
	for _, relPath := range relPaths {
		if m.rows[userID+"|"+libraryID+"|"+relPath] {
			result[relPath] = true
		}
	}

	return result, nil
}

func (m *flagMemoryStore) DeleteForPath(
	_ context.Context,
	libraryID, relPath string,
) error {
	suffix := "|" + libraryID + "|" + relPath
	for key := range m.rows {
		if len(key) >= len(suffix) && key[len(key)-len(suffix):] == suffix {
			delete(m.rows, key)
		}
	}

	return nil
}

type watchHandlerAccessStore struct{}

func (watchHandlerAccessStore) ListLibraries(_ context.Context) ([]access.Library, error) {
	return []access.Library{{
		ID:      watchTestLibraryID,
		RelPath: playTestLibraryPath,
		Slug:    playTestLibraryPath,
		Name:    playTestLibraryName,
		Type:    access.LibraryTypeFilm,
	}}, nil
}

func (watchHandlerAccessStore) GetLibrary(
	_ context.Context,
	libraryID string,
) (access.Library, error) {
	if libraryID == watchTestLibraryID {
		return access.Library{
			ID:      watchTestLibraryID,
			RelPath: playTestLibraryPath,
			Slug:    playTestLibraryPath,
			Name:    playTestLibraryName,
			Type:    access.LibraryTypeFilm,
		}, nil
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (watchHandlerAccessStore) UpsertLibrary(
	_ context.Context,
	library access.Library,
) (access.Library, error) {
	return library, nil
}

func (s watchHandlerAccessStore) CreateLibrary(
	ctx context.Context,
	library access.Library,
) (access.Library, error) {
	return s.UpsertLibrary(ctx, library)
}

func (s watchHandlerAccessStore) AddRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (s watchHandlerAccessStore) RemoveRoot(
	ctx context.Context,
	libraryID, _ string,
) (access.Library, error) {
	return s.GetLibrary(ctx, libraryID)
}

func (watchHandlerAccessStore) DeleteLibrary(_ context.Context, _ string) error {
	return nil
}

func (watchHandlerAccessStore) UpdateLibrary(
	ctx context.Context,
	libraryID string,
	name *string,
	libraryType *access.LibraryType,
) (access.Library, error) {
	library, err := (watchHandlerAccessStore{}).GetLibrary(ctx, libraryID)
	if err != nil {
		return access.Library{}, err
	}
	if name != nil {
		library.Name = *name
	}
	if libraryType != nil {
		library.Type = *libraryType
	}

	return library, nil
}

func (watchHandlerAccessStore) GetUserGrantMap(
	_ context.Context,
	_ string,
) (map[string]access.LibraryPermissions, error) {
	return map[string]access.LibraryPermissions{}, nil
}

func (watchHandlerAccessStore) ReplaceUserGrants(
	_ context.Context,
	_ string,
	_ []access.GrantInput,
) error {
	return nil
}

func newWatchHandlerFixture(t *testing.T) (*handler, string) {
	t.Helper()

	root := t.TempDir()
	rel := playTestSampleRel
	abs := filepath.Join(root, rel)
	err := os.MkdirAll(filepath.Dir(abs), 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err = os.WriteFile(abs, []byte("fake-video"), playTestFileMode)
	if err != nil {
		t.Fatalf("write video: %v", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("media service: %v", err)
	}

	accessSvc := access.NewService(watchHandlerAccessStore{})
	store := &watchHandlerMemoryStore{rows: map[string]watch.State{}}
	watchSvc := watch.NewService(media, accessSvc, store)

	return &handler{media: media, watch: watchSvc}, rel
}

func TestGetWatch_ReturnsWatchedState(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET /api/watch returns watched flag for user", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		watchHandler, rel := newWatchHandlerFixture(t)
		user := auth.PublicUser{
			ID:    watchTestUserID,
			Email: watchTestUserEmail,
			Role:  auth.RoleAdmin,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/watch/"+rel,
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel}}
		setTestUser(ctx, user)

		watchHandler.getWatch(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var body WatchResponse
		err := json.Unmarshal(recorder.Body.Bytes(), &body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Watched {
			t.Fatal("expected unwatched initially")
		}
	})
}

func TestPatchWatch_UpdatesWatchedState(t *testing.T) {
	t.Parallel()

	allure.Test(t, "PATCH /api/watch toggles watched for user", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		watchHandler, rel := newWatchHandlerFixture(t)
		user := auth.PublicUser{
			ID:    watchTestUserID,
			Email: watchTestUserEmail,
			Role:  auth.RoleAdmin,
		}

		payload, err := json.Marshal(WatchPatchRequest{Watched: &watchedTrue})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/api/watch/"+rel,
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel}}
		setTestUser(ctx, user)

		watchHandler.patchWatch(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("patch status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var patched WatchResponse
		err = json.Unmarshal(recorder.Body.Bytes(), &patched)
		if err != nil {
			t.Fatalf("decode patch: %v", err)
		}
		if !patched.Watched {
			t.Fatal("expected watched true after patch")
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/watch/"+rel,
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel}}
		setTestUser(ctx, user)

		watchHandler.getWatch(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("get status: got %d", recorder.Code)
		}

		err = json.Unmarshal(recorder.Body.Bytes(), &patched)
		if err != nil {
			t.Fatalf("decode get: %v", err)
		}
		if !patched.Watched {
			t.Fatal("expected watched true on get")
		}
	})
}

func TestWatchHandlers_RequireAuth(t *testing.T) {
	t.Parallel()

	allure.Test(t, "watch endpoints return 401 without user", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		watchHandler, rel := newWatchHandlerFixture(t)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/watch/"+rel,
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel}}

		watchHandler.getWatch(ctx)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("get status: got %d", recorder.Code)
		}
	})
}

func TestWatchHandlers_UnavailableWhenNilService(t *testing.T) {
	t.Parallel()

	allure.Test(t, "watch endpoints return 503 when service is nil", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		watchHandler := &handler{}
		user := auth.PublicUser{
			ID:    watchTestUserID,
			Email: watchTestUserEmail,
			Role:  auth.RoleAdmin,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/watch/"+playTestSampleRel,
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: playTestSampleRel}}
		setTestUser(ctx, user)

		watchHandler.getWatch(ctx)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func TestHandleWatchError_MapsDomainErrors(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"handleWatchError maps invalid and unknown paths to 400",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			watchHandler := &handler{}
			cases := []error{
				watch.ErrUnknownLibrary,
				mediafs.ErrInvalidPath,
				mediafs.ErrPathOutsideRoot,
			}

			for _, domainErr := range cases {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequestWithContext(
					context.Background(),
					http.MethodGet,
					"/api/watch/movies/demo.mp4",
					nil,
				)

				watchHandler.handleWatchError(ctx, "movies/demo.mp4", domainErr)

				if recorder.Code != http.StatusBadRequest {
					t.Fatalf("%v: status got %d", domainErr, recorder.Code)
				}
			}
		},
	)

	allure.Test(t, "handleWatchError maps unexpected errors to 500", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		watchHandler := &handler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/api/watch/movies/demo.mp4",
			nil,
		)

		watchHandler.handleWatchError(ctx, "movies/demo.mp4", errTestWatchHandler)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPatchWatch_RejectsInvalidBody(t *testing.T) {
	t.Parallel()

	allure.Test(t, "PATCH /api/watch returns 400 for invalid JSON", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		watchHandler, rel := newWatchHandlerFixture(t)
		user := auth.PublicUser{
			ID:    watchTestUserID,
			Email: watchTestUserEmail,
			Role:  auth.RoleAdmin,
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/api/watch/"+rel,
			bytes.NewReader([]byte("{")),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel}}
		setTestUser(ctx, user)

		watchHandler.patchWatch(ctx)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPatchWatch_UnavailableWhenNilService(t *testing.T) {
	t.Parallel()

	allure.Test(t, "PATCH /api/watch returns 503 when service is nil", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		watchHandler := &handler{}
		user := auth.PublicUser{
			ID:    watchTestUserID,
			Email: watchTestUserEmail,
			Role:  auth.RoleAdmin,
		}

		payload, err := json.Marshal(WatchPatchRequest{Watched: &watchedTrue})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/api/watch/"+playTestSampleRel,
			bytes.NewReader(payload),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: playTestSampleRel}}
		setTestUser(ctx, user)

		watchHandler.patchWatch(ctx)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func TestPatchWatch_ProgressWatchedAndUnwatched(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"PATCH /api/watch saves progress, completes, and deletes on unwatched",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			hdl, rel := newWatchHandlerFixture(t)
			user := auth.PublicUser{
				ID:    watchTestUserID,
				Email: watchTestUserEmail,
				Role:  auth.RoleAdmin,
			}

			recorder := patchWatch(t, hdl, rel, user, WatchPatchRequest{})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("empty body: got %d %s", recorder.Code, recorder.Body.String())
			}

			short := 4.0
			duration := 120.0
			got := decodeWatch(t, patchWatch(t, hdl, rel, user, WatchPatchRequest{
				PositionSeconds: &short,
				DurationSeconds: &duration,
			}))
			if got.Watched || got.PositionSeconds != nil {
				t.Fatalf("short start should be ignored: %#v", got)
			}

			position := 25.0
			got = decodeWatch(t, patchWatch(t, hdl, rel, user, WatchPatchRequest{
				PositionSeconds: &position,
				DurationSeconds: &duration,
			}))
			if got.Watched || got.PositionSeconds == nil || *got.PositionSeconds != position {
				t.Fatalf("progress: %#v", got)
			}

			complete := 110.0
			got = decodeWatch(t, patchWatch(t, hdl, rel, user, WatchPatchRequest{
				PositionSeconds: &complete,
				DurationSeconds: &duration,
			}))
			if !got.Watched {
				t.Fatalf("complete should mark watched: %#v", got)
			}

			got = decodeWatch(t, patchWatch(t, hdl, rel, user, WatchPatchRequest{
				Watched: &watchedFalse,
			}))
			if got.Watched || got.PositionSeconds != nil {
				t.Fatalf("unwatched should delete row: %#v", got)
			}
		},
	)
}

func patchWatch(
	t *testing.T,
	hdl *handler,
	rel string,
	user auth.PublicUser,
	body WatchPatchRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPatch,
		"/api/watch/"+rel,
		bytes.NewReader(payload),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel}}
	setTestUser(ctx, user)
	hdl.patchWatch(ctx)

	return recorder
}

func decodeWatch(t *testing.T, recorder *httptest.ResponseRecorder) WatchResponse {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d %s", recorder.Code, recorder.Body.String())
	}
	var got WatchResponse
	err := json.Unmarshal(recorder.Body.Bytes(), &got)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	return got
}

var (
	watchedTrue  = true
	watchedFalse = false
)
