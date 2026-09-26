package auth_test

import (
	"context"
	"errors"
	"strings"
	"sudoStream/internal/auth"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/pquerna/otp/totp"
)

//nolint:paralleltest,cyclop // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthSettings_DefaultsAndUpdate(t *testing.T) {
	allure.Test(
		t,
		"settings return false defaults and persist policy flags",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, _ := newAuthIntegration(ctx, t)

			defaults, err := service.GetSettings(ctx)
			if err != nil {
				t.Fatalf("get settings: %v", err)
			}
			if defaults.EmailConfirmationRequired || defaults.TwoFactorRequired {
				t.Fatalf("unexpected defaults: %+v", defaults)
			}

			err = service.EnsureSettingsDefaults(ctx)
			if err != nil {
				t.Fatalf("seed defaults: %v", err)
			}
			err = service.EnsureSettingsDefaults(ctx)
			if err != nil {
				t.Fatalf("re-seed defaults should be a no-op: %v", err)
			}

			updated, err := service.UpdateSettings(ctx, auth.Settings{
				EmailConfirmationRequired: true,
				TwoFactorRequired:         true,
			})
			if err != nil {
				t.Fatalf("update settings: %v", err)
			}
			if !updated.EmailConfirmationRequired || !updated.TwoFactorRequired {
				t.Fatalf("unexpected update result: %+v", updated)
			}

			reloaded, err := service.GetSettings(ctx)
			if err != nil {
				t.Fatalf("reload settings: %v", err)
			}
			if !reloaded.EmailConfirmationRequired || !reloaded.TwoFactorRequired {
				t.Fatalf("settings did not persist: %+v", reloaded)
			}
		},
	)
}

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthLogin_PendingInviteRejectsWithoutPasswordCheck(t *testing.T) {
	allure.Test(
		t,
		"pending invite login returns invite pending and resends when expired",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender := newAuthIntegration(ctx, t)

			err := service.CreateInvite(ctx, "pending@example.com", auth.RoleUser, "", time.Hour)
			if err != nil {
				t.Fatalf("create invite: %v", err)
			}
			before := len(sender.bodies)

			_, err = service.Login(ctx, "pending@example.com", "any-password", defaultSessionMeta())
			if !errors.Is(err, auth.ErrInvitePending) {
				t.Fatalf("expected ErrInvitePending, got %v", err)
			}
			if len(sender.bodies) != before {
				t.Fatal("unexpired invite should not resend on login")
			}

			err = service.CreateInvite(
				ctx,
				"expired@example.com",
				auth.RoleUser,
				"",
				time.Millisecond,
			)
			if err != nil {
				t.Fatalf("create short invite: %v", err)
			}
			time.Sleep(5 * time.Millisecond)
			sender.reset()

			_, err = service.Login(ctx, "expired@example.com", "any-password", defaultSessionMeta())
			if !errors.Is(err, auth.ErrInvitePending) {
				t.Fatalf("expected ErrInvitePending after expiry, got %v", err)
			}
			if len(sender.bodies) != 1 {
				t.Fatalf("expected invite resend email, got %d", len(sender.bodies))
			}

			acceptToken := sender.lastToken(t)
			err = service.AcceptInvite(ctx, acceptToken, testNewPassword)
			if err != nil {
				t.Fatalf("accept resent invite: %v", err)
			}
			outcome, err := service.Login(
				ctx,
				"expired@example.com",
				testNewPassword,
				defaultSessionMeta(),
			)
			if err != nil || outcome.Status != "ok" {
				t.Fatalf("login after accept failed: status=%q err=%v", outcome.Status, err)
			}
		},
	)
}

