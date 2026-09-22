package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/auth/ginjwt"
	"sudoStream/internal/db"
	"sudoStream/internal/email"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/observability"
	"sync"
	"testing"
	"time"

	accesspostgres "sudoStream/internal/access/postgres"
	authpostgres "sudoStream/internal/auth/postgres"
	metadatapostgres "sudoStream/internal/metadata/postgres"

	"github.com/gin-gonic/gin"
)

var testDBMu sync.Mutex

const (
	testAdminEmail      = "admin@hpserver.lan"
	testDefaultPassword = "changeme"
)

func testAdminLoginJSON(password string) []byte {
	return fmt.Appendf(nil, `{"email":%q,"password":%q}`, testAdminEmail, password)
}

func testChangePasswordJSON(currentPassword, newPassword string) []byte {
	return fmt.Appendf(nil,
		`{"currentPassword":%q,"newPassword":%q}`,
		currentPassword,
		newPassword,
	)
}

func testEmailJSON(email string) []byte {
	return fmt.Appendf(nil, `{"email":%q}`, email)
}

func testDefaultSettingsPatchJSON() []byte {
	return []byte(`{
			"emailConfirmationRequired":false,
			"twoFactorRequired":false
		}`)
}

func testEmailConfirmationSettingsPatchJSON() []byte {
	return []byte(`{
			"emailConfirmationRequired":true,
			"twoFactorRequired":false
		}`)
}

func testRouteConfig() RouteConfig {
	return RouteConfig{
		CookieSecure: false,
		MediaRoot:    "",
	}
}

func integrationRouteConfig(root string) RouteConfig {
	return RouteConfig{
		CookieSecure: false,
		MediaRoot:    root,
	}
}

func setupTestCacheEnv(t *testing.T) {
	t.Helper()

	cacheRoot := t.TempDir()
	restore := setTestCacheRoot(cacheRoot)
	t.Cleanup(restore)
}

func requireTestDatabase(t *testing.T) {
	t.Helper()

	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip(
			"SUDOSTREAM_DATABASE_URL not set; run make test-db-up or set SUDOSTREAM_DATABASE_URL for integration tests",
		)
	}
}

// setupTestPool provisions an isolated Postgres schema per test so the shared
// integration database is safe to use concurrently across packages.
func setupTestPool(ctx context.Context, t *testing.T) *db.Database {
	t.Helper()
	requireTestDatabase(t)

	schema := uniqueSchemaName(ctx, t)

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

func uniqueSchemaName(ctx context.Context, t *testing.T) string {
	t.Helper()

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	if err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}

	schema := "httpapi_it_" + hex.EncodeToString(buf)
	provisionSchema(ctx, t, schema)

	return schema
}

// provisionSchema creates the schema and drops it during cleanup using a
// short-lived admin connection on the default search path.
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

func prepareIntegrationAuth(
	ctx context.Context,
	t *testing.T,
	database *db.Database,
) *auth.Service {
	t.Helper()

	return prepareIntegrationAuthWithMail(ctx, t, database, email.LogSender{})
}

func prepareIntegrationAuthWithMail(
	ctx context.Context,
	t *testing.T,
	database *db.Database,
	mail email.Sender,
) *auth.Service {
	t.Helper()

	testDBMu.Lock()
	defer testDBMu.Unlock()

	resetAuthTablesHTTP(ctx, t, database.SQLDB())

	store := authpostgres.NewStore(database.GORM)
	issuer, err := ginjwt.New(store, testAuthConfig())
	if err != nil {
		t.Fatalf("init token issuer: %v", err)
	}

	authService, err := auth.NewService(store, testAuthConfig(), mail, issuer)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	err = authService.SeedAdmin(ctx, testAdminEmail, testDefaultPassword)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	return authService
}

