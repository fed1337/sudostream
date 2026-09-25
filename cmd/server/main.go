// Package main starts the sudoStream HTTP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/auth/ginjwt"
	"sudoStream/internal/catalog"
	"sudoStream/internal/db"
	"sudoStream/internal/dlna"
	"sudoStream/internal/email"
	"sudoStream/internal/favorite"
	"sudoStream/internal/frontend"
	"sudoStream/internal/homeshelf"
	"sudoStream/internal/httpapi"
	"sudoStream/internal/maintenance"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/network"
	"sudoStream/internal/observability"
	"sudoStream/internal/provider"
	"sudoStream/internal/provider/anidb"
	"sudoStream/internal/provider/fanart"
	"sudoStream/internal/provider/tmdb"
	"sudoStream/internal/provider/tvdb"
	"sudoStream/internal/provider/tvmaze"
	"sudoStream/internal/transcode"
	"sudoStream/internal/trash"
	"sudoStream/internal/version"
	"sudoStream/internal/watch"
	"time"

	accesspostgres "sudoStream/internal/access/postgres"
	authpostgres "sudoStream/internal/auth/postgres"
	favoritepostgres "sudoStream/internal/favorite/postgres"
	maintenancepostgres "sudoStream/internal/maintenance/postgres"
	metadatapostgres "sudoStream/internal/metadata/postgres"
	anilist "sudoStream/internal/provider/anilist"
	opensubtitles "sudoStream/internal/provider/opensubtitles"
	providerpostgres "sudoStream/internal/provider/postgres"
	trashpostgres "sudoStream/internal/trash/postgres"
	watchpostgres "sudoStream/internal/watch/postgres"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultJWTAccessTTL   = 15 * time.Minute
	defaultMediaRoot      = "/media"
	defaultListenAddr     = ":8080"
	defaultAppBaseURL     = "http://localhost:8080"
	defaultTokenTTLHours  = 24
	defaultTOTPLeewaySecs = 30
)

var errDatabaseURLRequired = errors.New("SUDOSTREAM_DATABASE_URL is required when auth is enabled")

type authBundle struct {
	auth              *auth.Service
	authStore         *authpostgres.Store
	access            *access.Service
	metadata          *metadata.Service
	watch             *watch.Service
	favorite          *favorite.Service
	homeShelf         *homeshelf.Service
	maintenance       *maintenance.Service
	providers         *provider.Service
	providerArtifacts provider.ArtifactStore
	providerCache     *provider.Cache
	networkSettings   *network.KVSettingsStore
	networkLive       *network.Live
	trash             *trash.Service
	dlnaSettings      *dlna.KVSettingsStore
	dlnaController    *dlna.Controller
	database          *db.Database
}

//	@title					sudoStream API
//	@version				0.1.0
//	@description			HTTP API for browsing and streaming media files.
//	@servers.url			http://localhost:8080
//	@servers.description	Local development server
//go:generate go run github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc5 init --v3.1 --parseDependency --parseInternal --outputTypes json,yaml --output ./openapi -g cmd/server/main.go

