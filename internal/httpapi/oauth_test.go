package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/email"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

func oauthPasswordGrantForm() url.Values {
	return oauthPasswordForm(testDefaultPassword)
}

func oauthPasswordForm(password string) url.Values {
	return url.Values{
		oauthFormKeyGrantType: {oauthGrantPassword},
		"username":            {testAdminEmail},
		"password":            {password},
	}
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_PasswordGrantIssuesTokens(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "password grant returns bearer tokens", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)
		form := oauthPasswordGrantForm()

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/oauth/token",
			strings.NewReader(form.Encode()),
		)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf(
				"token status: got %d want %d body=%s",
				recorder.Code,
				http.StatusOK,
				recorder.Body.String(),
			)
		}

		var response OAuthTokenResponse
		err := json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode token response: %v", err)
		}
		if response.AccessToken == "" || response.RefreshToken == "" {
			t.Fatalf("expected tokens: %+v", response)
		}
		if response.TokenType != oauthTokenTypeBearer {
			t.Fatalf("unexpected token type: %q", response.TokenType)
		}
		if response.ExpiresIn <= 0 {
			t.Fatalf("expected positive expires_in, got %d", response.ExpiresIn)
		}

		meRecorder := httptest.NewRecorder()
		meRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/auth/me",
			nil,
		)
		setBearerAuth(meRequest, response.AccessToken)
		router.ServeHTTP(meRecorder, meRequest)

		if meRecorder.Code != http.StatusOK {
			t.Fatalf("me status: got %d body=%s", meRecorder.Code, meRecorder.Body.String())
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_PasswordGrantRejectsInvalidCredentials(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "password grant rejects bad password", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)
		form := oauthPasswordForm("wrong")

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/oauth/token",
			strings.NewReader(form.Encode()),
		)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var response OAuthErrorResponse
		err := json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if response.Error != oauthErrorInvalidGrant {
			t.Fatalf("unexpected error code: %q", response.Error)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_RefreshGrantRotatesAccessToken(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "refresh_token grant returns new access token", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)
		form := oauthPasswordGrantForm()

		issueRecorder := httptest.NewRecorder()
		issueRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/oauth/token",
			strings.NewReader(form.Encode()),
		)
		issueRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(issueRecorder, issueRequest)

		var issued OAuthTokenResponse
		err := json.NewDecoder(issueRecorder.Body).Decode(&issued)
		if err != nil {
			t.Fatalf("decode issued token: %v", err)
		}

		refreshForm := url.Values{
			oauthFormKeyGrantType:    {oauthGrantRefreshToken},
			oauthFormKeyRefreshToken: {issued.RefreshToken},
		}
		refreshRecorder := httptest.NewRecorder()
		refreshRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/oauth/token",
			strings.NewReader(refreshForm.Encode()),
		)
		refreshRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(refreshRecorder, refreshRequest)

		if refreshRecorder.Code != http.StatusOK {
			t.Fatalf(
				"refresh status: got %d body=%s",
				refreshRecorder.Code,
				refreshRecorder.Body.String(),
			)
		}

		var refreshed OAuthTokenResponse
		err = json.NewDecoder(refreshRecorder.Body).Decode(&refreshed)
		if err != nil {
			t.Fatalf("decode refreshed token: %v", err)
		}
		if refreshed.AccessToken == "" {
			t.Fatal("expected refreshed access token")
		}
		if refreshed.RefreshToken == "" {
			t.Fatal("expected rotated refresh token in response")
		}
		if refreshed.RefreshToken == issued.RefreshToken {
			t.Fatal("expected refresh token rotation")
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_UnsupportedGrantType(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "unsupported grant_type returns oauth error", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)
		form := url.Values{oauthFormKeyGrantType: {"client_credentials"}}

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/oauth/token",
			strings.NewReader(form.Encode()),
		)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: got %d", recorder.Code)
		}

		var response OAuthErrorResponse
		err := json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if response.Error != oauthErrorUnsupported {
			t.Fatalf("unexpected error: %q", response.Error)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_PasswordGrantRequiresMFAWhenConfigured(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"password grant returns mfa_required when 2FA policy enabled",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			ctx := context.Background()
			pool := setupTestPool(ctx, t)
			authService := prepareIntegrationAuth(ctx, t, pool)
			enableRequiredTwoFactor(ctx, t, authService)

			router := newAuthTestRouterFromService(t, authService)
			recorder := serveOAuthTokenForm(ctx, router, oauthPasswordGrantForm())
			assertOAuthMFAGrantWithoutSession(ctx, t, recorder, authService)
		},
	)
}

