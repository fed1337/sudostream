package auth_test

import (
	"context"
	"errors"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

//nolint:paralleltest,cyclop,gocognit,funlen // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthChangeEmail_SelfAndAdminConfirmFlows(t *testing.T) {
	allure.Test(
		t,
		"self and admin email change send confirm and reach unconfirmed",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender := newAuthIntegration(ctx, t)
			_ = seedAdminUser(ctx, t, service)

			member, err := service.CreateUser(ctx, auth.CreateUserInput{
				Email:    "member-email@example.com",
				Password: testAdminPassword,
				Role:     auth.RoleUser,
			})
			if err != nil {
				t.Fatalf("create member: %v", err)
			}

			sender.reset()
			err = service.RequestChangeEmail(
				ctx,
				member.ID,
				"member-new@example.com",
				testAdminPassword,
				"",
			)
			if err != nil {
				t.Fatalf("request change email: %v", err)
			}
			if len(sender.bodies) != 1 {
				t.Fatalf("expected one change-email, got %d", len(sender.bodies))
			}

			listed, err := service.ListUsers(ctx)
			if err != nil {
				t.Fatalf("list users: %v", err)
			}
			memberStatus := findAdminUser(t, listed, member.ID)
			if memberStatus.Status != auth.UserStatusUnconfirmed {
				t.Fatalf("expected unconfirmed after self change, got %q", memberStatus.Status)
			}
			if memberStatus.Email != "member-email@example.com" {
				t.Fatalf("email should stay old until confirm, got %q", memberStatus.Email)
			}

			err = service.ResendConfirmEmail(ctx, member.ID)
			if err != nil {
				t.Fatalf("resend change-email: %v", err)
			}
			if len(sender.bodies) != 2 {
				t.Fatalf("expected resend email, got %d bodies", len(sender.bodies))
			}

			token := sender.lastToken(t)
			err = service.ConfirmEmail(ctx, token)
			if err != nil {
				t.Fatalf("confirm change email: %v", err)
			}
			// Second confirm (Strict Mode / double-click) must stay successful.
			err = service.ConfirmEmail(ctx, token)
			if err != nil {
				t.Fatalf("idempotent confirm change email: %v", err)
			}

			reloaded, err := service.GetUserByID(ctx, member.ID)
			if err != nil {
				t.Fatalf("reload member: %v", err)
			}
			if reloaded.Email != "member-new@example.com" {
				t.Fatalf("expected new email, got %q", reloaded.Email)
			}
			if reloaded.EmailVerifiedAt == nil {
				t.Fatal("email should be verified after confirm")
			}

			sender.reset()
			newEmail := "admin-changed@example.com"
			_, err = service.UpdateUser(ctx, member.ID, auth.UserPatch{Email: &newEmail})
			if err != nil {
				t.Fatalf("admin change email: %v", err)
			}
			if len(sender.bodies) != 1 {
				t.Fatalf("expected admin confirm email, got %d", len(sender.bodies))
			}

			listed, err = service.ListUsers(ctx)
			if err != nil {
				t.Fatalf("list after admin change: %v", err)
			}
			memberStatus = findAdminUser(t, listed, member.ID)
			if memberStatus.Status != auth.UserStatusUnconfirmed {
				t.Fatalf("expected unconfirmed after admin change, got %q", memberStatus.Status)
			}
			if memberStatus.Email != newEmail {
				t.Fatalf("admin change should update email immediately, got %q", memberStatus.Email)
			}

			err = service.RequestChangeEmail(
				ctx,
				member.ID,
				"taken@example.com",
				"wrong-password",
				"",
			)
			if !errors.Is(err, auth.ErrInvalidCredentials) {
				t.Fatalf("expected ErrInvalidCredentials, got %v", err)
			}

			dup := "dup-target@example.com"
			_, err = service.CreateUser(ctx, auth.CreateUserInput{
				Email:    dup,
				Password: testAdminPassword,
				Role:     auth.RoleUser,
			})
			if err != nil {
				t.Fatalf("create duplicate target: %v", err)
			}
			err = service.RequestChangeEmail(ctx, member.ID, dup, testAdminPassword, "")
			if !errors.Is(err, auth.ErrUserExists) {
				t.Fatalf("expected ErrUserExists, got %v", err)
			}
		},
	)
}

