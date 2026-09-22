package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAuth_BrowseRequiresSession(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "browse returns 401 without session cookie", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/browse",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: got %d want %d", recorder.Code, http.StatusUnauthorized)
		}
	})
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAuth_LoginRejectsInvalidPassword(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "login rejects invalid password", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)

		loginBody := testAdminLoginJSON("wrong-password")
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/auth/login",
			bytes.NewReader(loginBody),
		)
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: got %d want %d", recorder.Code, http.StatusUnauthorized)
		}
	})
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAuth_LogoutClearsSession(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "logout invalidates tokens", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)
		credentials := loginTestAuth(context.Background(), t, router)

		logoutRecorder := httptest.NewRecorder()
		logoutRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/auth/logout",
			nil,
		)
		logoutRequest.AddCookie(credentials.RefreshCookie)
		router.ServeHTTP(logoutRecorder, logoutRequest)

		if logoutRecorder.Code != http.StatusNoContent {
			t.Fatalf("logout status: got %d want %d", logoutRecorder.Code, http.StatusNoContent)
		}

		browseRecorder := httptest.NewRecorder()
		browseRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/browse",
			nil,
		)
		setBearerAuth(browseRequest, credentials.AccessToken)
		router.ServeHTTP(browseRecorder, browseRequest)

		if browseRecorder.Code != http.StatusUnauthorized {
			t.Fatalf(
				"browse after logout: got %d want %d",
				browseRecorder.Code,
				http.StatusUnauthorized,
			)
		}
	})
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestAuth_LoginSetsCookieAndAllowsBrowse(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "login returns access token and unlocks browse", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)
		credentials := loginTestAuth(context.Background(), t, router)
		assertBrowseAuthorized(t, router, credentials.AccessToken)
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_PasswordResetFlow(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "forgot and reset password updates credentials", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		mail := &captureSender{}
		router := newAuthTestRouterWithMail(t, mail)

		forgotBody := testEmailJSON(testAdminEmail)
		forgotRecorder := httptest.NewRecorder()
		forgotRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/forgot-password",
			bytes.NewReader(forgotBody),
		)
		forgotRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(forgotRecorder, forgotRequest)
		if forgotRecorder.Code != http.StatusNoContent {
			t.Fatalf("forgot status: got %d", forgotRecorder.Code)
		}

		token := extractTokenFromEmail(mail.lastBody)
		if token == "" {
			t.Fatal("expected reset token in email body")
		}

		resetBody := []byte(`{"token":"` + token + `","password":"new-secret-pass"}`)
		resetRecorder := httptest.NewRecorder()
		resetRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/reset-password",
			bytes.NewReader(resetBody),
		)
		resetRequest.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(resetRecorder, resetRequest)
		if resetRecorder.Code != http.StatusNoContent {
			t.Fatalf(
				"reset status: got %d body=%s",
				resetRecorder.Code,
				resetRecorder.Body.String(),
			)
		}

		loginBody := testAdminLoginJSON("new-secret-pass")
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
			t.Fatalf(
				"login after reset: got %d body=%s",
				loginRecorder.Code,
				loginRecorder.Body.String(),
			)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_ChangePasswordInvalidCurrentPassword(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "change password rejects wrong current password", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		router := newAuthTestRouter(t)
		credentials := loginTestAuth(ctx, t, router)

		// Seeded admin starts with mustChangePassword; clear it so current-password checks apply.
		clearBody := testChangePasswordJSON(testDefaultPassword, "cleared-mcp-pass-123")
		clearRecorder := httptest.NewRecorder()
		clearRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/change-password",
			bytes.NewReader(clearBody),
		)
		clearRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(clearRequest, credentials.AccessToken)
		router.ServeHTTP(clearRecorder, clearRequest)
		if clearRecorder.Code != http.StatusOK {
			t.Fatalf(
				"clear mustChangePassword status: got %d body=%s",
				clearRecorder.Code,
				clearRecorder.Body.String(),
			)
		}

		var clearResponse struct {
			AccessToken string `json:"accessToken"`
		}
		err := json.NewDecoder(clearRecorder.Body).Decode(&clearResponse)
		if err != nil {
			t.Fatalf("decode clear response: %v", err)
		}
		if clearResponse.AccessToken == "" {
			t.Fatal("expected access token after clearing mustChangePassword")
		}

		changeBody := testChangePasswordJSON("wrong-current-password", "updated-pass-123")
		changeRecorder := httptest.NewRecorder()
		changeRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/change-password",
			bytes.NewReader(changeBody),
		)
		changeRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(changeRequest, clearResponse.AccessToken)
		router.ServeHTTP(changeRecorder, changeRequest)

		if changeRecorder.Code != http.StatusUnauthorized {
			t.Fatalf(
				"change password status: got %d want %d body=%s",
				changeRecorder.Code,
				http.StatusUnauthorized,
				changeRecorder.Body.String(),
			)
		}

		var errorResponse ErrorResponse
		err = json.NewDecoder(changeRecorder.Body).Decode(&errorResponse)
		if err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errorResponse.Error != errInvalidCurrentPassword {
			t.Fatalf(
				"unexpected error: got %q want %q",
				errorResponse.Error,
				errInvalidCurrentPassword,
			)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_ChangePasswordRevokesOtherSessions(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "change password succeeds for authenticated user", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		router := newAuthTestRouter(t)
		credentials := loginTestAuth(ctx, t, router)

		changeBody := testChangePasswordJSON(testDefaultPassword, "updated-pass-123")
		changeRecorder := httptest.NewRecorder()
		changeRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/change-password",
			bytes.NewReader(changeBody),
		)
		changeRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(changeRequest, credentials.AccessToken)
		router.ServeHTTP(changeRecorder, changeRequest)
		if changeRecorder.Code != http.StatusOK {
			t.Fatalf(
				"change password status: got %d body=%s",
				changeRecorder.Code,
				changeRecorder.Body.String(),
			)
		}

		var changeResponse struct {
			AccessToken string `json:"accessToken"`
			User        struct {
				MustChangePassword bool `json:"mustChangePassword"`
			} `json:"user"`
		}
		err := json.NewDecoder(changeRecorder.Body).Decode(&changeResponse)
		if err != nil {
			t.Fatalf("decode change password response: %v", err)
		}
		if changeResponse.AccessToken == "" {
			t.Fatal("expected access token after password change")
		}
		if changeResponse.User.MustChangePassword {
			t.Fatal("mustChangePassword should be cleared after password change")
		}

		loginBody := testAdminLoginJSON("updated-pass-123")
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
			t.Fatalf("login with new password: got %d", loginRecorder.Code)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_SessionsListAndRevoke(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "user can list and revoke sessions", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		router := newAuthTestRouter(t)
		credentials := loginTestAuth(ctx, t, router)

		listRecorder := httptest.NewRecorder()
		listRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/auth/sessions",
			nil,
		)
		setBearerAuth(listRequest, credentials.AccessToken)
		router.ServeHTTP(listRecorder, listRequest)
		if listRecorder.Code != http.StatusOK {
			t.Fatalf("list sessions: got %d body=%s", listRecorder.Code, listRecorder.Body.String())
		}

		var listResponse struct {
			Sessions []struct {
				ID        string `json:"id"`
				IsCurrent bool   `json:"isCurrent"`
			} `json:"sessions"`
		}
		err := json.NewDecoder(listRecorder.Body).Decode(&listResponse)
		if err != nil {
			t.Fatalf("decode sessions: %v", err)
		}
		if len(listResponse.Sessions) == 0 {
			t.Fatal("expected at least one session")
		}

		sessionID := listResponse.Sessions[0].ID
		revokeRecorder := httptest.NewRecorder()
		revokeRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			"/api/auth/sessions/"+sessionID,
			nil,
		)
		setBearerAuth(revokeRequest, credentials.AccessToken)
		router.ServeHTTP(revokeRecorder, revokeRequest)
		if revokeRecorder.Code != http.StatusNoContent {
			t.Fatalf(
				"revoke session: got %d body=%s",
				revokeRecorder.Code,
				revokeRecorder.Body.String(),
			)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_ChangeEmailAndRevokeAllSessions(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "change email and revoke-all session endpoints", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		mail := &captureSender{}
		router := newAuthTestRouterWithMail(t, mail)
		credentials := loginTestAuth(ctx, t, router)

		revokeAllRecorder := httptest.NewRecorder()
		revokeAllRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/sessions/revoke-all",
			nil,
		)
		setBearerAuth(revokeAllRequest, credentials.AccessToken)
		router.ServeHTTP(revokeAllRecorder, revokeAllRequest)
		if revokeAllRecorder.Code != http.StatusNoContent {
			t.Fatalf(
				"revoke all: got %d body=%s",
				revokeAllRecorder.Code,
				revokeAllRecorder.Body.String(),
			)
		}

		refreshRecorder := httptest.NewRecorder()
		refreshRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/refresh",
			nil,
		)
		refreshRequest.AddCookie(credentials.RefreshCookie)
		router.ServeHTTP(refreshRecorder, refreshRequest)
		if refreshRecorder.Code != http.StatusUnauthorized {
			t.Fatalf(
				"refresh after revoke-all: got %d want 401 body=%s",
				refreshRecorder.Code,
				refreshRecorder.Body.String(),
			)
		}

		credentials = loginTestAuth(ctx, t, router)

		badPasswordRecorder := httptest.NewRecorder()
		badPasswordRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/change-email",
			bytes.NewReader([]byte(
				`{"newEmail":"new-admin@example.com","password":"wrong-password"}`,
			)),
		)
		badPasswordRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(badPasswordRequest, credentials.AccessToken)
		router.ServeHTTP(badPasswordRecorder, badPasswordRequest)
		if badPasswordRecorder.Code != http.StatusUnauthorized {
			t.Fatalf(
				"change email bad password: got %d body=%s",
				badPasswordRecorder.Code,
				badPasswordRecorder.Body.String(),
			)
		}

		changeRecorder := httptest.NewRecorder()
		changeRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/change-email",
			bytes.NewReader([]byte(
				`{"newEmail":"new-admin@example.com","password":"`+testDefaultPassword+`"}`,
			)),
		)
		changeRequest.Header.Set("Content-Type", "application/json")
		setBearerAuth(changeRequest, credentials.AccessToken)
		router.ServeHTTP(changeRecorder, changeRequest)
		if changeRecorder.Code != http.StatusNoContent {
			t.Fatalf(
				"change email: got %d body=%s",
				changeRecorder.Code,
				changeRecorder.Body.String(),
			)
		}
		if mail.lastBody == "" {
			t.Fatal("expected change-email confirmation mail")
		}
	})
}