func enableRequiredTwoFactor(ctx context.Context, t *testing.T, authService *auth.Service) {
	t.Helper()

	_, err := authService.UpdateSettings(ctx, auth.Settings{
		EmailConfirmationRequired: false,
		TwoFactorRequired:         true,
	})
	if err != nil {
		t.Fatalf("update settings: %v", err)
	}
}

func assertOAuthMFAGrantWithoutSession(
	ctx context.Context,
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	authService *auth.Service,
) {
	t.Helper()

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var response OAuthErrorResponse
	err := json.NewDecoder(recorder.Body).Decode(&response)
	if err != nil {
		t.Fatalf("decode mfa response: %v", err)
	}
	if response.Error != oauthErrorMFARequired || response.PendingToken == "" {
		t.Fatalf("unexpected mfa response: %+v", response)
	}
	if strings.Contains(recorder.Body.String(), "access_token") ||
		strings.Contains(recorder.Body.String(), "refresh_token") {
		t.Fatal("mfa challenge must not return token material")
	}

	assertUserHasNoSessions(ctx, t, authService)
}

func assertUserHasNoSessions(ctx context.Context, t *testing.T, authService *auth.Service) {
	t.Helper()

	users, err := authService.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected seeded admin, got %d users", len(users))
	}

	sessions, err := authService.ListUserSessions(ctx, users[0].ID, "")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("mfa challenge created %d sessions", len(sessions))
	}
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_PasswordGrantCompletesConfiguredTOTP(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "password grant accepts totp in the same request", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		ctx := context.Background()
		pool := setupTestPool(ctx, t)
		authService := prepareIntegrationAuth(ctx, t, pool)
		secret := confirmAdminTOTP(ctx, t, authService)

		router := newAuthTestRouterFromService(t, authService)
		assertInvalidOAuthTOTPRejected(ctx, t, router, authService)

		freshCode, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("generate totp: %v", err)
		}

		form := oauthPasswordGrantForm()
		form.Set("totp", freshCode)
		recorder := serveOAuthTokenForm(ctx, router, form)
		if recorder.Code != http.StatusOK {
			t.Fatalf("totp token status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var response OAuthTokenResponse
		err = json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode totp token response: %v", err)
		}
		if response.AccessToken == "" || response.RefreshToken == "" {
			t.Fatalf("expected token pair: %+v", response)
		}
	})
}