//nolint:paralleltest,cyclop,gocognit // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthConfirmRequired_LoginAndForgotAutoResend(t *testing.T) {
	allure.Test(
		t,
		"login and forgot-password auto-resend missing confirm tokens",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender, store := newAuthIntegrationStore(ctx, t)
			_ = seedAdminUser(ctx, t, service)

			_, err := service.UpdateSettings(ctx, confirmRequiredSettings())
			if err != nil {
				t.Fatalf("enable confirmation: %v", err)
			}

			member, err := service.CreateUser(ctx, auth.CreateUserInput{
				Email:    "needs-confirm@example.com",
				Password: testAdminPassword,
				Role:     auth.RoleUser,
			})
			if err != nil {
				t.Fatalf("create member: %v", err)
			}

			err = store.ClearEmailVerified(ctx, member.ID)
			if err != nil {
				t.Fatalf("clear verified: %v", err)
			}

			// No outstanding token → login should auto-resend confirm.
			sender.reset()
			_, err = service.Login(ctx, member.Email, testAdminPassword, defaultSessionMeta())
			if !errors.Is(err, auth.ErrUserDisabled) {
				t.Fatalf("expected ErrUserDisabled for unverified login, got %v", err)
			}
			if len(sender.bodies) != 1 {
				t.Fatalf("expected auto-resend confirm, got %d emails", len(sender.bodies))
			}
			confirmTok := sender.lastToken(t)

			// Active token exists → no second email.
			sender.reset()
			_, err = service.Login(ctx, member.Email, testAdminPassword, defaultSessionMeta())
			if !errors.Is(err, auth.ErrUserDisabled) {
				t.Fatalf("expected ErrUserDisabled, got %v", err)
			}
			if len(sender.bodies) != 0 {
				t.Fatalf("should not resend when confirm token exists, got %d", len(sender.bodies))
			}

			_, err = service.Login(ctx, member.Email, "wrong-password", defaultSessionMeta())
			if !errors.Is(err, auth.ErrInvalidCredentials) {
				t.Fatalf("expected invalid credentials, got %v", err)
			}
			if len(sender.bodies) != 0 {
				t.Fatal("wrong password must not auto-resend confirm")
			}

			// Forgot-password with active confirm → no reset/extra mail.
			err = service.RequestPasswordReset(ctx, member.Email)
			if err != nil {
				t.Fatalf("forgot password: %v", err)
			}
			if len(sender.bodies) != 0 {
				t.Fatalf("forgot should not add mail when confirm token exists")
			}

			err = service.ConfirmEmail(ctx, confirmTok)
			if err != nil {
				t.Fatalf("confirm email: %v", err)
			}

			sender.reset()
			err = service.RequestPasswordReset(ctx, member.Email)
			if err != nil {
				t.Fatalf("forgot after verify: %v", err)
			}
			if len(sender.bodies) != 1 {
				t.Fatalf("expected reset email, got %d", len(sender.bodies))
			}

			// Forgot with unverified and no token → auto-resend confirm (not reset).
			err = store.ClearEmailVerified(ctx, member.ID)
			if err != nil {
				t.Fatalf("clear verified again: %v", err)
			}
			sender.reset()
			err = service.RequestPasswordReset(ctx, member.Email)
			if err != nil {
				t.Fatalf("forgot unverified: %v", err)
			}
			if len(sender.bodies) != 1 {
				t.Fatalf("expected confirm auto-resend on forgot, got %d", len(sender.bodies))
			}
		},
	)
}

