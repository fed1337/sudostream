package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type openAPISpec struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

type routeKey struct {
	method string
	path   string
}

const (
	routeAdminUsers             = "/api/admin/users"
	routeAdminSettings          = "/api/admin/settings"
	routeAdminTranscodeSettings = "/api/admin/transcode/settings"
	routeAdminDLNASettings      = "/api/admin/dlna/settings"
	routeAdminNetworkSettings   = "/api/admin/network/settings"
	routeAdminNetworkTest       = "/api/admin/network/settings/test"
	routeAdminInvites           = "/api/admin/invites"
	routeMetadataPath           = "/api/metadata/{path}"
	routeAdminUserLibraries     = "/api/admin/users/{id}/libraries"
	routeOAuthEndpoint          = "/api/oauth/token"
	routeUserSubtitlePath       = "/api/user-subtitle/{path}"
)

// apiRouteCatalog lists every public HTTP API route that must appear in OpenAPI.
// Update this list when adding handlers; each route needs a @Router annotation.
func apiRouteCatalog() []routeKey {
	return []routeKey{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/metrics"},
		{http.MethodPost, routeOAuthEndpoint},
		{http.MethodPost, "/api/auth/login"},
		{http.MethodPost, "/api/auth/refresh"},
		{http.MethodPost, "/api/auth/logout"},
		{http.MethodPost, "/api/auth/forgot-password"},
		{http.MethodPost, "/api/auth/reset-password"},
		{http.MethodGet, "/api/auth/confirm-email"},
		{http.MethodPost, "/api/auth/accept-invite"},
		{http.MethodPost, "/api/auth/2fa/verify"},
		{http.MethodGet, "/api/auth/me"},
		{http.MethodGet, "/api/me/continue"},
		{http.MethodGet, "/api/me/favorites"},
		{http.MethodGet, "/api/me/watched"},
		{http.MethodGet, "/api/me/unwatched"},
		{http.MethodGet, "/api/me/stats"},
		{http.MethodPost, "/api/auth/change-password"},
		{http.MethodPost, "/api/auth/change-email"},
		{http.MethodGet, "/api/auth/sessions"},
		{http.MethodDelete, "/api/auth/sessions/{id}"},
		{http.MethodPost, "/api/auth/sessions/revoke-all"},
		{http.MethodPost, "/api/auth/2fa/setup"},
		{http.MethodPost, "/api/auth/2fa/confirm"},
		{http.MethodDelete, "/api/auth/2fa"},
		{http.MethodGet, "/api/browse"},
		{http.MethodGet, "/api/browse/{path}"},
		{http.MethodGet, "/api/libraries"},
		{http.MethodGet, "/api/catalog/{librarySlug}"},
		{http.MethodGet, "/api/catalog/{librarySlug}/shows/{showKey}"},
		{http.MethodGet, "/api/catalog/{librarySlug}/shows/{showKey}/seasons/{season}/episodes"},
		{http.MethodGet, "/api/download/{path}"},
		{http.MethodGet, "/api/stream/{path}"},
		{http.MethodGet, "/api/thumbnail/{path}"},
		{http.MethodGet, "/api/provider-poster/{path}"},
		{http.MethodGet, "/api/provider-subtitle/{path}"},
		{http.MethodPut, routeUserSubtitlePath},
		{http.MethodGet, routeUserSubtitlePath},
		{http.MethodDelete, routeUserSubtitlePath},
		{http.MethodGet, "/api/play/{path}"},
		{http.MethodGet, "/api/playback/{path}"},
		{http.MethodPost, "/api/playback/{path}"},
		{http.MethodGet, "/api/watch/{path}"},
		{http.MethodPatch, "/api/watch/{path}"},
		{http.MethodGet, "/api/favorite/{path}"},
		{http.MethodPatch, "/api/favorite/{path}"},
		{http.MethodDelete, "/api/media/{path}"},
		{http.MethodGet, routeMetadataPath},
		{http.MethodPatch, routeMetadataPath},
		{http.MethodGet, routeAdminUsers},
		{http.MethodPost, routeAdminUsers},
		{http.MethodPatch, "/api/admin/users/{id}"},
		{http.MethodDelete, "/api/admin/users/{id}"},
		{http.MethodGet, routeAdminSettings},
		{http.MethodPatch, routeAdminSettings},
		{http.MethodGet, routeAdminTranscodeSettings},
		{http.MethodPatch, routeAdminTranscodeSettings},
		{http.MethodGet, routeAdminDLNASettings},
		{http.MethodPatch, routeAdminDLNASettings},
		{http.MethodGet, routeAdminNetworkSettings},
		{http.MethodPatch, routeAdminNetworkSettings},
		{http.MethodPost, routeAdminNetworkTest},
		{http.MethodPost, routeAdminInvites},
		{http.MethodGet, routeAdminInvites},
		{http.MethodDelete, "/api/admin/invites/{id}"},
		{http.MethodPost, "/api/admin/invites/{id}/resend"},
		{http.MethodGet, "/api/admin/users/{id}/sessions"},
		{http.MethodDelete, "/api/admin/sessions/{id}"},
		{http.MethodPost, "/api/admin/users/{id}/sessions/revoke-all"},
		{http.MethodPost, "/api/admin/users/{id}/2fa/reset"},
		{http.MethodPost, "/api/admin/users/{id}/resend-confirm"},
		{http.MethodGet, "/api/search"},
		{http.MethodGet, "/api/admin/libraries"},
		{http.MethodPost, "/api/admin/libraries"},
		{http.MethodPost, "/api/admin/libraries/sync"},
		{http.MethodGet, "/api/admin/folder-tree"},
		{http.MethodPost, "/api/admin/libraries/{id}/sync"},
		{http.MethodPost, "/api/admin/libraries/{id}/roots"},
		{http.MethodDelete, "/api/admin/libraries/{id}/roots"},
		{http.MethodPatch, "/api/admin/libraries/{id}"},
		{http.MethodDelete, "/api/admin/libraries/{id}"},
		{http.MethodGet, "/api/admin/libraries/{id}/maintenance"},
		{http.MethodPatch, "/api/admin/libraries/{id}/maintenance"},
		{http.MethodGet, "/api/admin/libraries/{id}/providers"},
		{http.MethodPatch, "/api/admin/libraries/{id}/providers"},
		{http.MethodGet, "/api/admin/maintenance"},
		{http.MethodPatch, "/api/admin/maintenance"},
		{http.MethodGet, "/api/admin/maintenance/runs"},
		{http.MethodPost, "/api/admin/maintenance/{action}/run"},
		{http.MethodGet, "/api/admin/trash"},
		{http.MethodDelete, "/api/admin/trash"},
		{http.MethodGet, "/api/admin/trash/settings"},
		{http.MethodPatch, "/api/admin/trash/settings"},
		{http.MethodPost, "/api/admin/trash/{id}/restore"},
		{http.MethodDelete, "/api/admin/trash/{id}"},
		{http.MethodGet, routeAdminUserLibraries},
		{http.MethodPut, routeAdminUserLibraries},
	}
}