func confirmAdminTOTP(
	ctx context.Context,
	t *testing.T,
	authService *auth.Service,
) string {
	t.Helper()

	users, err := authService.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected seeded admin, got %d users", len(users))
	}

	setup, err := authService.StartTwoFactorSetup(ctx, users[0].ID, testAdminEmail)
	if err != nil {
		t.Fatalf("start 2fa setup: %v", err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	_, err = authService.ConfirmTwoFactorSetup(ctx, users[0].ID, code)
	if err != nil {
		t.Fatalf("confirm 2fa setup: %v", err)
	}

	return setup.Secret
}

func assertInvalidOAuthTOTPRejected(
	ctx context.Context,
	t *testing.T,
	router *gin.Engine,
	authService *auth.Service,
) {
	t.Helper()

	invalidForm := oauthPasswordGrantForm()
	invalidForm.Set("totp", "000000")
	invalidRecorder := serveOAuthTokenForm(ctx, router, invalidForm)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"invalid totp status: got %d body=%s",
			invalidRecorder.Code,
			invalidRecorder.Body.String(),
		)
	}

	var invalidResponse OAuthErrorResponse
	err := json.NewDecoder(invalidRecorder.Body).Decode(&invalidResponse)
	if err != nil {
		t.Fatalf("decode invalid totp response: %v", err)
	}
	if invalidResponse.Error != oauthErrorMFARequired {
		t.Fatalf("unexpected invalid totp error: %+v", invalidResponse)
	}

	users, err := authService.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users after invalid totp: %v", err)
	}
	sessions, err := authService.ListUserSessions(ctx, users[0].ID, "")
	if err != nil {
		t.Fatalf("list sessions after invalid totp: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("invalid totp created %d sessions", len(sessions))
	}
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_DisabledUserCannotObtainToken(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "password grant rejects disabled account", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router, _ := newACLTestRouter(t)
		ctx := context.Background()

		createBody := []byte(
			`{"email":"oauth-disabled@hpserver.lan","password":"secret-pass","role":"user"}`,
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

		form := url.Values{
			oauthFormKeyGrantType: {oauthGrantPassword},
			"username":            {"oauth-disabled@hpserver.lan"},
			"password":            {"secret-pass"},
		}
		recorder := serveOAuthTokenForm(ctx, router, form)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf(
				"disabled user token status: got %d body=%s",
				recorder.Code,
				recorder.Body.String(),
			)
		}

		var response OAuthErrorResponse
		err = json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if response.Error != oauthErrorInvalidGrant {
			t.Fatalf("unexpected error code: %q", response.Error)
		}
		if strings.Contains(recorder.Body.String(), "access_token") {
			t.Fatal("disabled user response must not contain access_token")
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_RefreshGrantRejectsReplayedToken(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "replayed refresh_token is rejected after rotation", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		router := newAuthTestRouter(t)
		ctx := context.Background()
		issueRecorder := serveOAuthTokenForm(ctx, router, oauthPasswordGrantForm())

		var issued OAuthTokenResponse
		err := json.NewDecoder(issueRecorder.Body).Decode(&issued)
		if err != nil {
			t.Fatalf("decode issued token: %v", err)
		}

		refreshForm := url.Values{
			oauthFormKeyGrantType:    {oauthGrantRefreshToken},
			oauthFormKeyRefreshToken: {issued.RefreshToken},
		}
		refreshRecorder := serveOAuthTokenForm(ctx, router, refreshForm)
		if refreshRecorder.Code != http.StatusOK {
			t.Fatalf(
				"refresh status: got %d body=%s",
				refreshRecorder.Code,
				refreshRecorder.Body.String(),
			)
		}

		replayRecorder := serveOAuthTokenForm(ctx, router, refreshForm)
		if replayRecorder.Code != http.StatusUnauthorized {
			t.Fatalf(
				"replay refresh status: got %d body=%s",
				replayRecorder.Code,
				replayRecorder.Body.String(),
			)
		}

		var replayResponse OAuthErrorResponse
		err = json.NewDecoder(replayRecorder.Body).Decode(&replayResponse)
		if err != nil {
			t.Fatalf("decode replay error: %v", err)
		}
		if replayResponse.Error != oauthErrorInvalidGrant {
			t.Fatalf("unexpected replay error: %q", replayResponse.Error)
		}
	})
}

func serveOAuthTokenForm(
	ctx context.Context,
	router *gin.Engine,
	form url.Values,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"/api/oauth/token",
		strings.NewReader(form.Encode()),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(recorder, request)

	return recorder
}

func newAuthTestRouterFromService(t *testing.T, authService *auth.Service) *gin.Engine {
	t.Helper()

	setupTestCacheEnv(t)

	root := t.TempDir()
	router := gin.New()
	media, err := newMediaAtRoot(t, root)
	if err != nil {
		t.Fatalf("media service: %v", err)
	}

	RegisterRoutes(router, media, authService, nil, nil, nil, integrationRouteConfig(root))

	return router
}

func newAuthTestRouterWithMail(t *testing.T, mail email.Sender) *gin.Engine {
	t.Helper()

	ctx := context.Background()
	pool := setupTestPool(ctx, t)
	authService := prepareIntegrationAuthWithMail(ctx, t, pool, mail)

	return newAuthTestRouterFromService(t, authService)
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestOAuth_MissingFields(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "oauth token validates required form fields", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		router := newAuthTestRouter(t)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/oauth/token",
			strings.NewReader(oauthFormKeyGrantType+"="+oauthGrantRefreshToken),
		)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("missing refresh token: got %d", recorder.Code)
		}

		emptyGrantRecorder := httptest.NewRecorder()
		emptyGrantRequest := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/oauth/token",
			strings.NewReader(""),
		)
		emptyGrantRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		router.ServeHTTP(emptyGrantRecorder, emptyGrantRequest)
		if emptyGrantRecorder.Code != http.StatusBadRequest {
			t.Fatalf("missing grant type: got %d", emptyGrantRecorder.Code)
		}
	})
}

//nolint:paralleltest // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestAuth_RefreshWithoutCookie(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "refresh without cookie returns unauthorized", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		router := newAuthTestRouter(t)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/auth/refresh",
			nil,
		)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("refresh without cookie: got %d", recorder.Code)
		}
	})
}