//nolint:paralleltest,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_HandlerValidationBranches(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"auth handlers reject bad JSON and missing credentials via real router",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router := newAuthTestRouter(t)

			routeChangePW := "/api/auth/" + "change-password"
			routeChangeMail := "/api/auth/change-email"
			routeDisable2FA := "/api/auth/2fa"

			cases := []struct {
				name       string
				method     string
				path       string
				body       string
				auth       bool
				wantStatus int
			}{
				{
					"login bad json",
					http.MethodPost,
					"/api/auth/login",
					"{",
					false,
					http.StatusBadRequest,
				},
				{
					"forgot bad json",
					http.MethodPost,
					"/api/auth/forgot-password",
					"{",
					false,
					http.StatusBadRequest,
				},
				{
					"reset bad json",
					http.MethodPost,
					"/api/auth/reset-password",
					"{",
					false,
					http.StatusBadRequest,
				},
				{
					"accept invite bad json",
					http.MethodPost,
					"/api/auth/accept-invite",
					"{",
					false,
					http.StatusBadRequest,
				},
				{
					"refresh missing cookie",
					http.MethodPost,
					"/api/auth/refresh",
					"",
					false,
					http.StatusUnauthorized,
				},
				{
					"me unauthorized",
					http.MethodGet,
					"/api/auth/me",
					"",
					false,
					http.StatusUnauthorized,
				},
				{
					"sessions unauthorized",
					http.MethodGet,
					"/api/auth/sessions",
					"",
					false,
					http.StatusUnauthorized,
				},
				{
					"revoke session unauthorized",
					http.MethodDelete,
					"/api/auth/sessions/00000000-0000-0000-0000-000000000001",
					"",
					false,
					http.StatusUnauthorized,
				},
				{
					"revoke all unauthorized",
					http.MethodPost,
					"/api/auth/sessions/revoke-all",
					"",
					false,
					http.StatusUnauthorized,
				},
				{
					"change password unauthorized",
					http.MethodPost,
					routeChangePW,
					`{}`,
					false,
					http.StatusUnauthorized,
				},
				{
					"change email unauthorized",
					http.MethodPost,
					routeChangeMail,
					`{}`,
					false,
					http.StatusUnauthorized,
				},
				{
					"disable 2fa unauthorized",
					http.MethodDelete,
					routeDisable2FA,
					`{}`,
					false,
					http.StatusUnauthorized,
				},
				{
					"confirm 2fa bad json",
					http.MethodPost,
					"/api/auth/2fa/confirm",
					"{",
					false,
					http.StatusBadRequest,
				},
				{
					"verify 2fa bad json",
					http.MethodPost,
					"/api/auth/2fa/verify",
					"{",
					false,
					http.StatusBadRequest,
				},
				{
					"setup 2fa unauthorized",
					http.MethodPost,
					"/api/auth/2fa/setup",
					`{}`,
					false,
					http.StatusUnauthorized,
				},
			}

			for _, testCase := range cases {
				recorder := httptest.NewRecorder()
				var bodyReader *bytes.Reader
				if testCase.body != "" {
					bodyReader = bytes.NewReader([]byte(testCase.body))
				} else {
					bodyReader = bytes.NewReader(nil)
				}
				request := httptest.NewRequestWithContext(
					ctx,
					testCase.method,
					testCase.path,
					bodyReader,
				)
				if testCase.body != "" {
					request.Header.Set("Content-Type", "application/json")
				}
				router.ServeHTTP(recorder, request)
				if recorder.Code != testCase.wantStatus {
					t.Fatalf(
						"%s: got %d want %d body=%s",
						testCase.name,
						recorder.Code,
						testCase.wantStatus,
						recorder.Body.String(),
					)
				}
			}

			credentials := loginTestAuth(ctx, t, router)
			authedCases := []struct {
				name       string
				method     string
				path       string
				body       string
				wantStatus int
			}{
				{
					"change password bad json",
					http.MethodPost,
					routeChangePW,
					"{",
					http.StatusBadRequest,
				},
				{
					"change email bad json",
					http.MethodPost,
					routeChangeMail,
					"{",
					http.StatusBadRequest,
				},
				{
					"disable 2fa bad json",
					http.MethodDelete,
					routeDisable2FA,
					"{",
					http.StatusBadRequest,
				},
			}
			for _, testCase := range authedCases {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequestWithContext(
					ctx,
					testCase.method,
					testCase.path,
					bytes.NewReader([]byte(testCase.body)),
				)
				request.Header.Set("Content-Type", "application/json")
				setBearerAuth(request, credentials.AccessToken)
				router.ServeHTTP(recorder, request)
				if recorder.Code != testCase.wantStatus {
					t.Fatalf(
						"%s: got %d want %d body=%s",
						testCase.name,
						recorder.Code,
						testCase.wantStatus,
						recorder.Body.String(),
					)
				}
			}
		},
	)
}

