package auth_test

import (
	"context"
	"errors"
	"sudoStream/internal/auth"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/pquerna/otp/totp"
)

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthTokens_TwoFactorGatesChangeEmailAndPassword(t *testing.T) {
	allure.Test(
		t,
		"2FA required for change-email and change-password token flows",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender := newAuthIntegration(ctx, t)
			_ = seedAdminUser(ctx, t, service)

			member, err := service.CreateUser(ctx, auth.CreateUserInput{
				Email:    "totp-tokens@example.com",
				Password: testAdminPassword,
				Role:     auth.RoleUser,
			})
			if err != nil {
				t.Fatalf("create member: %v", err)
			}
			_, access, refresh, err := service.ChangePassword(
				ctx, member.ID, "", testNewPassword, "", defaultSessionMeta(),
			)
			if err != nil {
				t.Fatalf("clear must-change password: %v", err)
			}
			if access == "" || refresh == "" {
				t.Fatal("expected tokens after clearing must-change password")
			}

			secret := enrollTOTP(ctx, t, service, member.ID, member.Email)
			assertChangeEmailTOTP(ctx, t, service, sender, member.ID, secret)
			assertChangePasswordTOTP(ctx, t, service, member.ID, secret)
		},
	)
}

func enrollTOTP(
	ctx context.Context,
	t *testing.T,
	service *auth.Service,
	userID, email string,
) string {
	t.Helper()

	setup, err := service.StartTwoFactorSetup(ctx, userID, email)
	if err != nil {
		t.Fatalf("start 2FA: %v", err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate setup code: %v", err)
	}
	_, err = service.ConfirmTwoFactorSetup(ctx, userID, code)
	if err != nil {
		t.Fatalf("confirm 2FA: %v", err)
	}

	return setup.Secret
}

func assertChangeEmailTOTP(
	ctx context.Context,
	t *testing.T,
	service *auth.Service,
	sender *captureMailSender,
	userID, secret string,
) {
	t.Helper()

	err := service.RequestChangeEmail(
		ctx,
		userID,
		"totp-tokens-new@example.com",
		testNewPassword,
		"",
	)
	if !errors.Is(err, auth.ErrTwoFactorRequired) {
		t.Fatalf("expected ErrTwoFactorRequired, got %v", err)
	}
	err = service.RequestChangeEmail(
		ctx, userID, "totp-tokens-new@example.com", testNewPassword, "000000",
	)
	if !errors.Is(err, auth.ErrInvalidTwoFactorCode) {
		t.Fatalf("expected ErrInvalidTwoFactorCode, got %v", err)
	}

	goodCode, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate change-email code: %v", err)
	}
	sender.reset()
	err = service.RequestChangeEmail(
		ctx, userID, "totp-tokens-new@example.com", testNewPassword, goodCode,
	)
	if err != nil {
		t.Fatalf("change email with 2FA: %v", err)
	}
	if len(sender.bodies) != 1 {
		t.Fatalf("expected change-email message, got %d", len(sender.bodies))
	}
}

func assertChangePasswordTOTP(
	ctx context.Context,
	t *testing.T,
	service *auth.Service,
	userID, secret string,
) {
	t.Helper()

	_, access, refresh, err := service.ChangePassword(
		ctx, userID, testNewPassword, "password-after-2fa-123", "", defaultSessionMeta(),
	)
	if !errors.Is(err, auth.ErrTwoFactorRequired) {
		t.Fatalf("change password without totp: got %v", err)
	}
	if access != "" || refresh != "" {
		t.Fatal("expected no tokens when 2FA is required")
	}

	pwCode, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate password totp: %v", err)
	}
	_, access, refresh, err = service.ChangePassword(
		ctx, userID, testNewPassword, "password-after-2fa-123", pwCode, defaultSessionMeta(),
	)
	if err != nil {
		t.Fatalf("change password with 2FA: %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatal("expected reissued tokens after 2FA password change")
	}
}

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthTokens_ResendConfirmPaths(t *testing.T) {
	allure.Test(
		t,
		"resend confirm covers pending invite then unavailable after accept",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender := newAuthIntegration(ctx, t)
			_ = seedAdminUser(ctx, t, service)

			_, err := service.UpdateSettings(ctx, confirmRequiredSettings())
			if err != nil {
				t.Fatalf("enable confirm policy: %v", err)
			}

			err = service.CreateInvite(ctx, "needs-confirm@example.com", auth.RoleUser, "", 0)
			if err != nil {
				t.Fatalf("create invite: %v", err)
			}
			stub := findUserByEmail(ctx, t, service, "needs-confirm@example.com")
			sender.reset()
			err = service.ResendConfirmEmail(ctx, stub.ID)
			if err != nil {
				t.Fatalf("resend via pending invite: %v", err)
			}
			if len(sender.bodies) != 1 {
				t.Fatalf("expected invite resend email, got %d", len(sender.bodies))
			}

			err = service.AcceptInvite(ctx, sender.lastToken(t), testNewPassword)
			if err != nil {
				t.Fatalf("accept invite: %v", err)
			}
			err = service.ResendConfirmEmail(ctx, stub.ID)
			if !errors.Is(err, auth.ErrResendConfirmUnavailable) {
				t.Fatalf("expected ErrResendConfirmUnavailable after invite accept, got %v", err)
			}
		},
	)
}

func findUserByEmail(
	ctx context.Context,
	t *testing.T,
	service *auth.Service,
	email string,
) auth.AdminUser {
	t.Helper()

	users, err := service.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	for _, user := range users {
		if user.Email == email {
			return user
		}
	}

	t.Fatalf("user %q not found", email)

	return auth.AdminUser{}
}
