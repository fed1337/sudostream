package auth_test

import (
	"context"
	"errors"
	"sudoStream/internal/auth"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestAuthTokens_ExpiredAndUsedRejected(t *testing.T) {
	ctx := context.Background()
	service, sender, store := newAuthIntegrationStore(ctx, t)

	user, err := service.CreateUser(ctx, auth.CreateUserInput{
		Email:    "token-life@example.com",
		Password: testAdminPassword,
		Role:     auth.RoleUser,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	allure.Test(t, "expired reset token is rejected", func(a *allure.Context) {
		t := a.T()
		raw := "expired-reset-token-value"
		err := store.CreateAuthToken(ctx, auth.Token{
			UserID:    user.ID,
			Email:     user.Email,
			Purpose:   auth.TokenPurposeResetPassword,
			TokenHash: auth.HashToken(raw),
			ExpiresAt: time.Now().UTC().Add(-time.Hour),
		})
		if err != nil {
			t.Fatalf("seed expired reset token: %v", err)
		}
		err = service.ResetPassword(ctx, raw, testNewPassword)
		if !errors.Is(err, auth.ErrTokenExpired) {
			t.Fatalf("err=%v want %v", err, auth.ErrTokenExpired)
		}
	})

	allure.Test(t, "used reset token is rejected", func(a *allure.Context) {
		t := a.T()
		err := service.RequestPasswordReset(ctx, user.Email)
		if err != nil {
			t.Fatalf("request reset: %v", err)
		}
		raw := sender.lastToken(t)
		err = service.ResetPassword(ctx, raw, testNewPassword)
		if err != nil {
			t.Fatalf("first reset: %v", err)
		}
		err = service.ResetPassword(ctx, raw, "another-password-789")
		if !errors.Is(err, auth.ErrTokenUsed) {
			t.Fatalf("reuse reset: err=%v want %v", err, auth.ErrTokenUsed)
		}
	})

	allure.Test(t, "expired invite token is rejected", func(a *allure.Context) {
		t := a.T()
		raw := "expired-invite-token-value"
		err := store.CreateAuthToken(ctx, auth.Token{
			UserID:    user.ID,
			Email:     "expired-invite@example.com",
			Purpose:   auth.TokenPurposeInvite,
			TokenHash: auth.HashToken(raw),
			Role:      auth.RoleUser,
			ExpiresAt: time.Now().UTC().Add(-time.Minute),
		})
		if err != nil {
			t.Fatalf("seed expired invite: %v", err)
		}
		err = service.AcceptInvite(ctx, raw, testNewPassword)
		if !errors.Is(err, auth.ErrTokenExpired) {
			t.Fatalf("err=%v want %v", err, auth.ErrTokenExpired)
		}
	})

	allure.Test(t, "used invite token is rejected", func(a *allure.Context) {
		t := a.T()
		err := service.CreateInvite(ctx, "invite-once@example.com", auth.RoleUser, "", time.Hour)
		if err != nil {
			t.Fatalf("create invite: %v", err)
		}
		raw := sender.lastToken(t)
		err = service.AcceptInvite(ctx, raw, testNewPassword)
		if err != nil {
			t.Fatalf("first accept: %v", err)
		}
		err = service.AcceptInvite(ctx, raw, "another-password-789")
		if !errors.Is(err, auth.ErrTokenUsed) {
			t.Fatalf("reuse invite: err=%v want %v", err, auth.ErrTokenUsed)
		}
	})

	allure.Test(t, "expired confirm-email token is rejected", func(a *allure.Context) {
		t := a.T()
		raw := "expired-confirm-token-value"
		err := store.CreateAuthToken(ctx, auth.Token{
			UserID:    user.ID,
			Email:     user.Email,
			Purpose:   auth.TokenPurposeConfirmEmail,
			TokenHash: auth.HashToken(raw),
			ExpiresAt: time.Now().UTC().Add(-time.Hour),
		})
		if err != nil {
			t.Fatalf("seed expired confirm: %v", err)
		}
		err = service.ConfirmEmail(ctx, raw)
		if !errors.Is(err, auth.ErrTokenExpired) {
			t.Fatalf("err=%v want %v", err, auth.ErrTokenExpired)
		}
	})
}

func TestAuthTokens_SamePasswordRejectedEverywhere(t *testing.T) {
	ctx := context.Background()
	service, sender := newAuthIntegration(ctx, t)

	allure.Test(t, "change password rejects same password", func(a *allure.Context) {
		t := a.T()
		member, err := service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "same-pw-change@example.com",
			Password: testAdminPassword,
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		_, _, _, err = service.ChangePassword(
			ctx, member.ID, "", testNewPassword, "", defaultSessionMeta(),
		)
		if err != nil {
			t.Fatalf("clear must-change: %v", err)
		}
		_, _, _, err = service.ChangePassword(
			ctx, member.ID, testNewPassword, testNewPassword, "", defaultSessionMeta(),
		)
		if !errors.Is(err, auth.ErrPasswordUnchanged) {
			t.Fatalf("err=%v want %v", err, auth.ErrPasswordUnchanged)
		}
	})

	allure.Test(t, "reset password rejects same password", func(a *allure.Context) {
		t := a.T()
		_, err := service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "same-pw-reset@example.com",
			Password: testAdminPassword,
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		err = service.RequestPasswordReset(ctx, "same-pw-reset@example.com")
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		err = service.ResetPassword(ctx, sender.lastToken(t), testAdminPassword)
		if !errors.Is(err, auth.ErrPasswordUnchanged) {
			t.Fatalf("err=%v want %v", err, auth.ErrPasswordUnchanged)
		}
	})

	allure.Test(t, "accept invite rejects same password for existing account", func(a *allure.Context) {
		t := a.T()
		disabled, err := service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "same-pw-invite@example.com",
			Password: testAdminPassword,
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		enabled := false
		_, err = service.UpdateUser(ctx, disabled.ID, auth.UserPatch{Enabled: &enabled})
		if err != nil {
			t.Fatalf("disable user: %v", err)
		}
		err = service.CreateInvite(ctx, "same-pw-invite@example.com", auth.RoleUser, "", time.Hour)
		if err != nil {
			t.Fatalf("invite: %v", err)
		}
		err = service.AcceptInvite(ctx, sender.lastToken(t), testAdminPassword)
		if !errors.Is(err, auth.ErrPasswordUnchanged) {
			t.Fatalf("err=%v want %v", err, auth.ErrPasswordUnchanged)
		}
	})
}
