package auth_test

import (
	"strings"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseJWTSecret(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		raw     string
		wantErr bool
		wantLen int
	}{
		{name: "rejects short", raw: "too-short", wantErr: true},
		{name: "accepts 32 bytes", raw: strings.Repeat("a", 32), wantLen: 32},
	} {
		allure.Test(t, "parse jwt secret — "+tc.name, func(a *allure.Context) {
			t := a.T()
			secret, err := auth.ParseJWTSecret(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}

				return
			}
			if err != nil {
				t.Fatalf("parse secret: %v", err)
			}
			if len(secret) != tc.wantLen {
				t.Fatalf("unexpected secret length: %d", len(secret))
			}
		})
	}
}

func TestNormalizeJWTSecret(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		raw  []byte
		keep bool
	}{
		{name: "pads short to dev key", raw: []byte("short"), keep: false},
		{name: "keeps long secrets", raw: []byte(strings.Repeat("b", 40)), keep: true},
	} {
		allure.Test(t, "normalize jwt secret — "+tc.name, func(a *allure.Context) {
			t := a.T()
			normalized := auth.NormalizeJWTSecret(tc.raw)
			if len(normalized) < 32 {
				t.Fatalf("expected at least 32 bytes, got %d", len(normalized))
			}
			if tc.keep && string(normalized) != string(tc.raw) {
				t.Fatal("expected unchanged secret")
			}
		})
	}
}

func TestHashToken_IsDeterministic(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HashToken returns stable hex digest", func(a *allure.Context) {
		t := a.T()
		first := auth.HashToken("refresh-token")
		second := auth.HashToken("refresh-token")
		if first != second || first == "" {
			t.Fatalf("unexpected hash: %q vs %q", first, second)
		}
	})
}
