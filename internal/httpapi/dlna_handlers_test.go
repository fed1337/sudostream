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
	"sudoStream/internal/auth"
	"sudoStream/internal/dlna"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

var errDLNASettingsBoom = errors.New("dlna settings boom")

type memoryDLNAKV struct {
	data map[string][]byte
}

func (m *memoryDLNAKV) GetSettingValue(_ context.Context, key string) ([]byte, error) {
	raw, ok := m.data[key]
	if !ok {
		return nil, os.ErrNotExist
	}

	return raw, nil
}

func (m *memoryDLNAKV) SaveSettingValue(_ context.Context, key string, value []byte) error {
	if m.data == nil {
		m.data = map[string][]byte{}
	}
	m.data[key] = append([]byte(nil), value...)

	return nil
}

type stubDLNASettings struct {
	settings dlna.Settings
	getErr   error
	saveErr  error
}

func (s *stubDLNASettings) GetSettings(_ context.Context) (dlna.Settings, error) {
	if s.getErr != nil {
		return dlna.Settings{}, s.getErr
	}

	return s.settings, nil
}

func (s *stubDLNASettings) SaveSettings(_ context.Context, settings dlna.Settings) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.settings = settings

	return nil
}

func TestAdminDLNASettings_NilStore(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	allure.Test(t, "DLNA get returns defaults when store unset; patch is unavailable", func(a *allure.Context) {
		t := a.T()
		admin := &adminHandler{}

		getRecorder := httptest.NewRecorder()
		getCtx, _ := gin.CreateTestContext(getRecorder)
		getCtx.Request = httptest.NewRequestWithContext(
			t.Context(), http.MethodGet, "/api/admin/dlna/settings", nil,
		)
		admin.getDLNASettings(getCtx)
		if getRecorder.Code != http.StatusOK {
			t.Fatalf("get status: %d", getRecorder.Code)
		}
		var got dlna.Settings
		_ = json.Unmarshal(getRecorder.Body.Bytes(), &got)
		if got.Enabled || got.UserID != "" {
			t.Fatalf("expected defaults, got %+v", got)
		}

		patchRecorder := httptest.NewRecorder()
		patchCtx, _ := gin.CreateTestContext(patchRecorder)
		patchCtx.Request = httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPatch,
			"/api/admin/dlna/settings",
			bytes.NewReader([]byte(`{"enabled":false}`)),
		)
		patchCtx.Request.Header.Set("Content-Type", "application/json")
		admin.patchDLNASettings(patchCtx)
		if patchRecorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("patch status: %d body=%s", patchRecorder.Code, patchRecorder.Body.String())
		}
	})
}

func TestAdminDLNASettings_ValidationAndPersistence(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	allure.Test(t, "DLNA admin settings validate TV user and persist", func(a *allure.Context) {
		t := a.T()
		requireTestDatabase(t)

		ctx := context.Background()
		database := setupTestPool(ctx, t)
		authService := prepareIntegrationAuth(ctx, t, database)

		tvUser, err := authService.CreateUser(ctx, auth.CreateUserInput{
			Email:    "livingroom-tv@lan",
			Password: testDefaultPassword,
			Role:     auth.RoleTV,
		})
		if err != nil {
			t.Fatalf("create tv: %v", err)
		}
		normalUser, err := authService.CreateUser(ctx, auth.CreateUserInput{
			Email:    "human@lan",
			Password: testDefaultPassword,
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}

		store := dlna.NewKVSettingsStore(&memoryDLNAKV{})
		admin := &adminHandler{auth: authService, dlnaSettings: store}

		assertDLNAGetOK(t, admin)
		assertDLNAPatchBadBody(t, admin)
		assertDLNAPatchRequiresTV(t, admin, normalUser.ID)
		assertDLNAPatchUnknownUser(t, admin)
		assertDLNAPatchEnableOK(t, admin, store, tvUser.ID)
		assertDLNAPatchDisableOK(t, admin, store, tvUser.ID)
	})
}

func TestAdminDLNASettings_StoreErrors(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	allure.Test(t, "DLNA admin settings map store errors to HTTP status", func(a *allure.Context) {
		t := a.T()

		getFail := &adminHandler{dlnaSettings: &stubDLNASettings{getErr: errDLNASettingsBoom}}
		getRecorder := httptest.NewRecorder()
		getCtx, _ := gin.CreateTestContext(getRecorder)
		getCtx.Request = httptest.NewRequestWithContext(
			t.Context(), http.MethodGet, "/api/admin/dlna/settings", nil,
		)
		getFail.getDLNASettings(getCtx)
		if getRecorder.Code != http.StatusInternalServerError {
			t.Fatalf("get err status: %d", getRecorder.Code)
		}

		patchLoadFail := &adminHandler{dlnaSettings: &stubDLNASettings{getErr: errDLNASettingsBoom}}
		assertDLNAPatchStatus(
			t, patchLoadFail, `{"enabled":false}`, http.StatusInternalServerError,
		)

		tvRequired := &adminHandler{dlnaSettings: &stubDLNASettings{}}
		assertDLNAPatchStatus(
			t, tvRequired, `{"enabled":true}`, http.StatusBadRequest,
		)

		saveTVRequired := &adminHandler{
			dlnaSettings: &stubDLNASettings{saveErr: dlna.ErrTVUserRequired},
		}
		assertDLNAPatchStatus(
			t, saveTVRequired, `{"enabled":false,"userId":"x"}`, http.StatusBadRequest,
		)

		saveBoom := &adminHandler{
			dlnaSettings: &stubDLNASettings{saveErr: errDLNASettingsBoom},
		}
		assertDLNAPatchStatus(
			t, saveBoom, `{"enabled":false,"userId":"x"}`, http.StatusInternalServerError,
		)
	})
}

//nolint:paralleltest // setupTestCacheEnv mutates package testCacheRoot
func TestAdminDLNASettings_HTTPRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	allure.Test(t, "admin DLNA settings routes require auth and round-trip", func(a *allure.Context) {
		t := a.T()
		requireTestDatabase(t)
		setupTestCacheEnv(t)

		ctx := context.Background()
		database := setupTestPool(ctx, t)
		authService := prepareIntegrationAuth(ctx, t, database)
		router := newDLNASettingsTestRouter(t, authService)
		assertDLNARouteRequiresAuth(t, router)

		adminToken := adminAccessToken(ctx, t, router)
		tvUser, err := authService.CreateUser(ctx, auth.CreateUserInput{
			Email:    "dlna-route-tv@lan",
			Password: testDefaultPassword,
			Role:     auth.RoleTV,
		})
		if err != nil {
			t.Fatalf("create tv: %v", err)
		}

		assertDLNARouteGetOK(t, router, adminToken)
		assertDLNARouteEnableOK(t, router, adminToken, tvUser.ID)
	})
}

