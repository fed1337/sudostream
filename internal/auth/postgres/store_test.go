package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sudoStream/internal/auth"
	"sudoStream/internal/db"
	"testing"
	"time"

	authpostgres "sudoStream/internal/auth/postgres"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

//nolint:paralleltest,gocognit,cyclop,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestStore_AuthPersistence(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"postgres auth store persists users sessions and settings",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := openAuthStoreDatabase(ctx, t)
			store := authpostgres.NewStore(database.GORM)

			err := store.CreateUser(ctx, auth.User{
				Email:              "store-test@example.com",
				Role:               auth.RoleAdmin,
				Enabled:            true,
				MustChangePassword: true,
			}, "hash")
			if err != nil {
				t.Fatalf("create user: %v", err)
			}

			user, passwordHash, err := store.GetUserByEmail(ctx, "store-test@example.com")
			if err != nil {
				t.Fatalf("get user by email: %v", err)
			}
			if passwordHash != "hash" || user.Email != "store-test@example.com" {
				t.Fatalf("unexpected user: %#v hash=%q", user, passwordHash)
			}

			count, err := store.CountUsers(ctx)
			if err != nil || count != 1 {
				t.Fatalf("count users: got %d err=%v", count, err)
			}

			adminCount, err := store.CountAdmins(ctx)
			if err != nil || adminCount != 1 {
				t.Fatalf("count admins: got %d err=%v", adminCount, err)
			}

			sessionID := "00000000-0000-0000-0000-00000000aa01"
			expiresAt := time.Now().UTC().Add(time.Hour)
			err = store.CreateSession(
				ctx,
				sessionID,
				user.ID,
				auth.HashToken("refresh-token"),
				expiresAt,
				auth.SessionMeta{IP: testLoopbackIP, UserAgent: "store-test"},
			)
			if err != nil {
				t.Fatalf("create session: %v", err)
			}

			session, err := store.GetSessionByID(ctx, sessionID)
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if session.UserID != user.ID {
				t.Fatalf("unexpected session user: %q", session.UserID)
			}

			byToken, err := store.GetSessionByTokenHash(ctx, auth.HashToken("refresh-token"))
			if err != nil {
				t.Fatalf("get session by token hash: %v", err)
			}
			if byToken.ID != sessionID {
				t.Fatalf("unexpected session id from token hash: %q", byToken.ID)
			}

			err = store.UpdateSessionRefreshToken(
				ctx,
				sessionID,
				auth.HashToken("rotated-token"),
				expiresAt.Add(time.Hour),
			)
			if err != nil {
				t.Fatalf("update session refresh token: %v", err)
			}

			settings := auth.Settings{
				EmailConfirmationRequired: true,
				TwoFactorRequired:         false,
			}
			err = store.SaveSettings(ctx, settings)
			if err != nil {
				t.Fatalf("save settings: %v", err)
			}

			loaded, err := store.GetSettings(ctx)
			if err != nil {
				t.Fatalf("get settings: %v", err)
			}
			if !loaded.EmailConfirmationRequired || loaded.TwoFactorRequired {
				t.Fatalf("unexpected settings: %+v", loaded)
			}

			err = store.CreateAuthToken(ctx, auth.Token{
				UserID:    user.ID,
				Purpose:   auth.TokenPurposeResetPassword,
				TokenHash: auth.HashToken("reset-token"),
				ExpiresAt: expiresAt,
			})
			if err != nil {
				t.Fatalf("create auth token: %v", err)
			}

			token, err := store.GetAuthTokenByHash(
				ctx,
				auth.HashToken("reset-token"),
				auth.TokenPurposeResetPassword,
			)
			if err != nil {
				t.Fatalf("get auth token: %v", err)
			}
			if token.UserID != user.ID {
				t.Fatalf("unexpected token user: %q", token.UserID)
			}

			err = store.MarkAuthTokenUsed(ctx, token.ID)
			if err != nil {
				t.Fatalf("mark auth token used: %v", err)
			}

			err = store.DeleteSessionByTokenHash(ctx, auth.HashToken("rotated-token"))
			if err != nil {
				t.Fatalf("delete session: %v", err)
			}

			_, err = store.GetSessionByID(ctx, sessionID)
			if err == nil {
				t.Fatal("expected session lookup to fail after delete")
			}
		},
	)
}

func openAuthStoreDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
	schema := uniqueAuthStoreSchema()

	err := db.CreateSchema(ctx, databaseURL, schema)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	//nolint:contextcheck // cleanup runs after the test context is cancelled
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dropErr := db.DropSchema(dropCtx, databaseURL, schema)
		if dropErr != nil {
			t.Errorf("drop schema %s: %v", schema, dropErr)
		}
	})

	opts := db.PoolOptionsFromEnv()
	opts.SearchPath = schema

	database, err := db.Open(ctx, databaseURL, opts)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(database.Close)

	err = db.Migrate(database.SQLDB())
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return database
}

func uniqueAuthStoreSchema() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	return "auth_store_it_" + hex.EncodeToString(buf)
}
