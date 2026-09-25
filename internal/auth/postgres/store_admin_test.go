package postgres_test

import (
	"context"
	"os"
	"sudoStream/internal/auth"
	"testing"
	"time"

	authpostgres "sudoStream/internal/auth/postgres"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

//nolint:paralleltest,cyclop,gocognit,funlen,gocyclo,maintidx // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestStore_UserAdminSessionsAndTOTP(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"postgres auth store covers admin user session and totp flows",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := openAuthStoreDatabase(ctx, t)
			store := authpostgres.NewStore(database.GORM)

			err := store.CreateUser(ctx, auth.User{
				Email:              "admin-flow@example.com",
				Role:               auth.RoleUser,
				Enabled:            false,
				MustChangePassword: true,
			}, "old-hash")
			if err != nil {
				t.Fatalf("create user: %v", err)
			}

			user, _, err := store.GetUserByEmail(ctx, "admin-flow@example.com")
			if err != nil {
				t.Fatalf("get user: %v", err)
			}

			enabled := true
			role := auth.RoleAdmin
			err = store.UpdateUser(ctx, user.ID, auth.UserPatch{
				Enabled: &enabled,
				Role:    &role,
			})
			if err != nil {
				t.Fatalf("update user: %v", err)
			}

			err = store.EnableUser(ctx, user.ID, true)
			if err != nil {
				t.Fatalf("enable user: %v", err)
			}

			loginAt := time.Now().UTC()
			err = store.UpdateLastLogin(ctx, user.ID, loginAt)
			if err != nil {
				t.Fatalf("update last login: %v", err)
			}

			err = store.UpdatePasswordHash(ctx, user.ID, "new-hash", false)
			if err != nil {
				t.Fatalf("update password hash: %v", err)
			}

			loaded, passwordHash, err := store.GetUserWithPasswordByID(ctx, user.ID)
			if err != nil {
				t.Fatalf("get user with password: %v", err)
			}
			if passwordHash != "new-hash" || !loaded.Enabled || loaded.Role != auth.RoleAdmin {
				t.Fatalf("unexpected loaded user: %#v hash=%q", loaded, passwordHash)
			}

			users, err := store.ListUsers(ctx)
			if len(users) != 1 {
				t.Fatalf("list users: got %d err=%v", len(users), err)
			}
			if users[0].Has2FA {
				t.Fatal("expected no 2FA before enrollment")
			}

			sessionA := "00000000-0000-0000-0000-00000000ee01"
			sessionB := "00000000-0000-0000-0000-00000000ee02"
			expiresAt := time.Now().UTC().Add(time.Hour)
			for _, sessionID := range []string{sessionA, sessionB} {
				err = store.CreateSession(
					ctx,
					sessionID,
					user.ID,
					auth.HashToken("token-"+sessionID),
					expiresAt,
					auth.SessionMeta{IP: testLoopbackIP, UserAgent: "admin-flow"},
				)
				if err != nil {
					t.Fatalf("create session %s: %v", sessionID, err)
				}
			}

			sessions, err := store.ListSessionsByUserID(ctx, user.ID)
			if err != nil || len(sessions) != 2 {
				t.Fatalf("list sessions: got %d err=%v", len(sessions), err)
			}

			err = store.DeleteSessionsExcept(ctx, user.ID, sessionA)
			if err != nil {
				t.Fatalf("delete sessions except: %v", err)
			}

			sessions, err = store.ListSessionsByUserID(ctx, user.ID)
			if err != nil || len(sessions) != 1 || sessions[0].ID != sessionA {
				t.Fatalf("unexpected remaining sessions: %#v err=%v", sessions, err)
			}

			err = store.DeleteSessionByID(ctx, sessionA)
			if err != nil {
				t.Fatalf("delete session by id: %v", err)
			}

			err = store.CreateAuthToken(ctx, auth.Token{
				Email:     "invite@example.com",
				Purpose:   auth.TokenPurposeInvite,
				TokenHash: auth.HashToken("invite-token"),
				Role:      auth.RoleUser,
				ExpiresAt: expiresAt,
			})
			if err != nil {
				t.Fatalf("create invite token: %v", err)
			}

			invites, err := store.ListPendingInvites(ctx, "invite@example.com")
			if err != nil || len(invites) != 1 {
				t.Fatalf("list pending invites: got %d err=%v", len(invites), err)
			}

			invite, err := store.GetAuthTokenByID(ctx, invites[0].ID)
			if err != nil || invite.Email != "invite@example.com" {
				t.Fatalf("get auth token by id: %#v err=%v", invite, err)
			}

			err = store.InvalidateUnusedTokens(ctx, "invite@example.com", auth.TokenPurposeInvite)
			if err != nil {
				t.Fatalf("invalidate unused tokens: %v", err)
			}

			hasTOTP, err := store.UserHasTOTP(ctx, user.ID)
			if err != nil || hasTOTP {
				t.Fatalf("expected no totp: has=%v err=%v", hasTOTP, err)
			}

			err = store.SavePendingTOTPSecret(ctx, user.ID, "encrypted-secret")
			if err != nil {
				t.Fatalf("save pending totp secret: %v", err)
			}

			secret, err := store.GetPendingTOTPSecret(ctx, user.ID)
			if err != nil || secret != "encrypted-secret" {
				t.Fatalf("get pending totp secret: %q err=%v", secret, err)
			}

			err = store.EnableUserTOTP(ctx, user.ID, "encrypted-secret", []string{"code1", "code2"})
			if err != nil {
				t.Fatalf("enable user totp: %v", err)
			}

			listed, err := store.ListUsers(ctx)
			if err != nil {
				t.Fatalf("list users after totp: %v", err)
			}
			found := false
			for _, listedUser := range listed {
				if listedUser.ID == user.ID {
					found = true
					if !listedUser.Has2FA {
						t.Fatal("ListUsers Has2FA should be true after EnableUserTOTP")
					}

					break
				}
			}
			if !found {
				t.Fatal("enabled user missing from ListUsers")
			}

			record, err := store.GetUserTOTP(ctx, user.ID)
			if err != nil || record.SecretEncrypted != "encrypted-secret" {
				t.Fatalf("get user totp: %#v err=%v", record, err)
			}

			err = store.ConsumeBackupCode(ctx, user.ID, 0)
			if err != nil {
				t.Fatalf("consume backup code: %v", err)
			}

			err = store.DeleteUserTOTP(ctx, user.ID)
			if err != nil {
				t.Fatalf("delete user totp: %v", err)
			}

			err = store.DeleteAllSessionsByUserID(ctx, user.ID)
			if err != nil {
				t.Fatalf("delete all sessions: %v", err)
			}
		},
	)
}
