package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sudoStream/internal/auth/oidc"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestOIDCStatus_DisabledByDefault(t *testing.T) {
	t.Parallel()

	allure.Test(t, "oidc status returns disabled when provider unset", func(a *allure.Context) {
		t := a.T()
		router := newAuthTestRouter(t)

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

	allure.Test(t, "oidc start returns 404 when disabled", func(a *allure.Context) {
		t := a.T()
		router := newAuthTestRouter(t)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}