func newTestRouter(t *testing.T, root string) *gin.Engine {
	t.Helper()

	setupTestCacheEnv(t)

	ctx := context.Background()
	database := setupTestPool(ctx, t)
	authService := prepareIntegrationAuth(ctx, t, database)

	placeholder := filepath.Join(root, ".keep")
	err := os.WriteFile(placeholder, []byte("ok"), 0o600)
	if err != nil {
		t.Fatalf("write placeholder: %v", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("new media service: %v", err)
	}

	router := gin.New()
	RegisterRoutes(router, media, authService, nil, nil, nil, integrationRouteConfig(root))

	return router
}

func newACLTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()

	setupTestCacheEnv(t)

	ctx := context.Background()
	database := setupTestPool(ctx, t)
	authService := prepareIntegrationAuth(ctx, t, database)

	root := t.TempDir()
	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("new media service: %v", err)
	}

	accessStore := accesspostgres.NewStore(database.GORM)
	accessService := access.NewService(accessStore)

	router := gin.New()
	router.Use(observability.RequestLogger())
	router.Use(observability.HTTPMetrics())
	RegisterRoutes(
		router,
		media,
		authService,
		accessService,
		nil,
		nil,
		integrationRouteConfig(root),
	)

	return router, root
}

func newMetadataTestRouter(t *testing.T) (*gin.Engine, string, *db.Database) {
	t.Helper()

	setupTestCacheEnv(t)

	ctx := context.Background()
	database := setupTestPool(ctx, t)
	authService := prepareIntegrationAuth(ctx, t, database)

	root := t.TempDir()
	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("new media service: %v", err)
	}

	accessStore := accesspostgres.NewStore(database.GORM)
	accessService := access.NewService(accessStore)
	metadataStore := metadatapostgres.NewStore(database.GORM)
	indexer := metadata.NewIndexer(media, metadataStore)
	metadataService := metadata.NewService(media, accessService, metadataStore, indexer)

	router := gin.New()
	router.Use(observability.RequestLogger())
	router.Use(observability.HTTPMetrics())
	RegisterRoutes(
		router,
		media,
		authService,
		accessService,
		metadataService,
		nil,
		integrationRouteConfig(root),
	)

	return router, root, database
}

func newAuthTestRouter(t *testing.T) *gin.Engine {
	t.Helper()

	setupTestCacheEnv(t)

	ctx := context.Background()
	database := setupTestPool(ctx, t)
	authService := prepareIntegrationAuth(ctx, t, database)

	root := t.TempDir()
	placeholder := filepath.Join(root, ".keep")
	err := os.WriteFile(placeholder, []byte("ok"), 0o600)
	if err != nil {
		t.Fatalf("write placeholder: %v", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("new media service: %v", err)
	}

	router := gin.New()
	RegisterRoutes(router, media, authService, nil, nil, nil, integrationRouteConfig(root))

	return router
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

func resetAuthTablesHTTP(ctx context.Context, t *testing.T, sqlDB *sql.DB) {
	t.Helper()

	queries := []string{
		`DELETE FROM media_metadata`,
		`DELETE FROM library_grants`,
		`DELETE FROM libraries`,
		`DELETE FROM auth_tokens`,
		`DELETE FROM user_totp`,
		`DELETE FROM sessions`,
		`DELETE FROM settings`,
		`DELETE FROM users`,
	}
	for _, query := range queries {
		_, err := sqlDB.ExecContext(ctx, query)
		if err != nil && !missingRelation(err) {
			t.Fatalf("clear auth tables: %v", err)
		}
	}
}

func missingRelation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "does not exist")
}

// captureSender stores the last outbound email for integration tests.
type captureSender struct {
	lastBody    string
	lastSubject string
	lastTo      string
}

func (s *captureSender) Send(_ context.Context, recipient, subject, body string) error {
	s.lastTo = recipient
	s.lastSubject = subject
	s.lastBody = body

	return nil
}

func extractTokenFromEmail(body string) string {
	_, after, ok := strings.Cut(body, "token=")
	if !ok {
		return ""
	}

	rest := after
	for i, ch := range rest {
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '"' {
			return rest[:i]
		}
	}

	return rest
}

func newMediaAtRoot(t *testing.T, root string) (*mediafs.Service, error) {
	t.Helper()

	placeholder := filepath.Join(root, ".keep")
	err := os.WriteFile(placeholder, []byte("ok"), 0o600)
	if err != nil {
		return nil, fmt.Errorf("write placeholder: %w", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		return nil, fmt.Errorf("new media service: %w", err)
	}

	return media, nil
}
