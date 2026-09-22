package httpapi

import (
	"context"
	"sudoStream/internal/dlna"
	"sudoStream/internal/favorite"
	"sudoStream/internal/homeshelf"
	"sudoStream/internal/maintenance"
	"sudoStream/internal/network"
	"sudoStream/internal/provider"
	"sudoStream/internal/transcode"
	"sudoStream/internal/trash"
	"sudoStream/internal/usersub"
)

// RouteConfig configures HTTP route registration.
type RouteConfig struct {
	CookieSecure      bool
	MediaRoot         string
	DBPing            func(context.Context) error
	TranscodeSettings transcode.SettingsStore
	NetworkSettings   network.SettingsStore
	NetworkLive       *network.Live
	Maintenance       *maintenance.Service
	Providers         *provider.Service
	ProviderArtifacts provider.ArtifactStore
	ProviderCache     *provider.Cache
	Favorite          *favorite.Service
	HomeShelf         *homeshelf.Service
	Trash             *trash.Service
	UserSubtitle      *usersub.Service
	DLNASettings      dlna.SettingsStore
	DLNAController    *dlna.Controller
}