//nolint:paralleltest,cyclop // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthInvites_Lifecycle(t *testing.T) {
	allure.Test(t, "invites create, list, resend, accept, and revoke", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		service, sender := newAuthIntegration(ctx, t)

		err := service.CreateInvite(ctx, "   ", "", "", 0)
		if !errors.Is(err, auth.ErrMissingEmail) {
			t.Fatalf("expected ErrMissingEmail, got %v", err)
		}

		err = service.CreateInvite(ctx, "invitee@example.com", auth.RoleUser, "", 0)
		if err != nil {
			t.Fatalf("create invite: %v", err)
		}

		invites, err := service.ListPendingInvites(ctx, "")
		if err != nil || len(invites) != 1 {
			t.Fatalf("expected 1 pending invite, got %d (err=%v)", len(invites), err)
		}

		sender.reset()
		err = service.ResendInvite(ctx, invites[0].ID, "", 0)
		if err != nil {
			t.Fatalf("resend invite: %v", err)
		}

		acceptToken := sender.lastToken(t)
		err = service.AcceptInvite(ctx, acceptToken, testNewPassword)
		if err != nil {
			t.Fatalf("accept invite: %v", err)
		}

		outcome, err := service.Login(
			ctx,
			"invitee@example.com",
			testNewPassword,
			defaultSessionMeta(),
		)
		if err != nil || outcome.Status != "ok" {
			t.Fatalf("invited user login failed: status=%q err=%v", outcome.Status, err)
		}

		err = service.CreateInvite(ctx, "invitee@example.com", "", "", 0)
		if !errors.Is(err, auth.ErrUserExists) {
			t.Fatalf("expected ErrUserExists for accepted email, got %v", err)
		}

		err = service.CreateInvite(ctx, "second@example.com", "", "", 0)
		if err != nil {
			t.Fatalf("create second invite: %v", err)
		}
		second, err := service.ListPendingInvites(ctx, "second@example.com")
		if err != nil || len(second) != 1 {
			t.Fatalf("expected second invite, got %d (err=%v)", len(second), err)
		}

		err = service.RevokeInvite(ctx, second[0].ID)
		if err != nil {
			t.Fatalf("revoke invite: %v", err)
		}
		remaining, err := service.ListPendingInvites(ctx, "second@example.com")
		if err != nil || len(remaining) != 0 {
			t.Fatalf("invite should be revoked, got %d (err=%v)", len(remaining), err)
		}
	})
}

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthPasswordReset_ResetsAndRejectsInvalidToken(t *testing.T) {
	allure.Test(
		t,
		"password reset emails known users only and rotates the password",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender := newAuthIntegration(ctx, t)

			_, err := service.CreateUser(ctx, auth.CreateUserInput{
				Email:    "reset@example.com",
				Password: testAdminPassword,
				Role:     auth.RoleUser,
			})
			if err != nil {
				t.Fatalf("create user: %v", err)
			}

			before := len(sender.bodies)
			err = service.RequestPasswordReset(ctx, "nobody@example.com")
			if err != nil {
				t.Fatalf("reset for unknown email should not error: %v", err)
			}
			if len(sender.bodies) != before {
				t.Fatal("unknown email should not trigger a reset email")
			}

			err = service.RequestPasswordReset(ctx, "reset@example.com")
			if err != nil {
				t.Fatalf("request reset: %v", err)
			}
			if len(sender.bodies) == 0 {
				t.Fatal("expected reset email body")
			}
			lastBody := sender.bodies[len(sender.bodies)-1]
			if !strings.Contains(lastBody, "http://localhost:8080/reset-password?token=") {
				t.Fatalf("reset link must use configured BaseURL, not request Host: %q", lastBody)
			}
			token := sender.lastToken(t)

			err = service.ResetPassword(ctx, token, testNewPassword)
			if err != nil {
				t.Fatalf("reset password: %v", err)
			}

			outcome, err := service.Login(
				ctx,
				"reset@example.com",
				testNewPassword,
				defaultSessionMeta(),
			)
			if err != nil || outcome.Status != "ok" {
				t.Fatalf("login with new password failed: status=%q err=%v", outcome.Status, err)
			}

			err = service.ResetPassword(ctx, "bogus-token", testNewPassword)
			if !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}
		},
	)
}

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthAcceptInvite_EnablesEvenWhenConfirmationRequired(t *testing.T) {
	allure.Test(
		t,
		"accept invite enables login even when email confirmation policy is on",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender := newAuthIntegration(ctx, t)

			_, err := service.UpdateSettings(ctx, confirmRequiredSettings())
			if err != nil {
				t.Fatalf("enable confirmation policy: %v", err)
			}

			err = service.CreateInvite(ctx, "confirm@example.com", auth.RoleUser, "", 0)
			if err != nil {
				t.Fatalf("create invite: %v", err)
			}

			inviteToken := sender.lastToken(t)
			err = service.AcceptInvite(ctx, inviteToken, testNewPassword)
			if err != nil {
				t.Fatalf("accept invite: %v", err)
			}

			reloaded := findUserByEmail(ctx, t, service, "confirm@example.com")
			if !reloaded.Enabled {
				t.Fatal("invited account should be enabled after accept")
			}
			if !reloaded.EmailVerified {
				t.Fatal("invite accept should mark email verified")
			}
			if reloaded.Status != auth.UserStatusActive {
				t.Fatalf("unexpected status %q", reloaded.Status)
			}

			outcome, err := service.Login(
				ctx,
				"confirm@example.com",
				testNewPassword,
				defaultSessionMeta(),
			)
			if err != nil || outcome.Status != "ok" {
				t.Fatalf("invited user login failed: status=%q err=%v", outcome.Status, err)
			}
		},
	)
}

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthAdminUserManagement_UpdatesAndGuardsLastAdmin(t *testing.T) {
	allure.Test(t, "admin user management enforces the last-admin guard", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		service, _ := newAuthIntegration(ctx, t)
		admin := seedAdminUser(ctx, t, service)

		_, err := service.CreateUser(ctx, auth.CreateUserInput{Email: "", Password: ""})
		if !errors.Is(err, auth.ErrMissingCredentials) {
			t.Fatalf("expected ErrMissingCredentials, got %v", err)
		}

		member, err := service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "member@example.com",
			Password: testAdminPassword,
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("create member: %v", err)
		}

		_, err = service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "member@example.com",
			Password: testAdminPassword,
		})
		if !errors.Is(err, auth.ErrUserExists) {
			t.Fatalf("expected ErrUserExists, got %v", err)
		}

		disabled := false
		_, err = service.UpdateUser(ctx, admin.ID, auth.UserPatch{Enabled: &disabled})
		if !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("disabling the only admin should be forbidden, got %v", err)
		}

		adminRole := auth.RoleAdmin
		_, err = service.UpdateUser(ctx, member.ID, auth.UserPatch{Role: &adminRole})
		if err != nil {
			t.Fatalf("promote member: %v", err)
		}

		updated, err := service.UpdateUser(ctx, admin.ID, auth.UserPatch{Enabled: &disabled})
		if err != nil {
			t.Fatalf("disable admin with a second admin present: %v", err)
		}
		if updated.Enabled {
			t.Fatal("admin should now be disabled")
		}

		users, err := service.ListUsers(ctx)
		if err != nil || len(users) != 2 {
			t.Fatalf("expected 2 users, got %d (err=%v)", len(users), err)
		}
	})
}