//nolint:paralleltest,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_HandlerErrorBranches(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"auth handlers surface invalid token and login error paths",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			router := newAuthTestRouter(t)

			confirmRecorder := httptest.NewRecorder()
			confirmRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/auth/confirm-email?token=not-a-real-token",
				nil,
			)
			router.ServeHTTP(confirmRecorder, confirmRequest)
			if confirmRecorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"confirm email: got %d body=%s",
					confirmRecorder.Code,
					confirmRecorder.Body.String(),
				)
			}

			resetRecorder := httptest.NewRecorder()
			resetRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/auth/reset-password",
				bytes.NewReader([]byte(`{"token":"bad","password":"whatever-pass"}`)),
			)
			resetRequest.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(resetRecorder, resetRequest)
			if resetRecorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"reset password: got %d body=%s",
					resetRecorder.Code,
					resetRecorder.Body.String(),
				)
			}

			inviteRecorder := httptest.NewRecorder()
			inviteRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/auth/accept-invite",
				bytes.NewReader([]byte(`{"token":"bad","password":"whatever-pass"}`)),
			)
			inviteRequest.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(inviteRecorder, inviteRequest)
			if inviteRecorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"accept invite: got %d body=%s",
					inviteRecorder.Code,
					inviteRecorder.Body.String(),
				)
			}

			refreshRecorder := httptest.NewRecorder()
			refreshRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/auth/refresh",
				nil,
			)
			//nolint:gosec // G124: insecure cookie is intentional for a stale-token negative case
			refreshRequest.AddCookie(&http.Cookie{
				Name:     refreshCookieName,
				Value:    "stale-token",
				HttpOnly: true,
				Secure:   false,
				SameSite: http.SameSiteLaxMode,
			})
			router.ServeHTTP(refreshRecorder, refreshRequest)
			if refreshRecorder.Code != http.StatusUnauthorized {
				t.Fatalf(
					"refresh stale: got %d body=%s",
					refreshRecorder.Code,
					refreshRecorder.Body.String(),
				)
			}

			forgotRecorder := httptest.NewRecorder()
			forgotRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodPost,
				"/api/auth/forgot-password",
				bytes.NewReader(testEmailJSON(testAdminEmail)),
			)
			forgotRequest.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(forgotRecorder, forgotRequest)
			if forgotRecorder.Code != http.StatusNoContent {
				t.Fatalf("forgot password: got %d", forgotRecorder.Code)
			}

			credentials := loginTestAuth(ctx, t, router)
			disableRecorder := httptest.NewRecorder()
			disableRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodDelete,
				"/api/auth/2fa",
				bytes.NewReader([]byte(
					`{"password":"`+testDefaultPassword+`","code":"000000"}`,
				)),
			)
			disableRequest.Header.Set("Content-Type", "application/json")
			setBearerAuth(disableRequest, credentials.AccessToken)
			router.ServeHTTP(disableRecorder, disableRequest)
			if disableRecorder.Code != http.StatusBadRequest {
				t.Fatalf(
					"disable 2fa without setup: got %d body=%s",
					disableRecorder.Code,
					disableRecorder.Body.String(),
				)
			}

			revokeForeignRecorder := httptest.NewRecorder()
			revokeForeignRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodDelete,
				"/api/auth/sessions/00000000-0000-0000-0000-000000000000",
				nil,
			)
			setBearerAuth(revokeForeignRequest, credentials.AccessToken)
			router.ServeHTTP(revokeForeignRecorder, revokeForeignRequest)
			if revokeForeignRecorder.Code != http.StatusForbidden &&
				revokeForeignRecorder.Code != http.StatusInternalServerError {
				t.Fatalf(
					"revoke foreign session: got %d body=%s",
					revokeForeignRecorder.Code,
					revokeForeignRecorder.Body.String(),
				)
			}
		},
	)
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_RefreshCookieIssuesNewAccessToken(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "refresh endpoint returns new access token", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		ctx := context.Background()

		router := newAuthTestRouter(t)
		credentials := loginTestAuth(ctx, t, router)

		refreshRecorder := httptest.NewRecorder()
		refreshRequest := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/api/auth/refresh",
			nil,
		)
		refreshRequest.AddCookie(credentials.RefreshCookie)
		router.ServeHTTP(refreshRecorder, refreshRequest)
		if refreshRecorder.Code != http.StatusOK {
			t.Fatalf(
				"refresh status: got %d body=%s",
				refreshRecorder.Code,
				refreshRecorder.Body.String(),
			)
		}

		var refreshResponse struct {
			AccessToken string `json:"accessToken"`
		}
		err := json.NewDecoder(refreshRecorder.Body).Decode(&refreshResponse)
		if err != nil {
			t.Fatalf("decode refresh: %v", err)
		}
		if refreshResponse.AccessToken == "" {
			t.Fatal("expected refreshed access token")
		}

		assertBrowseAuthorized(t, router, refreshResponse.AccessToken)
	})
}

