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

func TestOIDCStatus_NilProvider(t *testing.T) {
	t.Parallel()

	allure.Test(t, "oidc status returns disabled when provider unset", func(a *allure.Context) {
		t := a.T()
		router := newOIDCUnitRouter(&authHandler{config: RouteConfig{}})

		req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/status", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var body oidc.PublicStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Enabled {
			t.Fatalf("expected disabled status: %+v", body)
		}
	})
}

func TestOIDCStart_NotFoundWhenDisabled(t *testing.T) {
	t.Parallel()

	allure.Test(t, "oidc start returns 404 when provider unset", func(a *allure.Context) {
		t := a.T()
		router := newOIDCUnitRouter(&authHandler{config: RouteConfig{}})

		req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestOIDCCallback_RedirectsWhenDisabled(t *testing.T) {
	t.Parallel()

	allure.Test(t, "oidc callback redirects to login when disabled", func(a *allure.Context) {
		t := a.T()
		router := newOIDCUnitRouter(&authHandler{config: RouteConfig{}})

		req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?code=x&state=y", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "/login") || !strings.Contains(loc, "oidc_error=") {
			t.Fatalf("unexpected Location: %q", loc)
		}
	})
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
