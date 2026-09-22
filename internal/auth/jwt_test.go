package auth_test

import (
	"strings"
	"sudoStream/internal/auth"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseJWTSecret_RejectsShortSecret(t *testing.T) {
	t.Parallel()

	allure.Test(t, "short jwt secret is rejected", func(a *allure.Context) {
		t := a.T()
		_, err := auth.ParseJWTSecret("too-short")
		if err == nil {
			t.Fatal("expected error for short secret")
		}
	})
}

func TestParseJWTSecret_AcceptsValidSecret(t *testing.T) {
	t.Parallel()

	allure.Test(t, "valid jwt secret is accepted", func(a *allure.Context) {
		t := a.T()
		raw := strings.Repeat("a", 32)
		secret, err := auth.ParseJWTSecret(raw)
		if err != nil {
			t.Fatalf("parse secret: %v", err)
		}
		if len(secret) != 32 {
			t.Fatalf("unexpected secret length: %d", len(secret))
		}
	})
}

func TestNormalizeJWTSecret_PadsShortSecrets(t *testing.T) {
	t.Parallel()

	allure.Test(t, "short secrets fall back to dev key", func(a *allure.Context) {
		t := a.T()
		normalized := auth.NormalizeJWTSecret([]byte("short"))
		if len(normalized) < 32 {
			t.Fatalf("expected at least 32 bytes, got %d", len(normalized))
		}
	})

	allure.Test(t, "long secrets are returned unchanged", func(a *allure.Context) {
		t := a.T()
		raw := []byte(strings.Repeat("b", 40))
		normalized := auth.NormalizeJWTSecret(raw)
		if string(normalized) != string(raw) {
			t.Fatal("expected unchanged secret")
		}
	})
}

func TestDefaultTokenTTLs(t *testing.T) {
	t.Parallel()

	allure.Test(t, "default access and refresh TTLs are positive", func(a *allure.Context) {
		t := a.T()
		if auth.DefaultAccessTTL() <= 0 {
			t.Fatal("expected positive access TTL")
		}
		if auth.DefaultRefreshTTL() <= auth.DefaultAccessTTL() {
			t.Fatal("expected refresh TTL longer than access TTL")
		}
		if auth.DefaultAccessTTL() != 15*time.Minute {
			t.Fatalf("unexpected access TTL: %v", auth.DefaultAccessTTL())
		}
	})
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

func TestUserPublic_MapsCoreFields(t *testing.T) {
	t.Parallel()

	allure.Test(t, "User.Public copies identity fields", func(a *allure.Context) {
		t := a.T()
		user := auth.User{
			ID:                 "user-1",
			Email:              "viewer@example.com",
			Role:               auth.RoleUser,
			MustChangePassword: true,
		}
		public := user.Public()
		if public.ID != user.ID || public.Email != user.Email || public.Role != user.Role {
			t.Fatalf("unexpected public user: %+v", public)
		}
		if !public.MustChangePassword {
			t.Fatal("expected mustChangePassword flag")
		}
	})
}
