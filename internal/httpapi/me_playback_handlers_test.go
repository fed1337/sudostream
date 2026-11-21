package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

		for _, tc := range []struct {
			name   string
			method string
			body   string
		}{
			{name: "GET", method: http.MethodGet},
			{name: "PATCH", method: http.MethodPatch, body: `{"audioLanguages":["en"]}`},
		} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				context.Background(),
				tc.method,
				"/api/me/playback-preferences",
				strings.NewReader(tc.body),
			)
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("%s: got %d", tc.name, recorder.Code)
			}
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
		if empty.AutoSkipIntro || empty.AutoNextEpisode {
			t.Fatalf("expected auto toggles false, got %+v", empty)
		}

		patchBody := []byte(`{"audioLanguages":["jpn","en"],"autoSkipIntro":true,"autoNextEpisode":true}`)
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
		if !saved.AutoSkipIntro || !saved.AutoNextEpisode {
			t.Fatalf("saved toggles: %+v", saved)
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
		if !empty.AutoSkipIntro || !empty.AutoNextEpisode {
			t.Fatalf("reloaded toggles: %+v", empty)
		}

		partial := httptest.NewRecorder()
		partialRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPatch,
			"/api/me/playback-preferences",
			bytes.NewReader([]byte(`{"autoSkipIntro":false}`)),
		)
		partialRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(partialRequest, token)
		router.ServeHTTP(partial, partialRequest)
		if partial.Code != http.StatusOK {
			t.Fatalf("partial PATCH: %d %s", partial.Code, partial.Body.String())
		}

		var afterPartial PlaybackPreferencesResponse
		err = json.NewDecoder(partial.Body).Decode(&afterPartial)
		if err != nil {
			t.Fatalf("decode partial: %v", err)
		}
		if afterPartial.AutoSkipIntro || !afterPartial.AutoNextEpisode {
			t.Fatalf("partial merge: %+v", afterPartial)
		}
		if len(afterPartial.AudioLanguages) != 2 || afterPartial.AudioLanguages[0] != "jpn" {
			t.Fatalf("partial must keep languages: %+v", afterPartial.AudioLanguages)
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

		for _, tc := range []struct {
			name string
			body string
		}{
			{name: "too many langs", body: `{"audioLanguages":["a","b","c","d"]}`},
			{name: "bad json", body: `not-json`},
		} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				ctx,
				http.MethodPatch,
				"/api/me/playback-preferences",
				strings.NewReader(tc.body),
			)
			request.Header.Set("Content-Type", "application/json")
			setBearerAuth(request, token)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("%s: got %d", tc.name, recorder.Code)
			}
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

	for _, tc := range []struct {
		name   string
		method string
		body   string
		call   func(*handler, *gin.Context)
	}{
		{
			name:   "GET",
			method: http.MethodGet,
			call:   (*handler).getPlaybackPreferences,
		},
		{
			name:   "PATCH",
			method: http.MethodPatch,
			body:   `{"audioLanguages":["en"]}`,
			call:   (*handler).patchPlaybackPreferences,
		},
	} {
		allure.Test(t, "playback prefs return 500 when auth service unset — "+tc.name, func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				tc.method,
				"/api/me/playback-preferences",
				strings.NewReader(tc.body),
			)
			ctx.Request.Header.Set("Content-Type", "application/json")
			setTestUser(ctx, auth.PublicUser{ID: "user-id", Role: auth.RoleUser})
			tc.call(&handler{}, ctx)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("got %d", recorder.Code)
			}
		})
	}
}