func newDLNASettingsTestRouter(t *testing.T, authService *auth.Service) *gin.Engine {
	t.Helper()

	root := t.TempDir()
	err := os.WriteFile(filepath.Join(root, ".keep"), []byte("ok"), 0o600)
	if err != nil {
		t.Fatalf("write placeholder: %v", err)
	}
	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("media: %v", err)
	}

	cfg := integrationRouteConfig(root)
	cfg.DLNASettings = dlna.NewKVSettingsStore(&memoryDLNAKV{})

	router := gin.New()
	RegisterRoutes(router, media, authService, nil, nil, nil, cfg)

	return router
}

func assertDLNARouteRequiresAuth(t *testing.T, router *gin.Engine) {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/admin/dlna/settings", nil,
	)
	router.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusOK {
		t.Fatalf("expected auth required, got %d", recorder.Code)
	}
}

func assertDLNARouteGetOK(t *testing.T, router *gin.Engine, adminToken string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/admin/dlna/settings", nil,
	)
	setBearerAuth(req, adminToken)
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get: %d %s", recorder.Code, recorder.Body.String())
	}
}

func assertDLNARouteEnableOK(
	t *testing.T,
	router *gin.Engine,
	adminToken, tvID string,
) {
	t.Helper()
	body, err := json.Marshal(dlna.Settings{Enabled: true, UserID: tvID})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		t.Context(), http.MethodPatch, "/api/admin/dlna/settings", bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	setBearerAuth(req, adminToken)
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", recorder.Code, recorder.Body.String())
	}
	var saved dlna.Settings
	_ = json.Unmarshal(recorder.Body.Bytes(), &saved)
	if !saved.Enabled || saved.UserID != tvID || saved.UDN == "" {
		t.Fatalf("saved: %+v", saved)
	}
}

func assertDLNAGetOK(t *testing.T, admin *adminHandler) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/api/admin/dlna/settings", nil,
	)
	admin.getDLNASettings(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get: %d", recorder.Code)
	}
}

func assertDLNAPatchBadBody(t *testing.T, admin *adminHandler) {
	t.Helper()
	assertDLNAPatchStatus(t, admin, `{`, http.StatusBadRequest)
}

func assertDLNAPatchRequiresTV(t *testing.T, admin *adminHandler, userID string) {
	t.Helper()
	body, err := json.Marshal(dlna.Settings{Enabled: true, UserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	assertDLNAPatchStatus(t, admin, string(body), http.StatusBadRequest)
}

func assertDLNAPatchUnknownUser(t *testing.T, admin *adminHandler) {
	t.Helper()
	body, err := json.Marshal(dlna.Settings{
		Enabled: true,
		UserID:  "00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertDLNAPatchStatus(t, admin, string(body), http.StatusBadRequest)
}

func assertDLNAPatchEnableOK(
	t *testing.T,
	admin *adminHandler,
	store *dlna.KVSettingsStore,
	tvID string,
) {
	t.Helper()
	body, err := json.Marshal(dlna.Settings{Enabled: true, UserID: tvID})
	if err != nil {
		t.Fatal(err)
	}
	recorder := assertDLNAPatchStatus(t, admin, string(body), http.StatusOK)
	var got dlna.Settings
	_ = json.Unmarshal(recorder.Body.Bytes(), &got)
	if !got.Enabled || got.UserID != tvID || got.UDN == "" {
		t.Fatalf("enable body: %+v", got)
	}
	stored, err := store.GetSettings(context.Background())
	if err != nil || !stored.Enabled || stored.UserID != tvID {
		t.Fatalf("stored enable: %+v err=%v", stored, err)
	}
}

func assertDLNAPatchDisableOK(
	t *testing.T,
	admin *adminHandler,
	store *dlna.KVSettingsStore,
	tvID string,
) {
	t.Helper()
	body, err := json.Marshal(dlna.Settings{Enabled: false, UserID: tvID})
	if err != nil {
		t.Fatal(err)
	}
	assertDLNAPatchStatus(t, admin, string(body), http.StatusOK)
	stored, err := store.GetSettings(context.Background())
	if err != nil || stored.Enabled {
		t.Fatalf("stored disable: %+v err=%v", stored, err)
	}
}

func assertDLNAPatchStatus(
	t *testing.T,
	admin *adminHandler,
	body string,
	wantStatus int,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPatch,
		"/api/admin/dlna/settings",
		bytes.NewReader([]byte(body)),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	admin.patchDLNASettings(ctx)
	if recorder.Code != wantStatus {
		t.Fatalf("patch status: want %d got %d body=%s", wantStatus, recorder.Code, recorder.Body.String())
	}

	return recorder
}
