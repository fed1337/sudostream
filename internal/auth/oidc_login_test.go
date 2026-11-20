package auth_test

import (
	"context"
	"errors"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestCompleteOIDCLogin_ResolutionPaths(t *testing.T) {
	ctx := context.Background()
	service, _ := newAuthIntegration(ctx, t)

	local, err := service.CreateUser(ctx, auth.CreateUserInput{
		Email:    "merge-me@example.com",
		Password: "secret-pass",
		Role:     auth.RoleUser,
	})
	if err != nil {
		t.Fatalf("create local user: %v", err)
	}

	for _, tc := range []struct {
		name      string
		input     auth.OIDCLoginInput
		wantErr   error
		wantRole  string
		wantEmail string
	}{
		{
			name: "deny unknown when flags off",
			input: auth.OIDCLoginInput{
				Issuer:        "https://idp.example",
				Subject:       "sub-unknown",
				Email:         "new@example.com",
				EmailVerified: true,
			},
			wantErr: auth.ErrOIDCUnknownIdentity,
		},
		{
			name: "merge by email",
			input: auth.OIDCLoginInput{
				Issuer:        "https://idp.example",
				Subject:       "sub-merge",
				Email:         "merge-me@example.com",
				EmailVerified: true,
				MergeUsers:    true,
			},
			wantEmail: local.Email,
			wantRole:  auth.RoleUser,
		},
		{
			name: "auto register user",
			input: auth.OIDCLoginInput{
				Issuer:        "https://idp.example",
				Subject:       "sub-register",
				Email:         "oidc-new@example.com",
				EmailVerified: true,
				AutoRegister:  true,
			},
			wantEmail: "oidc-new@example.com",
			wantRole:  auth.RoleUser,
		},
		{
			name: "auto register admin group",
			input: auth.OIDCLoginInput{
				Issuer:        "https://idp.example",
				Subject:       "sub-admin",
				Email:         "oidc-admin@example.com",
				EmailVerified: true,
				Groups:        []string{"sudostream-admins"},
				AdminGroup:    "sudostream-admins",
				AutoRegister:  true,
			},
			wantEmail: "oidc-admin@example.com",
			wantRole:  auth.RoleAdmin,
		},
		{
			name: "require email verification blocks register",
			input: auth.OIDCLoginInput{
				Issuer:                   "https://idp.example",
				Subject:                  "sub-unverified",
				Email:                    "unverified@example.com",
				EmailVerified:            false,
				AutoRegister:             true,
				RequireEmailVerification: true,
			},
			wantErr: auth.ErrOIDCEmailNotVerified,
		},
	} {
		allure.Test(t, "oidc login — "+tc.name, func(a *allure.Context) {
			t := a.T()
			outcome, err := service.CompleteOIDCLogin(ctx, tc.input, auth.SessionMeta{})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err=%v want %v", err, tc.wantErr)
				}

				return
			}
			if err != nil {
				t.Fatalf("CompleteOIDCLogin: %v", err)
			}
			if outcome.Status != "ok" || outcome.AccessToken == "" || outcome.RefreshToken == "" {
				t.Fatalf("unexpected outcome: %+v", outcome)
			}
			if outcome.User.Email != tc.wantEmail || outcome.User.Role != tc.wantRole {
				t.Fatalf("user=%+v want email=%s role=%s", outcome.User, tc.wantEmail, tc.wantRole)
			}
			again, err := service.CompleteOIDCLogin(ctx, tc.input, auth.SessionMeta{})
			if err != nil {
				t.Fatalf("second login: %v", err)
			}
			if again.User.ID != outcome.User.ID {
				t.Fatalf("mapping reused? %s vs %s", again.User.ID, outcome.User.ID)
			}
		})
	}
}
