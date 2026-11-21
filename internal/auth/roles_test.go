package auth

import (
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestValidRole(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		role  string
		valid bool
	}{
		{name: "admin", role: RoleAdmin, valid: true},
		{name: "user", role: RoleUser, valid: true},
		{name: "tv", role: RoleTV, valid: true},
		{name: "unknown", role: "moderator", valid: false},
	} {
		allure.Test(t, "valid role — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if ValidRole(tc.role) != tc.valid {
				t.Fatalf("ValidRole(%q)=%v want %v", tc.role, !tc.valid, tc.valid)
			}
		})
	}
}

func TestAllowsWebLogin(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		role string
		want bool
	}{
		{name: "user", role: RoleUser, want: true},
		{name: "admin", role: RoleAdmin, want: true},
		{name: "tv blocked", role: RoleTV, want: false},
	} {
		allure.Test(t, "allows web login — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if AllowsWebLogin(tc.role) != tc.want {
				t.Fatalf("AllowsWebLogin(%q)=%v want %v", tc.role, !tc.want, tc.want)
			}
		})
	}
}

func TestNormalizeRole(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{name: "empty defaults user", in: "", want: RoleUser},
		{name: "unknown rejected", in: "nope", wantErr: ErrInvalidRole},
	} {
		allure.Test(t, "normalize role — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got, err := NormalizeRole(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err=%v want %v", err, tc.wantErr)
				}

				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v want %q", got, err, tc.want)
			}
		})
	}
}
