package httpapi

import (
	"crypto/tls"
	"net/http"
	"os"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

// TestOIDC_AutheliaE2E is a one-shot smoke against a running Authelia overlay.
// Skipped unless SUDOSTREAM_OIDC_E2E=1 and Authelia answers on :9091.
//
//	make oidc-authelia-up
//	SUDOSTREAM_OIDC_E2E=1 go test ./internal/httpapi -run TestOIDC_AutheliaE2E -count=1
func TestOIDC_AutheliaE2E(t *testing.T) {
	if os.Getenv("SUDOSTREAM_OIDC_E2E") != "1" {
		t.Skip("set SUDOSTREAM_OIDC_E2E=1 after make oidc-authelia-up")
	}

	allure.Test(t, "authelia oidc discovery is reachable for e2e", func(a *allure.Context) {
		t := a.T()
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local Authelia TLS
			},
		}
		resp, err := client.Get("https://authelia.localhost:9091/.well-known/openid-configuration")
		if err != nil {
			t.Fatalf("authelia discovery: %v (is Authelia up?)", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("discovery status=%d", resp.StatusCode)
		}
	})
}