//nolint:paralleltest,cyclop // integration test drives the full 2FA lifecycle
func TestAuthTwoFactor_EnrollLoginDisable(t *testing.T) {
	allure.Test(
		t,
		"two-factor enrollment, login verification, and disable",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, _ := newAuthIntegration(ctx, t)
			admin := seedAdminUser(ctx, t, service)

			setup, err := service.StartTwoFactorSetup(ctx, admin.ID, admin.Email)
			if err != nil {
				t.Fatalf("start 2FA setup: %v", err)
			}

			_, err = service.ConfirmTwoFactorSetup(ctx, admin.ID, "000000")
			if !errors.Is(err, auth.ErrInvalidTwoFactorCode) {
				t.Fatalf("expected ErrInvalidTwoFactorCode, got %v", err)
			}

			code, err := totp.GenerateCode(setup.Secret, time.Now())
			if err != nil {
				t.Fatalf("generate totp code: %v", err)
			}
			result, err := service.ConfirmTwoFactorSetup(ctx, admin.ID, code)
			if err != nil {
				t.Fatalf("confirm 2FA setup: %v", err)
			}
			if len(result.BackupCodes) != 8 {
				t.Fatalf("expected 8 backup codes, got %d", len(result.BackupCodes))
			}

			_, err = service.StartTwoFactorSetup(ctx, admin.ID, admin.Email)
			if !errors.Is(err, auth.ErrTwoFactorAlreadyEnabled) {
				t.Fatalf("expected ErrTwoFactorAlreadyEnabled, got %v", err)
			}

			outcome, err := service.Login(ctx, admin.Email, testAdminPassword, defaultSessionMeta())
			if err != nil {
				t.Fatalf("login: %v", err)
			}
			if outcome.Status != "2fa_required" || outcome.PendingToken == "" {
				t.Fatalf("expected 2fa_required with pending token, got %+v", outcome)
			}

			loginCode, err := totp.GenerateCode(setup.Secret, time.Now())
			if err != nil {
				t.Fatalf("generate login totp code: %v", err)
			}
			_, access, refresh, err := service.VerifyLoginTwoFactor(
				ctx,
				outcome.PendingToken,
				loginCode,
				defaultSessionMeta(),
			)
			if err != nil {
				t.Fatalf("verify login 2FA: %v", err)
			}
			if access == "" || refresh == "" {
				t.Fatal("expected access and refresh tokens after 2FA login")
			}

			err = service.DisableTwoFactor(ctx, admin.ID, testAdminPassword, result.BackupCodes[0])
			if err != nil {
				t.Fatalf("disable 2FA with backup code: %v", err)
			}

			outcome, err = service.Login(ctx, admin.Email, testAdminPassword, defaultSessionMeta())
			if err != nil || outcome.Status != "ok" {
				t.Fatalf("login after disabling 2FA: status=%q err=%v", outcome.Status, err)
			}
		},
	)
}

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthTwoFactor_DisableErrors(t *testing.T) {
	allure.Test(
		t,
		"disable 2FA and pending-token lookups reject bad input",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, _ := newAuthIntegration(ctx, t)
			admin := seedAdminUser(ctx, t, service)

			err := service.DisableTwoFactor(ctx, admin.ID, testAdminPassword, "000000")
			if !errors.Is(err, auth.ErrTwoFactorNotConfigured) {
				t.Fatalf("expected ErrTwoFactorNotConfigured, got %v", err)
			}

			err = service.DisableTwoFactor(ctx, admin.ID, "wrong-password", "000000")
			if !errors.Is(err, auth.ErrInvalidCredentials) {
				t.Fatalf("expected ErrInvalidCredentials, got %v", err)
			}

			setup, err := service.StartTwoFactorSetup(ctx, admin.ID, admin.Email)
			if err != nil {
				t.Fatalf("start 2FA setup: %v", err)
			}
			code, err := totp.GenerateCode(setup.Secret, time.Now())
			if err != nil {
				t.Fatalf("generate totp code: %v", err)
			}
			_, err = service.ConfirmTwoFactorSetup(ctx, admin.ID, code)
			if err != nil {
				t.Fatalf("confirm 2FA setup: %v", err)
			}

			err = service.DisableTwoFactor(ctx, admin.ID, testAdminPassword, "000000")
			if !errors.Is(err, auth.ErrInvalidTwoFactorCode) {
				t.Fatalf("expected ErrInvalidTwoFactorCode, got %v", err)
			}

			_, err = service.UserFromPendingLoginToken(ctx, "not-a-real-token")
			if !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}
		},
	)
}

