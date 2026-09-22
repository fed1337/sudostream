package postgres_test

import (
	"context"
	"errors"
	"os"
	"sudoStream/internal/auth"
	authpostgres "sudoStream/internal/auth/postgres"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/appleboy/gin-jwt/v3/core"
)

//nolint:paralleltest,cyclop // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestRefreshTokenStore_SetGetDelete(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"refresh token store persists opaque tokens in sessions",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			database := openAuthStoreDatabase(ctx, t)
			store := authpostgres.NewStore(database.GORM)
			refreshStore := authpostgres.NewRefreshTokenStore(store)

			err := store.CreateUser(ctx, auth.User{
				Email:              "refresh-store@example.com",
				Role:               auth.RoleUser,
				Enabled:            true,
				MustChangePassword: false,
			}, "hash")
			if err != nil {
				t.Fatalf("create user: %v", err)
			}

			user, _, err := store.GetUserByEmail(ctx, "refresh-store@example.com")
			if err != nil {
				t.Fatalf("get user: %v", err)
			}

			identity := auth.SessionIdentity{
				SessionID: "00000000-0000-0000-0000-00000000dd01",
				UserID:    user.ID,
				User:      user.Public(),
				Meta:      auth.SessionMeta{IP: testLoopbackIP, UserAgent: "refresh-store-test"},
			}
			expiry := time.Now().UTC().Add(time.Hour)

			err = refreshStore.Set(ctx, "opaque-refresh", identity, expiry)
			if err != nil {
				t.Fatalf("set refresh token: %v", err)
			}

			loaded, err := refreshStore.Get(ctx, "opaque-refresh")
			if err != nil {
				t.Fatalf("get refresh token: %v", err)
			}
			loadedIdentity, ok := loaded.(auth.SessionIdentity)
			if !ok || loadedIdentity.SessionID != identity.SessionID {
				t.Fatalf("unexpected refresh identity: %#v", loaded)
			}

			count, err := refreshStore.Count(ctx)
			if err != nil || count != 0 {
				t.Fatalf("count should be zero: got %d err=%v", count, err)
			}

			cleaned, err := refreshStore.Cleanup(ctx)
			if err != nil || cleaned != 0 {
				t.Fatalf("cleanup should be no-op: got %d err=%v", cleaned, err)
			}

			err = refreshStore.Delete(ctx, "opaque-refresh")
			if err != nil {
				t.Fatalf("delete refresh token: %v", err)
			}

			_, err = refreshStore.Get(ctx, "opaque-refresh")
			if err == nil {
				t.Fatal("expected missing refresh token")
			}
			if !errors.Is(err, core.ErrRefreshTokenNotFound) {
				t.Fatalf("expected refresh token not found, got %v", err)
			}
		},
	)
}
