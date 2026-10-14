package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestPlaybackPreferencesHandlers_Unauthorized(t *testing.T) {
	t.Parallel()

	allure.Test(t, "playback prefs return 401 without login", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		router := newAuthTestRouter(t)

		getRecorder := httptest.NewRecorder()
		getRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/me/playback-preferences",
			nil,
		)
		router.ServeHTTP(getRecorder, getRequest)
		if getRecorder.Code != http.StatusUnauthorized {
			t.Fatalf("GET: got %d", getRecorder.Code)
		}

		patchRecorder := httptest.NewRecorder()
		patchRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/api/me/playback-preferences",
			bytes.NewReader([]byte(`{"audioLanguages":["en"]}`)),
		)
		patchRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(patchRecorder, patchRequest)
		if patchRecorder.Code != http.StatusUnauthorized {
			t.Fatalf("PATCH: got %d", patchRecorder.Code)
		}
	})
}

func TestPlaybackPreferencesHandlers_GetPatchRoundTrip(t *testing.T) {
	allure.Test(t, "GET and PATCH /api/me/playback-preferences persist languages", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		token := adminAccessToken(ctx, t, router)

		getRecorder := httptest.NewRecorder()
		getRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/me/playback-preferences",
			nil,
		)
		setBearerAuth(getRequest, token)
		router.ServeHTTP(getRecorder, getRequest)
		if getRecorder.Code != http.StatusOK {
			t.Fatalf("GET empty: %d %s", getRecorder.Code, getRecorder.Body.String())
		}

		var empty PlaybackPreferencesResponse
		err := json.NewDecoder(getRecorder.Body).Decode(&empty)
		if err != nil {
			t.Fatalf("decode GET: %v", err)
		}
		if empty.AudioLanguages == nil || len(empty.AudioLanguages) != 0 {
			t.Fatalf("expected empty slice, got %v", empty.AudioLanguages)
		}

		patchBody := []byte(`{"audioLanguages":["jpn","en"]}`)
		patchRecorder := httptest.NewRecorder()
		patchRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPatch,
			"/api/me/playback-preferences",
			bytes.NewReader(patchBody),
		)
		patchRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(patchRequest, token)
		router.ServeHTTP(patchRecorder, patchRequest)
		if patchRecorder.Code != http.StatusOK {
			t.Fatalf("PATCH: %d %s", patchRecorder.Code, patchRecorder.Body.String())
		}

		var saved PlaybackPreferencesResponse
		err = json.NewDecoder(patchRecorder.Body).Decode(&saved)
		if err != nil {
			t.Fatalf("decode PATCH: %v", err)
		}
		if len(saved.AudioLanguages) != 2 || saved.AudioLanguages[0] != "jpn" {
			t.Fatalf("saved: %+v", saved.AudioLanguages)
		}

		getRecorder = httptest.NewRecorder()
		getRequest = httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/me/playback-preferences",
			nil,
		)
		setBearerAuth(getRequest, token)
		router.ServeHTTP(getRecorder, getRequest)
		if getRecorder.Code != http.StatusOK {
			t.Fatalf("GET after patch: %d", getRecorder.Code)
		}

		err = json.NewDecoder(getRecorder.Body).Decode(&empty)
		if err != nil {
			t.Fatalf("decode GET after patch: %v", err)
		}
		if len(empty.AudioLanguages) != 2 || empty.AudioLanguages[1] != "en" {
			t.Fatalf("reloaded: %+v", empty.AudioLanguages)
		}
	})
}

func TestPlaybackPreferencesHandlers_Validation(t *testing.T) {
	allure.Test(t, "PATCH rejects more than three languages and bad JSON", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		router := newAuthTestRouter(t)
		token := adminAccessToken(ctx, t, router)

		tooMany := httptest.NewRecorder()
		tooManyRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPatch,
			"/api/me/playback-preferences",
			bytes.NewReader([]byte(`{"audioLanguages":["a","b","c","d"]}`)),
		)
		tooManyRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(tooManyRequest, token)
		router.ServeHTTP(tooMany, tooManyRequest)
		if tooMany.Code != http.StatusBadRequest {
			t.Fatalf("too many langs: got %d", tooMany.Code)
		}

		badJSON := httptest.NewRecorder()
		badRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPatch,
			"/api/me/playback-preferences",
			bytes.NewReader([]byte(`not-json`)),
		)
		badRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(badRequest, token)
		router.ServeHTTP(badJSON, badRequest)
		if badJSON.Code != http.StatusBadRequest {
			t.Fatalf("bad json: got %d", badJSON.Code)
		}
	})
}

func TestPlaybackPreferencesHandlers_LoadFailed(t *testing.T) {
	allure.Test(t, "GET returns 500 when playback prefs cannot be loaded", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()
		pool := setupTestPool(ctx, t)
		authService := prepareIntegrationAuth(ctx, t, pool)
		h := &handler{auth: authService}

		recorder := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(recorder)
		ginCtx.Request = httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/me/playback-preferences",
			nil,
		)
		setTestUser(ginCtx, auth.PublicUser{
			ID:   "00000000-0000-0000-0000-000000000099",
			Role: auth.RoleUser,
		})
		h.getPlaybackPreferences(ginCtx)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("GET: got %d %s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPlaybackPreferencesHandlers_AuthUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "playback prefs return 500 when auth service unset", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		user := auth.PublicUser{ID: "user-id", Role: auth.RoleUser}
		h := &handler{}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/me/playback-preferences",
			nil,
		)
		setTestUser(ctx, user)
		h.getPlaybackPreferences(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("GET nil auth: got %d", recorder.Code)
		}

		recorder = httptest.NewRecorder()
		ctx, _ = gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPatch,
			"/api/me/playback-preferences",
			bytes.NewReader([]byte(`{"audioLanguages":["en"]}`)),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		setTestUser(ctx, user)
		h.patchPlaybackPreferences(ctx)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("PATCH nil auth: got %d", recorder.Code)
		}
	})
}