func TestSwagger_EveryAPIRouteDocumented(t *testing.T) {
	t.Parallel()

	allure.Test(t, "every API route appears in generated OpenAPI", func(a *allure.Context) {
		t := a.T()
		documented, err := loadOpenAPIRoutes()
		if err != nil {
			t.Fatalf("load openapi: %v", err)
		}

		for _, route := range apiRouteCatalog() {
			if !documented[route] {
				t.Fatalf("missing OpenAPI docs for %s %s", route.method, route.path)
			}
		}

		for route := range documented {
			if !catalogHasRoute(route) {
				t.Fatalf(
					"OpenAPI documents unexpected route %s %s; update apiRouteCatalog or remove stale annotation",
					route.method,
					route.path,
				)
			}
		}
	})
}

func catalogHasRoute(route routeKey) bool {
	return slices.Contains(apiRouteCatalog(), route)
}

func loadOpenAPIRoutes() (map[routeKey]bool, error) {
	spec, err := loadOpenAPISpec()
	if err != nil {
		return nil, err
	}

	routes := make(map[routeKey]bool, len(spec.Paths)*2)
	for path, methods := range spec.Paths {
		for method := range methods {
			routes[routeKey{
				method: strings.ToUpper(method),
				path:   path,
			}] = true
		}
	}

	return routes, nil
}

func loadOpenAPISpec() (openAPISpec, error) {
	path := filepath.Clean(filepath.Join("..", "..", "openapi", "swagger.json"))

	raw, err := os.ReadFile(path)
	if err != nil {
		return openAPISpec{}, fmt.Errorf("read openapi spec: %w", err)
	}

	var spec openAPISpec

	err = json.Unmarshal(raw, &spec)
	if err != nil {
		return openAPISpec{}, fmt.Errorf("parse openapi spec: %w", err)
	}

	return spec, nil
}