func main() { //nolint:funlen // composition root wiring
	exitIfVersionRequested()

	mediaRoot := defaultMediaRoot

	fsService, err := mediafs.New(mediaRoot)
	if err != nil {
		log.Fatalf("init media filesystem service: %v", err)
	}

	authBundle, err := initAuthServices(mediaRoot, fsService)
	if err != nil {
		log.Fatalf("init auth service: %v", err)
	}

	catalogService := catalog.NewService(authBundle.metadata, authBundle.access, authBundle.metadata)
	authBundle.dlnaSettings = dlna.NewKVSettingsStore(authBundle.authStore)
	authBundle.dlnaController = dlna.NewController(dlna.Deps{
		Settings:      authBundle.dlnaSettings,
		Access:        authBundle.access,
		Media:         fsService,
		Catalog:       catalogService,
		Auth:          authBundle.auth,
		Probe:         transcode.ProbeSource,
		SignKey:       loadJWTSecret(),
		Port:          envInt("SUDOSTREAM_DLNA_HTTP_PORT", dlna.DefaultHTTPPort()),
		BindHost:      strings.TrimSpace(os.Getenv("SUDOSTREAM_DLNA_HTTP_BIND")),
		SSDPMulticast: envOrDefault("SUDOSTREAM_DLNA_SSDP_MULTICAST", dlna.DefaultSSDPMulticast()),
		AdvertiseHost: strings.TrimSpace(os.Getenv("SUDOSTREAM_DLNA_ADVERTISE_HOST")),
	})

	closeLog, logErr := observability.ConfigureFileLogging()
	if logErr != nil {
		log.Printf("warning: file logging disabled: %v", logErr)
	}
	_ = closeLog // keep log file open for process lifetime; host rotates the volume

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(observability.RequestLogger())
	router.Use(observability.HTTPMetrics())
	router.GET("/metrics", observability.MetricsHandler())
	router.HandleMethodNotAllowed = true

	workers := httpapi.RegisterRoutes(
		router,
		fsService,
		authBundle.auth,
		authBundle.access,
		authBundle.metadata,
		authBundle.watch,
		httpapi.RouteConfig{
			CookieSecure:      envBool("SUDOSTREAM_COOKIE_SECURE", false),
			MediaRoot:         mediaRoot,
			DBPing:            authBundle.database.Ping,
			TranscodeSettings: transcode.NewKVSettingsStore(authBundle.authStore),
			NetworkSettings:   authBundle.networkSettings,
			NetworkLive:       authBundle.networkLive,
			Maintenance:       authBundle.maintenance,
			Providers:         authBundle.providers,
			ProviderArtifacts: authBundle.providerArtifacts,
			ProviderCache:     authBundle.providerCache,
			Favorite:          authBundle.favorite,
			HomeShelf:         authBundle.homeShelf,
			Trash:             authBundle.trash,
			DLNASettings:      authBundle.dlnaSettings,
			DLNAController:    authBundle.dlnaController,
		},
	)

	if authBundle.dlnaController != nil {
		err = authBundle.dlnaController.Reload(context.Background())
		if err != nil {
			log.Printf("warning: dlna start failed: %v", err)
		}
	}

	if authBundle.maintenance != nil {
		err = authBundle.maintenance.Start(context.Background())
		if err != nil {
			log.Printf("warning: maintenance scheduler start failed: %v", err)
		}
	}

	observability.RegisterDBSQL(authBundle.database.SQLDB())

	if ui := frontend.Assets(); ui != nil {
		httpapi.RegisterFrontend(router, ui)
		log.Print("serving embedded frontend from web/dist")
	}

	srv := &http.Server{
		Addr:              defaultListenAddr,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	log.Printf("starting sudoStream v%s (media root: %s)", version.Version, mediaRoot)

	err = listenUntilSignal(srv)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	shutdownServer(srv)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownBackground(
		shutdownCtx,
		workers,
		authBundle.metadata,
		authBundle.maintenance,
		authBundle.database,
		authBundle.dlnaController,
	)
}

func initAuthServices( //nolint:funlen // composition root: DB + services wiring
	mediaRoot string,
	fsService *mediafs.Service,
) (*authBundle, error) {
	databaseURL := os.Getenv("SUDOSTREAM_DATABASE_URL")
	if databaseURL == "" {
		return nil, errDatabaseURLRequired
	}

	pool, err := db.Open(context.Background(), databaseURL, db.PoolOptionsFromEnv())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	err = db.Migrate(pool.SQLDB())
	if err != nil {
		pool.Close()

		return nil, fmt.Errorf("run migrations: %w", err)
	}

	authStore := authpostgres.NewStore(pool.GORM)
	accessStore := accesspostgres.NewStore(pool.GORM)
	metadataStore := metadatapostgres.NewStore(pool.GORM)
	watchStore := watchpostgres.NewStore(pool.GORM)
	favoriteStore := favoritepostgres.NewStore(pool.GORM)
	mail := email.NewSenderFromEnv(
		os.Getenv("SUDOSTREAM_SMTP_HOST"),
		envOrDefault("SUDOSTREAM_SMTP_PORT", "587"),
		os.Getenv("SUDOSTREAM_SMTP_USER"),
		os.Getenv("SUDOSTREAM_SMTP_PASSWORD"),
		envOrDefault("SUDOSTREAM_SMTP_FROM", "sudostream@localhost"),
		os.Getenv("SUDOSTREAM_SMTP_FROM_NAME"),
	)

	authService, err := newAuthService(authStore, mail)
	if err != nil {
		pool.Close()

		return nil, err
	}

	err = bootstrapAuthAdmin(context.Background(), authService)
	if err != nil {
		pool.Close()

		return nil, err
	}

	accessService := access.NewService(accessStore)
	indexer := metadata.NewIndexer(fsService, metadataStore)
	trashStore := trashpostgres.NewStore(pool.GORM)
	trashService := trash.NewService(fsService, trashStore, accessService, authStore)
	indexer.SetFrozenPaths(trashService)
	metadataService := metadata.NewService(fsService, accessService, metadataStore, indexer)
	watchService := watch.NewService(fsService, accessService, watchStore)
	favoriteService := favorite.NewService(fsService, accessService, favoriteStore)
	homeShelfService := homeshelf.NewService(
		accessService,
		favoriteStore,
		watchStore,
		metadataStore,
	)
	providerService, providerArtifacts, providerCache, providerEnricher, networkSettings, networkLive := wireProviderStack(
		fsService,
		accessService,
		metadataService,
		pool.GORM,
		authStore,
	)
	maintenanceService := maintenance.NewService(
		maintenancepostgres.NewStore(pool.GORM),
		maintenance.Deps{
			Access:    accessService,
			MediaRoot: mediaRoot,
			Indexer:   indexer,
			Trash:     trashService,
			Providers: providerEnricher,
		},
	)

	_, err = accessService.PruneMissingRoots(context.Background(), mediaRoot)
	if err != nil {
		log.Printf("warning: initial library sync failed: %v", err)
	}

	return newAuthBundle(
		authService,
		authStore,
		accessService,
		metadataService,
		watchService,
		favoriteService,
		homeShelfService,
		maintenanceService,
		providerService,
		providerArtifacts,
		providerCache,
		networkSettings,
		networkLive,
		trashService,
		pool,
	), nil
}

func newAuthBundle(
	authService *auth.Service,
	authStore *authpostgres.Store,
	accessService *access.Service,
	metadataService *metadata.Service,
	watchService *watch.Service,
	favoriteService *favorite.Service,
	homeShelfService *homeshelf.Service,
	maintenanceService *maintenance.Service,
	providerService *provider.Service,
	providerArtifacts provider.ArtifactStore,
	providerCache *provider.Cache,
	networkSettings *network.KVSettingsStore,
	networkLive *network.Live,
	trashService *trash.Service,
	pool *db.Database,
) *authBundle {
	return &authBundle{
		auth:              authService,
		authStore:         authStore,
		access:            accessService,
		metadata:          metadataService,
		watch:             watchService,
		favorite:          favoriteService,
		homeShelf:         homeShelfService,
		maintenance:       maintenanceService,
		providers:         providerService,
		providerArtifacts: providerArtifacts,
		providerCache:     providerCache,
		networkSettings:   networkSettings,
		networkLive:       networkLive,
		trash:             trashService,
		database:          pool,
	}
}

func wireProviderStack(
	fsService *mediafs.Service,
	accessService *access.Service,
	metadataService *metadata.Service,
	gormDB *gorm.DB,
	settingsKV network.RawSettingsKV,
) (
	*provider.Service,
	*providerpostgres.ArtifactStore,
	*provider.Cache,
	*provider.Enricher,
	*network.KVSettingsStore,
	*network.Live,
) {
	networkSettings := network.NewKVSettingsStore(settingsKV)
	initial, err := networkSettings.GetSettings(context.Background())
	if err != nil {
		log.Printf("warning: load network settings failed: %v", err)
		initial = network.DefaultSettings()
	}
	networkLive := network.NewLive(initial)
	providerHTTP := networkLive.HTTPClient()

	providerRegistry := provider.NewRegistry()
	anilistAdapter := anilist.New(anilist.WithHTTPClient(providerHTTP))
	providerRegistry.RegisterMetadata(anilistAdapter)
	providerRegistry.RegisterPoster(anilistAdapter)
	tvmazeAdapter := tvmaze.New(tvmaze.WithHTTPClient(providerHTTP))
	providerRegistry.RegisterMetadata(tvmazeAdapter)
	providerRegistry.RegisterPoster(tvmazeAdapter)
	tmdbAdapter := tmdb.New(tmdb.WithHTTPClient(providerHTTP))
	providerRegistry.RegisterMetadata(tmdbAdapter)
	providerRegistry.RegisterPoster(tmdbAdapter)
	tvdbAdapter := tvdb.New(tvdb.WithHTTPClient(providerHTTP))
	providerRegistry.RegisterMetadata(tvdbAdapter)
	providerRegistry.RegisterPoster(tvdbAdapter)
	providerRegistry.RegisterPoster(fanart.New(fanart.WithHTTPClient(providerHTTP)))
	anidbAdapter := anidb.New(anidb.WithHTTPClient(providerHTTP), anidb.WithUDPDialer(networkLive))
	providerRegistry.RegisterMetadata(anidbAdapter)
	providerRegistry.RegisterPoster(anidbAdapter)
	providerRegistry.RegisterSubtitle(opensubtitles.New(opensubtitles.WithHTTPClient(providerHTTP)))
	providerService := provider.NewService(providerpostgres.NewSettingsStore(gormDB), providerRegistry)
	providerArtifacts := providerpostgres.NewArtifactStore(gormDB)
	providerCache, err := provider.NewCache(provider.DefaultCacheRoot)
	if err != nil {
		log.Printf("warning: provider cache disabled: %v", err)
	}
	providerEnricher := provider.NewEnricher(provider.EnricherDeps{
		Libraries: accessService,
		Settings:  providerService,
		Registry:  providerRegistry,
		Metadata:  metadataService,
		Artifacts: providerArtifacts,
		Cache:     providerCache,
		ListVideos: func(roots []string) ([]string, error) {
			return metadata.ListVideoPaths(fsService, roots)
		},
		ResolvePath:            fsService.FilePath,
		LocalSubtitleLanguages: transcode.LocalSubtitleLanguages,
	})

	return providerService, providerArtifacts, providerCache, providerEnricher, networkSettings, networkLive
}

func newAuthService(authStore *authpostgres.Store, mail email.Sender) (*auth.Service, error) {
	authCfg := auth.Config{
		TOTPEncryptionKey:     loadTOTPEncryptionKey(),
		JWTSecret:             loadJWTSecret(),
		AccessTTL:             envDuration("SUDOSTREAM_JWT_ACCESS_TTL", defaultJWTAccessTTL),
		RefreshTTL:            envDuration("SUDOSTREAM_JWT_REFRESH_TTL", 7*24*time.Hour),
		BaseURL:               envOrDefault("SUDOSTREAM_BASE_URL", defaultAppBaseURL),
		InviteTTLHours:        envInt("SUDOSTREAM_INVITE_TTL_HOURS", defaultTokenTTLHours),
		ConfirmEmailTTLHours:  envInt("SUDOSTREAM_CONFIRM_EMAIL_TTL_HOURS", defaultTokenTTLHours),
		ResetPasswordTTLHours: envInt("SUDOSTREAM_RESET_PASSWORD_TTL_HOURS", defaultTokenTTLHours),
		TOTPLeewaySeconds:     envInt("SUDOSTREAM_TOTP_LEEWAY_SECONDS", defaultTOTPLeewaySecs),
	}

	issuer, err := ginjwt.New(authStore, authCfg)
	if err != nil {
		return nil, fmt.Errorf("init token issuer: %w", err)
	}

	authService, err := auth.NewService(authStore, authCfg, mail, issuer)
	if err != nil {
		return nil, fmt.Errorf("init auth service: %w", err)
	}

	err = authService.EnsureSettingsDefaults(context.Background())
	if err != nil {
		return nil, fmt.Errorf("seed settings: %w", err)
	}

	return authService, nil
}

func bootstrapAuthAdmin(ctx context.Context, authService *auth.Service) error {
	adminEmail := envOrDefault("SUDOSTREAM_ADMIN_EMAIL", "admin@localhost.lan")
	adminPassword := os.Getenv("SUDOSTREAM_ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "changeme"
		log.Print("warning: SUDOSTREAM_ADMIN_PASSWORD not set; using default dev password")
	}

	err := authService.SeedAdmin(ctx, adminEmail, adminPassword)
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Print("warning: invalid duration env value, using default")

		return fallback
	}

	return parsed
}

