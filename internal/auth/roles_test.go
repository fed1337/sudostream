package auth

import (
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestValidRole_KnownRoles(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ValidRole accepts admin user tv and rejects unknown", func(a *allure.Context) {
		t := a.T()
		for _, role := range []string{RoleAdmin, RoleUser, RoleTV} {
			if !ValidRole(role) {
				t.Fatalf("expected %q valid", role)
			}
		}
		if ValidRole("moderator") {
			t.Fatal("expected moderator invalid")
		}
	})
}

func TestAllowsWebLogin_TVBlocked(t *testing.T) {
	t.Parallel()

	allure.Test(t, "TV role cannot use web login", func(a *allure.Context) {
		t := a.T()
		if !AllowsWebLogin(RoleUser) || !AllowsWebLogin(RoleAdmin) {
			t.Fatal("expected admin/user web login")
		}
		if AllowsWebLogin(RoleTV) {
			t.Fatal("expected TV web login blocked")
		}
	})
}

func TestNormalizeRole_DefaultsAndErrors(t *testing.T) {
	t.Parallel()

	allure.Test(t, "NormalizeRole defaults empty and rejects unknown", func(a *allure.Context) {
		t := a.T()
		got, err := NormalizeRole("")
		if err != nil || got != RoleUser {
			t.Fatalf("empty: got %q %v", got, err)
		}
		_, err = NormalizeRole("nope")
		if !errors.Is(err, ErrInvalidRole) {
			t.Fatalf("expected ErrInvalidRole, got %v", err)
		}
	})
}