//nolint:paralleltest // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthDeleteUser_OnlyDisabledAndFreesEmail(t *testing.T) {
	allure.Test(t, "delete disabled users and reject enabled deletes", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		service, _ := newAuthIntegration(ctx, t)
		_ = seedAdminUser(ctx, t, service)

		member, err := service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "delete-me@example.com",
			Password: testAdminPassword,
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("create member: %v", err)
		}

		err = service.DeleteUser(ctx, member.ID)
		if !errors.Is(err, auth.ErrUserNotDisabled) {
			t.Fatalf("expected ErrUserNotDisabled, got %v", err)
		}

		disabled := false
		_, err = service.UpdateUser(ctx, member.ID, auth.UserPatch{Enabled: &disabled})
		if err != nil {
			t.Fatalf("disable member: %v", err)
		}

		err = service.DeleteUser(ctx, member.ID)
		if err != nil {
			t.Fatalf("delete disabled user: %v", err)
		}

		_, err = service.GetUserByID(ctx, member.ID)
		if err == nil {
			t.Fatal("deleted user should be gone")
		}

		_, err = service.CreateUser(ctx, auth.CreateUserInput{
			Email:    "delete-me@example.com",
			Password: testAdminPassword,
			Role:     auth.RoleUser,
		})
		if err != nil {
			t.Fatalf("email should be reusable after delete: %v", err)
		}
	})
}

//nolint:paralleltest,cyclop // integration tests share the SUDOSTREAM_DATABASE_URL fixture
func TestAuthChangePassword_RejectsReuseAndReissuesTokens(t *testing.T) {
	allure.Test(
		t,
		"change password rejects reuse and clears mustChangePassword in new tokens",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			service, sender := newAuthIntegration(ctx, t)
			_ = seedAdminUser(ctx, t, service)

			member, err := service.CreateUser(ctx, auth.CreateUserInput{
				Email:    "mcp@example.com",
				Password: testAdminPassword,
				Role:     auth.RoleUser,
			})
			if err != nil {
				t.Fatalf("create member: %v", err)
			}
			if !member.MustChangePassword {
				t.Fatal("created user should require password change")
			}

			_, _, _, err = service.ChangePassword(
				ctx,
				member.ID,
				testAdminPassword,
				testAdminPassword,
				"",
				defaultSessionMeta(),
			)
			if !errors.Is(err, auth.ErrPasswordUnchanged) {
				t.Fatalf("expected ErrPasswordUnchanged, got %v", err)
			}

			publicUser, access, refresh, err := service.ChangePassword(
				ctx,
				member.ID,
				"",
				testNewPassword,
				"",
				defaultSessionMeta(),
			)
			if err != nil {
				t.Fatalf("change password without current (MCP): %v", err)
			}
			if access == "" || refresh == "" {
				t.Fatal("expected reissued tokens")
			}
			if publicUser.MustChangePassword {
				t.Fatal("mustChangePassword should be false after change")
			}

			_, _, _, err = service.ChangePassword(
				ctx,
				member.ID,
				"",
				"another-after-mcp-cleared",
				"",
				defaultSessionMeta(),
			)
			if !errors.Is(err, auth.ErrInvalidCredentials) {
				t.Fatalf("empty current password without MCP: got %v", err)
			}

			outcome, err := service.Login(
				ctx,
				"mcp@example.com",
				testNewPassword,
				defaultSessionMeta(),
			)
			if err != nil {
				t.Fatalf("login with new password: %v", err)
			}
			if outcome.User.MustChangePassword {
				t.Fatal("login user should not require password change")
			}

			sender.reset()
			err = service.RequestPasswordReset(ctx, "mcp@example.com")
			if err != nil {
				t.Fatalf("request reset: %v", err)
			}
			resetTok := sender.lastToken(t)
			err = service.ResetPassword(ctx, resetTok, testNewPassword)
			if !errors.Is(err, auth.ErrPasswordUnchanged) {
				t.Fatalf("reset same password: expected ErrPasswordUnchanged, got %v", err)
			}

			err = service.ResetPassword(ctx, resetTok, "another-password-789")
			if err != nil {
				t.Fatalf("reset to new password: %v", err)
			}
		},
	)
}

func findAdminUser(t *testing.T, users []auth.AdminUser, userID string) auth.AdminUser {
	t.Helper()

	for _, user := range users {
		if user.ID == userID {
			return user
		}
	}

	t.Fatalf("user %s not found", userID)

	return auth.AdminUser{}
}
