package oidc

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestConfig_ValidateAndPublic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		cfg     Config
		wantErr bool
		public  PublicStatus
	}{
		{
			name: "disabled ok",
			cfg:  Config{Enabled: false},
		},
		{
			name: "enabled missing issuer",
			cfg: Config{
				Enabled:      true,
				ClientID:     "id",
				ClientSecret: "secret",
				BaseURL:      "http://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "enabled valid",
			cfg: Config{
				Enabled:      true,
				IssuerURL:    "http://authelia:9091",
				IssuerName:   "Authelia",
				ClientID:     "sudostream",
				ClientSecret: "secret",
				BaseURL:      "http://localhost:8080",
				Only:         true,
				AutoRedirect: true,
			},
			public: PublicStatus{
				Enabled:      true,
				IssuerName:   "Authelia",
				Only:         true,
				AutoRedirect: true,
			},
		},
	} {
		allure.Test(t, "oidc config — "+tc.name, func(a *allure.Context) {
			t := a.T()
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr {
				return
			}
			got := tc.cfg.Public()
			if got != tc.public {
				t.Fatalf("public=%+v want %+v", got, tc.public)
			}
			if !tc.cfg.Enabled {
				return
			}
			if tc.cfg.RedirectURI() != "http://localhost:8080/api/auth/oidc/callback" {
				t.Fatalf("redirect uri=%q", tc.cfg.RedirectURI())
			}
		})
	}
}

func TestPKCEChallenge_Deterministic(t *testing.T) {
	t.Parallel()

	allure.Test(t, "pkce s256 challenge is stable for verifier", func(a *allure.Context) {
		t := a.T()
		first := PKCEChallengeS256("verifier-value")
		second := PKCEChallengeS256("verifier-value")
		if first == "" || first != second {
			t.Fatalf("challenge mismatch %q vs %q", first, second)
		}
	})
}