type testAuthCredentials struct {
	AccessToken   string
	RefreshCookie *http.Cookie
}

func loginTestAuth(ctx context.Context, t *testing.T, router *gin.Engine) testAuthCredentials {
	t.Helper()

	loginBody := testAdminLoginJSON(testDefaultPassword)
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
		t.Fatalf(
			"login status: got %d want %d body=%s",
			loginRecorder.Code,
			http.StatusOK,
			loginRecorder.Body.String(),
		)
	}

	var loginResponse struct {
		Status      string          `json:"status"`
		AccessToken string          `json:"accessToken"`
		User        auth.PublicUser `json:"user"`
	}
	err := json.NewDecoder(loginRecorder.Body).Decode(&loginResponse)
	if err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResponse.Status != "ok" {
		t.Fatalf("unexpected login status: %q", loginResponse.Status)
	}
	if loginResponse.AccessToken == "" {
		t.Fatal("expected access token")
	}
	if loginResponse.User.Email != testAdminEmail {
		t.Fatalf("unexpected login user: %+v", loginResponse.User)
	}

	var refreshCookie *http.Cookie
	for _, cookie := range loginRecorder.Result().Cookies() {
		if cookie.Name == refreshCookieName && cookie.Value != "" {
			refreshCookie = cookie
		}
	}
	if refreshCookie == nil {
		t.Fatal("expected refresh cookie")
	}

	return testAuthCredentials{
		AccessToken:   loginResponse.AccessToken,
		RefreshCookie: refreshCookie,
	}
}

func setBearerAuth(request *http.Request, accessToken string) {
	request.Header.Set("Authorization", "Bearer "+accessToken)
}

func assertBrowseAuthorized(t *testing.T, router *gin.Engine, accessToken string) {
	t.Helper()

	browseRecorder := httptest.NewRecorder()
	browseRequest := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/browse",
		nil,
	)
	setBearerAuth(browseRequest, accessToken)
	router.ServeHTTP(browseRecorder, browseRequest)

	if browseRecorder.Code != http.StatusOK {
		t.Fatalf(
			"browse status: got %d want %d body=%s",
			browseRecorder.Code,
			http.StatusOK,
			browseRecorder.Body.String(),
		)
	}
}
