package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/auth/oidc"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func newOIDCUnitRouter(h *authHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/auth/oidc/status", h.oidcStatus)
	router.GET("/api/auth/oidc/start", h.oidcStart)
	router.GET("/api/auth/oidc/callback", h.oidcCallback)

	return router
}

func TestOIDC_DisabledRouteMatrix(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
		check      func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:       "status reports disabled",
			path:       "/api/auth/oidc/status",
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var body oidc.PublicStatus
				err := json.Unmarshal(rec.Body.Bytes(), &body)
				if err != nil {
					t.Fatalf("decode: %v", err)
				}
				if body.Enabled {
					t.Fatalf("expected disabled status: %+v", body)
				}
			},
		},
		{
			name:       "start not found",
			path:       "/api/auth/oidc/start",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "callback redirects to login",
			path:       "/api/auth/oidc/callback?code=x&state=y",
			wantStatus: http.StatusFound,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				loc := rec.Header().Get("Location")
				if !strings.Contains(loc, "/login") || !strings.Contains(loc, "oidc_error=") {
					t.Fatalf("unexpected Location: %q", loc)
				}
			},
		},
	} {
		allure.Test(t, "oidc disabled path matrix — "+tc.name, func(a *allure.Context) {
			t := a.T()
			router := newOIDCUnitRouter(&authHandler{config: RouteConfig{}})

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

func TestMapOIDCError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "unknown identity", err: auth.ErrOIDCUnknownIdentity, want: "oidc account not linked"},
		{name: "email not verified", err: auth.ErrOIDCEmailNotVerified, want: "oidc email not verified"},
		{name: "email required", err: auth.ErrOIDCEmailRequired, want: "oidc email required"},
		{name: "user exists", err: auth.ErrUserExists, want: "oidc email already registered"},
		{name: "disabled", err: auth.ErrUserDisabled, want: "account disabled"},
		{name: "tv web", err: auth.ErrTVWebAuth, want: "tv accounts cannot use web login"},
		{name: "invalid state", err: oidc.ErrInvalidState, want: "oidc login failed"},
		{name: "missing code", err: oidc.ErrMissingCode, want: "oidc login failed"},
		{name: "default", err: errors.New("boom"), want: "oidc login failed"},
	} {
		allure.Test(t, "map oidc error — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if got := mapOIDCError(tc.err); got != tc.want {
				t.Fatalf("mapOIDCError(%v)=%q want %q", tc.err, got, tc.want)
			}
		})
	}
}