func loadJWTSecret() []byte {
	raw := os.Getenv("SUDOSTREAM_JWT_SECRET")
	if raw == "" {
		if gin.Mode() == gin.ReleaseMode {
			log.Fatal("SUDOSTREAM_JWT_SECRET is required when GIN_MODE=release")
		}

		log.Print("warning: SUDOSTREAM_JWT_SECRET not set; using dev-only signing key")

		return []byte("dev-only-jwt-secret-32-bytes-min!!")
	}

	secret, err := auth.ParseJWTSecret(raw)
	if err != nil {
		log.Fatal(err)
	}

	return secret
}

func loadTOTPEncryptionKey() []byte {
	raw := os.Getenv("SUDOSTREAM_TOTP_ENCRYPTION_KEY")
	if raw == "" {
		if gin.Mode() == gin.ReleaseMode {
			log.Fatal("SUDOSTREAM_TOTP_ENCRYPTION_KEY is required when GIN_MODE=release")
		}

		log.Print("warning: SUDOSTREAM_TOTP_ENCRYPTION_KEY not set; using dev-only encryption key")

		return []byte("01234567890123456789012345678901")
	}

	decoded, err := auth.ParseTOTPEncryptionKey(raw)
	if err != nil {
		log.Fatal(err)
	}

	return decoded
}
