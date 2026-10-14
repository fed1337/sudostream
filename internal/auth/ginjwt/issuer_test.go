package ginjwt_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"sudoStream/internal/auth"
	"sudoStream/internal/auth/ginjwt"
	"sudoStream/internal/db"
	"testing"
	"time"

	authpostgres "sudoStream/internal/auth/postgres"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

//nolint:paralleltest,cyclop,gocognit,funlen // integration tests share SUDOSTREAM_DATABASE_URL fixture
func TestIssuer_IssueRefreshAndAuthenticate(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"issuer issues tokens and optional auth validates access",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)
			ctx := context.Background()
			database := openIssuerTestDatabase(ctx, t)
			store := authpostgres.NewStore(database.GORM)

			err := store.CreateUser(ctx, auth.User{
				Email:              "issuer-test@example.com",
				Role:               auth.RoleUser,
				Enabled:            true,
				MustChangePassword: false,
			}, "hash")
			if err != nil {
				t.Fatalf("create user: %v", err)
			}

			user, _, err := store.GetUserByEmail(ctx, "issuer-test@example.com")
			if err != nil {
				t.Fatalf("get user: %v", err)
			}

			issuer, err := ginjwt.New(store, auth.Config{
				TOTPEncryptionKey: []byte("01234567890123456789012345678901"),
				JWTSecret:         []byte("test-jwt-secret-at-least-32-bytes!!"),
				AccessTTL:         time.Hour,
				RefreshTTL:        24 * time.Hour,
			})
			if err != nil {
				t.Fatalf("new issuer: %v", err)
			}

			sessionID := "00000000-0000-0000-0000-00000000cc01"
			identity := auth.SessionIdentity{
				SessionID: sessionID,
				UserID:    user.ID,
				User:      user.Public(),
				Meta:      auth.SessionMeta{IP: "127.0.0.1", UserAgent: "issuer-test"},
			}

			pair, err := issuer.IssueTokens(identity)
			if err != nil {
				t.Fatalf("issue tokens: %v", err)
			}
			if pair.AccessToken == "" || pair.RefreshToken == "" {
				t.Fatal("expected access and refresh tokens")
			}
			if issuer.AccessTTL() != time.Hour || issuer.RefreshTTL() != 24*time.Hour {
				t.Fatalf(
					"unexpected token TTLs: access=%s refresh=%s",
					issuer.AccessTTL(),
					issuer.RefreshTTL(),
				)
			}

			loadedIdentity, err := issuer.ValidateRefresh(pair.RefreshToken)
			if err != nil {
				t.Fatalf("validate refresh: %v", err)
			}
			if loadedIdentity.SessionID != sessionID || loadedIdentity.UserID != user.ID {
				t.Fatalf("unexpected refresh identity: %+v", loadedIdentity)
			}

			_, err = issuer.ValidateRefresh("")
			if err == nil {
				t.Fatal("expected empty refresh validation to fail")
			}

			middleware := issuer.Middleware()
			protectedRouter := gin.New()
			protectedRouter.GET("/protected", middleware, func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			middlewareRecorder := httptest.NewRecorder()
			middlewareRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/protected",
				nil,
			)
			middlewareRequest.Header.Set("Authorization", "Bearer "+pair.AccessToken)
			protectedRouter.ServeHTTP(middlewareRecorder, middlewareRequest)
			if middlewareRecorder.Code != http.StatusNoContent {
				t.Fatalf(
					"middleware auth: %d %s",
					middlewareRecorder.Code,
					middlewareRecorder.Body.String(),
				)
			}

			recorder := httptest.NewRecorder()
			ginCtx, _ := gin.CreateTestContext(recorder)
			request := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/auth/me",
				nil,
			)
			request.Header.Set("Authorization", "Bearer "+pair.AccessToken)
			ginCtx.Request = request

			authenticatedUser, authenticatedSessionID, ok := issuer.TryAuthenticate(ginCtx)
			if !ok {
				t.Fatalf("try authenticate failed: %d %s", recorder.Code, recorder.Body.String())
			}
			if authenticatedUser.ID != user.ID || authenticatedSessionID != sessionID {
				t.Fatalf(
					"unexpected auth context: user=%+v session=%q",
					authenticatedUser,
					authenticatedSessionID,
				)
			}

			_, refreshedPair, err := issuer.RefreshWithRotation(pair.RefreshToken)
			if err != nil {
				t.Fatalf("refresh tokens: %v", err)
			}
			if refreshedPair.AccessToken == "" || refreshedPair.RefreshToken == "" {
				t.Fatal("expected rotated token pair")
			}

			err = issuer.RevokeRefresh(refreshedPair.RefreshToken)
			if err != nil {
				t.Fatalf("revoke refresh: %v", err)
			}
		},
	)
}

func openIssuerTestDatabase(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()

	databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
	schema := uniqueIssuerSchema()

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

func uniqueIssuerSchema() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	return "ginjwt_it_" + hex.EncodeToString(buf)
}