//nolint:paralleltest,cyclop // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthSessions_UserAndAdminRevoke(t *testing.T) {
	allure.Test(t, "users and admins can list and revoke sessions", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		service, _ := newAuthIntegration(ctx, t)
		admin := seedAdminUser(ctx, t, service)

		outcome, err := service.Login(ctx, testAdminEmail, testAdminPassword, defaultSessionMeta())
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		if outcome.AccessToken == "" {
			t.Fatal("expected access token")
		}

		adminSessions, err := service.AdminListUserSessions(ctx, admin.ID)
		if err != nil {
			t.Fatalf("admin list sessions: %v", err)
		}
		if len(adminSessions) == 0 {
			t.Fatal("expected at least one active session")
		}

		userSessions, err := service.ListUserSessions(ctx, admin.ID, adminSessions[0].ID)
		if err != nil {
			t.Fatalf("list user sessions: %v", err)
		}
		if len(userSessions) == 0 || !userSessions[0].Current {
			t.Fatalf("expected current session flagged: %+v", userSessions)
		}

		otherUser, err := service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "sessions@example.com",
			Password: "sessions-pass",
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}

		_, err = service.RevokeUserSession(ctx, otherUser.ID, adminSessions[0].ID, "")
		if !errors.Is(err, auth.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}

		isCurrent, err := service.RevokeUserSession(
			ctx,
			admin.ID,
			adminSessions[0].ID,
			adminSessions[0].ID,
		)
		if err != nil {
			t.Fatalf("revoke user session: %v", err)
		}
		if !isCurrent {
			t.Fatal("expected revoked session to be current")
		}

		_, err = service.Login(ctx, testAdminEmail, testAdminPassword, defaultSessionMeta())
		if err != nil {
			t.Fatalf("second login: %v", err)
		}

		err = service.AdminRevokeAllSessions(ctx, admin.ID)
		if err != nil {
			t.Fatalf("admin revoke all: %v", err)
		}

		remaining, err := service.AdminListUserSessions(ctx, admin.ID)
		if err != nil {
			t.Fatalf("admin list after revoke all: %v", err)
		}
		if len(remaining) != 0 {
			t.Fatalf("expected no sessions after revoke all, got %d", len(remaining))
		}
	})
}
