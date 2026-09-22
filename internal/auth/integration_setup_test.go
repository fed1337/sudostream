package auth_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"regexp"
	"sudoStream/internal/auth"
	"sudoStream/internal/auth/ginjwt"
	authpostgres "sudoStream/internal/auth/postgres"
	"sudoStream/internal/db"
	"testing"
	"time"
)

const (
	testAdminEmail    = "admin@sudostream.test"
	testAdminPassword = "admin-password-123"
	testNewPassword   = "new-password-456"
)

var tokenPattern = regexp.MustCompile(`token=([^\s]+)`)

type captureMailSender struct {
	bodies []string
}

func (s *captureMailSender) Send(_ context.Context, _, _, body string) error {
	s.bodies = append(s.bodies, body)

	return nil
}

func (s *captureMailSender) reset() {
	s.bodies = nil
}

func (s *captureMailSender) lastToken(t *testing.T) string {
	t.Helper()

	if len(s.bodies) == 0 {
		t.Fatal("no captured emails")
	}

	match := tokenPattern.FindStringSubmatch(s.bodies[len(s.bodies)-1])
	if len(match) != 2 {
		t.Fatalf("no token in email body: %q", s.bodies[len(s.bodies)-1])
	}

	return match[1]
}

func requireAuthDatabase(t *testing.T) {
	t.Helper()

	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}
}

// newAuthIntegration provisions an isolated Postgres schema per test so the
// shared integration database is safe to use concurrently across packages.
func newAuthIntegration(
	ctx context.Context,
	t *testing.T,
) (*auth.Service, *captureMailSender) {
	t.Helper()

	service, sender, _ := newAuthIntegrationStore(ctx, t)

	return service, sender
}

func newAuthIntegrationStore(
	ctx context.Context,
	t *testing.T,
) (*auth.Service, *captureMailSender, *authpostgres.Store) {
	t.Helper()
	requireAuthDatabase(t)

	schema := uniqueSchemaName(t)
	provisionSchema(ctx, t, schema)

	database := newSchemaDatabase(ctx, t, schema)
	store := authpostgres.NewStore(database.GORM)
	sender := &captureMailSender{bodies: nil}
	issuer, err := ginjwt.New(store, testAuthConfig())
	if err != nil {
		t.Fatalf("init token issuer: %v", err)
	}

	service, err := auth.NewService(store, testAuthConfig(), sender, issuer)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	return service, sender, store
}

func uniqueSchemaName(t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}

	return "auth_it_" + hex.EncodeToString(buf)
}

// provisionSchema creates a dedicated schema and drops it during cleanup using a
// short-lived admin connection (search_path stays on the default database).
func provisionSchema(ctx context.Context, t *testing.T, schema string) {
	t.Helper()

	databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
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
}

func newSchemaDatabase(ctx context.Context, t *testing.T, schema string) *db.Database {
	t.Helper()

	opts := db.PoolOptionsFromEnv()
	opts.SearchPath = schema

	database, err := db.Open(ctx, os.Getenv("SUDOSTREAM_DATABASE_URL"), opts)
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

func testAuthConfig() auth.Config {
	return auth.Config{
		TOTPEncryptionKey:     []byte("01234567890123456789012345678901"),
		JWTSecret:             []byte("test-jwt-secret-at-least-32-bytes!!"),
		AccessTTL:             time.Hour,
		RefreshTTL:            24 * time.Hour,
		BaseURL:               "http://localhost:8080",
		InviteTTLHours:        24,
		ConfirmEmailTTLHours:  24,
		ResetPasswordTTLHours: 24,
		TOTPLeewaySeconds:     30,
	}
}

func seedAdminUser(ctx context.Context, t *testing.T, service *auth.Service) *auth.User {
	t.Helper()

	err := service.SeedAdmin(ctx, testAdminEmail, testAdminPassword)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	users, err := service.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}

	for _, listed := range users {
		if listed.Email != testAdminEmail {
			continue
		}

		admin, err := service.GetUserByID(ctx, listed.ID)
		if err != nil {
			t.Fatalf("load seeded admin: %v", err)
		}

		return admin
	}

	t.Fatalf("admin user %s not found", testAdminEmail)

	return nil
}

func defaultSessionMeta() auth.SessionMeta {
	return auth.SessionMeta{IP: "127.0.0.1", UserAgent: "integration-test"}
}

func confirmRequiredSettings() auth.Settings {
	return auth.Settings{
		EmailConfirmationRequired: true,
		TwoFactorRequired:         false,
	}
}
