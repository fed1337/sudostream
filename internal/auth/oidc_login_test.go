package auth

import (
	"context"
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestEnsureOIDCEmailPolicy(t *testing.T) {
	t.Parallel()

	svc := &Service{}

	for _, tc := range []struct {
		name    string
		input   OIDCLoginInput
		wantErr error
	}{
		{
			name: "ok verified",
			input: OIDCLoginInput{
				Email:         "a@example.com",
				EmailVerified: true,
			},
		},
		{
			name: "ok without require verify",
			input: OIDCLoginInput{
				Email:         "a@example.com",
				EmailVerified: false,
			},
		},
		{
			name: "missing email",
			input: OIDCLoginInput{
				Email:         "  ",
				EmailVerified: true,
			},
			wantErr: ErrOIDCEmailRequired,
		},
		{
			name: "require verification blocks",
			input: OIDCLoginInput{
				Email:                    "a@example.com",
				EmailVerified:            false,
				RequireEmailVerification: true,
			},
			wantErr: ErrOIDCEmailNotVerified,
		},
	} {
		allure.Test(t, "oidc email policy — "+tc.name, func(a *allure.Context) {
			t := a.T()
			err := svc.ensureOIDCEmailPolicy(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err=%v want %v", err, tc.wantErr)
				}

				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestOIDCHasAdminGroup(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		groups     []string
		adminGroup string
		want       bool
	}{
		{name: "empty admin group", groups: []string{"admins"}, adminGroup: "", want: false},
		{name: "match", groups: []string{"sudostream-admins"}, adminGroup: "sudostream-admins", want: true},
		{name: "case insensitive", groups: []string{"Admins"}, adminGroup: "admins", want: true},
		{name: "no match", groups: []string{"users"}, adminGroup: "admins", want: false},
		{name: "trim spaces", groups: []string{"  admins  "}, adminGroup: "admins", want: true},
	} {
		allure.Test(t, "oidc admin group — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if got := oidcHasAdminGroup(tc.groups, tc.adminGroup); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCompleteOIDCLogin_IncompleteIdentity(t *testing.T) {
	t.Parallel()

	allure.Test(t, "oidc login rejects missing issuer or subject", func(a *allure.Context) {
		t := a.T()
		svc := &Service{}
		_, err := svc.CompleteOIDCLogin(context.Background(), OIDCLoginInput{
			Email: "a@example.com",
		}, SessionMeta{})
		if !errors.Is(err, errOIDCIdentityIncomplete) {
			t.Fatalf("err=%v want %v", err, errOIDCIdentityIncomplete)
		}
	})
}

func TestProvisionOIDCUser_Flags(t *testing.T) {
	t.Parallel()

	svc := &Service{}

	allure.Test(t, "oidc provision denies unknown when flags off", func(a *allure.Context) {
		t := a.T()
		_, err := svc.provisionOIDCUser(context.Background(), OIDCLoginInput{
			Issuer:        "https://idp.example",
			Subject:       "sub",
			Email:         "new@example.com",
			EmailVerified: true,
		})
		if !errors.Is(err, ErrOIDCUnknownIdentity) {
			t.Fatalf("err=%v want %v", err, ErrOIDCUnknownIdentity)
		}
	})

	allure.Test(t, "oidc provision requires email before merge/register", func(a *allure.Context) {
		t := a.T()
		_, err := svc.provisionOIDCUser(context.Background(), OIDCLoginInput{
			Issuer:        "https://idp.example",
			Subject:       "sub",
			AutoRegister:  true,
			EmailVerified: true,
		})
		if !errors.Is(err, ErrOIDCEmailRequired) {
			t.Fatalf("err=%v want %v", err, ErrOIDCEmailRequired)
		}
	})
}
