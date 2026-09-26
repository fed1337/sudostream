// Package httpapi registers HTTP handlers for media browsing and streaming.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/catalog"
	"sudoStream/internal/favorite"
	"sudoStream/internal/homeshelf"
	"sudoStream/internal/maintenance"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/observability"
	"sudoStream/internal/playback"
	"sudoStream/internal/thumbnail"
	"sudoStream/internal/transcode"
	"sudoStream/internal/trash"
	"sudoStream/internal/usersub"
	"sudoStream/internal/watch"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

const (
	errorKey             = "error"
	defaultThumbnailSize = 320
	minThumbnailSize     = 32
	maxThumbnailSize     = 1024
	supportedImageError  = "file is not a supported image"
	thumbnailEncodeError = "failed to encode thumbnail"
	invalidPathError     = "invalid path"
)

// ErrorResponse is the common JSON error payload returned by the API.
type ErrorResponse struct {
	Error string `json:"error"`
}

type handler struct {
	auth       *auth.Service
	media      *mediafs.Service
	access     *access.Service
	metadata   *metadata.Service
	watch      *watch.Service
	favorite   *favorite.Service
	homeShelf  *homeshelf.Service
	catalog    *catalog.Service
	videoThumb *thumbnail.VideoThumbnailer
	transcode  *transcode.Service
	trash      *trash.Service
	usersub    *usersub.Service
	posters    *posterCache
	dbPing     func(context.Context) error
}

// Workers holds background services that need graceful shutdown.
type Workers struct {
	Transcode   *transcode.Service
	Maintenance *maintenance.Service
}

// RegisterRoutes attaches media, auth, and API routes to router.
//
//nolint:funlen,cyclop // setup function
func RegisterRoutes(
	router *gin.Engine,
	media *mediafs.Service,
	authService *auth.Service,
	accessService *access.Service,
	metadataService *metadata.Service,
	watchService *watch.Service,
	cfg RouteConfig,
) *Workers {
	videoThumb, err := thumbnail.NewVideoThumbnailer(cacheSubdir("thumbs"))
	if err != nil {
		slog.Error("failed to init video thumbnailer", "err", err)
	}
	transcodeSvc, err := transcode.NewService(cacheSubdir("hls"))
	if err != nil {
		slog.Error("failed to init transcode service", "err", err)
	}
	wireTranscodeHwAccel(transcodeSvc, cfg.TranscodeSettings)
	if cfg.Trash != nil {
		cfg.Trash.SetCleaner(trashCleanup{
			media:     media,
			metadata:  metadataService,
			watch:     watchService,
			favorite:  cfg.Favorite,
			thumbs:    videoThumb,
			transcode: transcodeSvc,
		})
	}
	if cfg.Maintenance != nil {
		cfg.Maintenance.SetMediaDeps(media, videoThumb, transcodeSvc)
	}

	registerInventoryMetrics(transcodeSvc, accessService, metadataService, authService, cfg.Trash)

	mediaHandler := &handler{
		auth:       authService,
		media:      media,
		access:     accessService,
		metadata:   metadataService,
		watch:      watchService,
		favorite:   cfg.Favorite,
		homeShelf:  cfg.HomeShelf,
		videoThumb: videoThumb,
		transcode:  transcodeSvc,
		trash:      cfg.Trash,
		usersub:    cfg.UserSubtitle,
		dbPing:     cfg.DBPing,
	}
	if mediaHandler.usersub == nil {
		userSubSvc, userSubErr := usersub.NewService(cacheSubdir("user-subs"))
		if userSubErr != nil {
			slog.Error("failed to init user subtitle cache", "err", userSubErr)
		} else {
			mediaHandler.usersub = userSubSvc
		}
	}
	if cfg.ProviderArtifacts != nil && cfg.ProviderCache != nil {
		mediaHandler.posters = &posterCache{
			artifacts: cfg.ProviderArtifacts,
			cache:     cfg.ProviderCache,
		}
	}
	if metadataService != nil && accessService != nil {
		mediaHandler.catalog = catalog.NewService(metadataService, accessService, metadataService)
		mediaHandler.catalog.SetPosterIndex(mediaHandler.posters)
		if mediaHandler.homeShelf != nil {
			mediaHandler.homeShelf.SetPosterIndex(mediaHandler.posters)
		}
	}
	authRoutes := &authHandler{
		auth:   authService,
		config: cfg,
	}
	adminRoutes := &adminHandler{
		auth:              authService,
		access:            accessService,
		media:             media,
		mediaRoot:         cfg.MediaRoot,
		metadata:          metadataService,
		videoThumb:        videoThumb,
		transcodeSettings: cfg.TranscodeSettings,
		networkSettings:   cfg.NetworkSettings,
		networkLive:       cfg.NetworkLive,
		trash:             cfg.Trash,
		dlnaSettings:      cfg.DLNASettings,
		dlnaController:    cfg.DLNAController,
	}
	if cfg.Maintenance != nil {
		adminRoutes.maintenance = cfg.Maintenance
	}
	if cfg.Providers != nil {
		adminRoutes.providers = cfg.Providers
	}

	authGroup := router.Group("/api/auth")
	{
		authGroup.POST("/login", authRoutes.login)
		authGroup.POST("/refresh", authRoutes.refresh)
		authGroup.POST("/logout", authRoutes.logout)
		authGroup.POST("/forgot-password", authRoutes.forgotPassword)
		authGroup.POST("/reset-password", authRoutes.resetPassword)
		authGroup.GET("/confirm-email", authRoutes.confirmEmail)
		authGroup.POST("/accept-invite", authRoutes.acceptInvite)
		authGroup.POST("/2fa/verify", authRoutes.verifyTwoFactor)
		if authService != nil {
			authGroup.POST("/2fa/setup", optionalAuth(authService), authRoutes.setupTwoFactor)
			authGroup.POST("/2fa/confirm", optionalAuth(authService), authRoutes.confirmTwoFactor)
		} else {
			authGroup.POST("/2fa/setup", authRoutes.setupTwoFactor)
			authGroup.POST("/2fa/confirm", authRoutes.confirmTwoFactor)
		}
	}

	if authService != nil {
		oauthRoutes := &oauthHandler{auth: authService, config: cfg}
		router.POST("/api/oauth/token", oauthRoutes.token)
	}

	api := router.Group("/api")
	api.GET("/health", mediaHandler.health)
	if authService != nil {
		api.Use(jwtMiddleware(authService))
	}
	{
		api.GET("/auth/me", authRoutes.me)
		api.GET("/me/continue", mediaHandler.getHomeContinue)
		api.GET("/me/favorites", mediaHandler.getHomeFavorites)
		api.GET("/me/watched", mediaHandler.getHomeWatched)
		api.GET("/me/unwatched", mediaHandler.getHomeUnwatched)
		api.GET("/me/stats", mediaHandler.getHomeStats)
		api.GET("/me/playback-preferences", mediaHandler.getPlaybackPreferences)
		api.PATCH("/me/playback-preferences", mediaHandler.patchPlaybackPreferences)
		api.POST("/auth/change-password", authRoutes.changePassword)
		api.POST("/auth/change-email", authRoutes.changeEmail)
		api.GET("/auth/sessions", authRoutes.listSessions)
		api.DELETE("/auth/sessions/:id", authRoutes.revokeSession)
		api.POST("/auth/sessions/revoke-all", authRoutes.revokeAllSessions)
		api.DELETE("/auth/2fa", authRoutes.disableTwoFactor)
		api.GET("/browse", mediaHandler.browseRoot)
		api.GET("/browse/*path", mediaHandler.browse)
		api.GET("/libraries", mediaHandler.listReadableLibraries)
		api.GET("/search", mediaHandler.searchMedia)
		api.GET("/catalog/:librarySlug", mediaHandler.getLibraryCatalog)
		api.GET("/catalog/:librarySlug/shows/:showKey", mediaHandler.getShowCatalog)
		api.GET(
			"/catalog/:librarySlug/shows/:showKey/seasons/:season/episodes",
			mediaHandler.getShowSeasonEpisodes,
		)
		api.GET("/download/*path", mediaHandler.download)
		api.GET("/stream/*path", mediaHandler.stream)
		api.GET("/thumbnail/*path", mediaHandler.thumbnail)
		api.GET("/provider-poster/*path", mediaHandler.providerPoster)
		api.GET("/provider-subtitle/*path", mediaHandler.providerSubtitle)
		api.PUT("/user-subtitle/*path", mediaHandler.putUserSubtitle)
		api.GET("/user-subtitle/*path", mediaHandler.getUserSubtitle)
		api.DELETE("/user-subtitle/*path", mediaHandler.deleteUserSubtitle)
		api.GET("/play/*path", mediaHandler.play)
		api.GET("/playback/*path", mediaHandler.playback)
		api.POST("/playback/*path", mediaHandler.playbackNegotiate)
		api.GET("/watch/*path", mediaHandler.getWatch)
		api.PATCH("/watch/*path", mediaHandler.patchWatch)
		api.GET("/favorite/*path", mediaHandler.getFavorite)
		api.PATCH("/favorite/*path", mediaHandler.patchFavorite)
		api.DELETE("/media/*path", mediaHandler.deleteMedia)
		api.GET("/metadata/*path", mediaHandler.getMetadata)
		api.PATCH("/metadata/*path", mediaHandler.patchMetadata)
	}

	if authService != nil {
		registerAdminRoutes(api, adminRoutes)
	}

	return &Workers{Transcode: transcodeSvc, Maintenance: cfg.Maintenance}
}

func registerAdminRoutes(api *gin.RouterGroup, adminRoutes *adminHandler) { //nolint:funlen // admin route table
	admin := api.Group("/admin")
	admin.Use(requireAdmin())
	{
		admin.GET("/users", adminRoutes.listUsers)
		admin.POST("/users", adminRoutes.createUser)
		admin.PATCH("/users/:id", adminRoutes.updateUser)
		admin.DELETE("/users/:id", adminRoutes.deleteUser)
		admin.GET("/settings", adminRoutes.getSettings)
		admin.PATCH("/settings", adminRoutes.patchSettings)
		admin.GET("/transcode/settings", adminRoutes.getTranscodeSettings)
		admin.PATCH("/transcode/settings", adminRoutes.patchTranscodeSettings)
		admin.GET("/dlna/settings", adminRoutes.getDLNASettings)
		admin.PATCH("/dlna/settings", adminRoutes.patchDLNASettings)
		registerAdminNetworkRoutes(admin, adminRoutes)
		admin.POST("/invites", adminRoutes.createInvite)
		admin.GET("/invites", adminRoutes.listInvites)
		admin.DELETE("/invites/:id", adminRoutes.revokeInvite)
		admin.POST("/invites/:id/resend", adminRoutes.resendInvite)
		admin.GET("/users/:id/sessions", adminRoutes.listUserSessions)
		admin.DELETE("/sessions/:id", adminRoutes.revokeSession)
		admin.POST("/users/:id/sessions/revoke-all", adminRoutes.revokeAllSessions)
		admin.POST("/users/:id/2fa/reset", adminRoutes.resetUserTwoFactor)
		admin.POST("/users/:id/resend-confirm", adminRoutes.resendConfirmEmail)
		admin.GET("/libraries", adminRoutes.listLibraries)
		admin.POST("/libraries", adminRoutes.createLibrary)
		admin.POST("/libraries/sync", adminRoutes.syncLibraries)
		admin.GET("/folder-tree", adminRoutes.folderTree)
		admin.POST("/libraries/:id/sync", adminRoutes.syncLibrary)
		admin.POST("/libraries/:id/roots", adminRoutes.addLibraryRoot)
		admin.DELETE("/libraries/:id/roots", adminRoutes.removeLibraryRoot)
		admin.PATCH("/libraries/:id", adminRoutes.patchLibrary)
		admin.DELETE("/libraries/:id", adminRoutes.deleteLibrary)
		admin.GET("/libraries/:id/maintenance", adminRoutes.getLibraryMaintenance)
		admin.PATCH("/libraries/:id/maintenance", adminRoutes.patchLibraryMaintenance)
		admin.GET("/libraries/:id/providers", adminRoutes.getLibraryProviders)
		admin.PATCH("/libraries/:id/providers", adminRoutes.patchLibraryProviders)
		admin.GET("/maintenance", adminRoutes.getMaintenance)
		admin.PATCH("/maintenance", adminRoutes.patchMaintenance)
		admin.GET("/maintenance/runs", adminRoutes.listMaintenanceRuns)
		admin.POST("/maintenance/:action/run", adminRoutes.runMaintenance)
		registerAdminTrashRoutes(admin, adminRoutes)
		admin.GET("/users/:id/libraries", adminRoutes.getUserLibraries)
		admin.PUT("/users/:id/libraries", adminRoutes.putUserLibraries)
	}
}

func registerAdminNetworkRoutes(admin *gin.RouterGroup, adminRoutes *adminHandler) {
	admin.GET("/network/settings", adminRoutes.getNetworkSettings)
	admin.PATCH("/network/settings", adminRoutes.patchNetworkSettings)
	admin.POST("/network/settings/test", adminRoutes.testNetworkSettings)
}

func registerAdminTrashRoutes(admin *gin.RouterGroup, adminRoutes *adminHandler) {
	admin.GET("/trash", adminRoutes.listTrash)
	admin.DELETE("/trash", adminRoutes.emptyTrash)
	admin.GET("/trash/settings", adminRoutes.getTrashSettings)
	admin.PATCH("/trash/settings", adminRoutes.patchTrashSettings)
	admin.POST("/trash/:id/restore", adminRoutes.restoreTrash)
	admin.DELETE("/trash/:id", adminRoutes.deleteTrashForever)
}

func wireTranscodeHwAccel(service *transcode.Service, store transcode.SettingsStore) {
	if service == nil || store == nil {
		return
	}

	service.SetSettingsProvider(func() transcode.TranscodeSettings {
		settings, err := store.GetTranscodeSettings(context.Background())
		if err != nil {
			return transcode.DefaultTranscodeSettings()
		}

		return settings
	})
}

// Browse media root.
//
//	@Summary		Browse media root
//	@Description	Returns immediate children (shallow) for the media root, paginated.
//	@Tags			media
//	@Produce		json
//	@Param			limit	query		int	false	"Page size (default 28, max 100)"
//	@Param			offset	query		int	false	"Offset (default 0)"
//	@Success		200		{object}	mediafs.BrowseResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/browse [get]
func (h *handler) browseRoot(c *gin.Context) {
	h.browseWithPath(c, "/")
}

// Browse a folder.
//
//	@Summary		Browse folder
//	@Description	Returns immediate children (shallow) for a path under the media root, paginated.
//	@Tags			media
//	@Produce		json
//	@Param			path	path		string	true	"Path under media root"
//	@Param			limit	query		int		false	"Page size (default 28, max 100)"
//	@Param			offset	query		int		false	"Offset (default 0)"
//	@Success		200		{object}	mediafs.BrowseResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/browse/{path} [get]
func (h *handler) browse(c *gin.Context) {
	h.browseWithPath(c, c.Param("path"))
}

func (h *handler) browseWithPath(c *gin.Context, rawPath string) {
	if !h.ensureBrowseAccess(c, rawPath) {
		return
	}

	response, err := h.media.Browse(rawPath)
	if err != nil {
		observability.LogAction(
			c,
			"browse.error",
			slog.String("path", rawPath),
			slog.String("error", err.Error()),
		)
		h.handlePathError(
			c,
			err,
			"failed to browse path",
			"path not found",
			"path is not a directory",
		)

		return
	}

	user, userOK := currentUser(c)
	if userOK && h.access != nil {
		response, err = h.access.FilterBrowseResponse(c.Request.Context(), user, response)
		if err != nil {
			c.JSON(
				http.StatusInternalServerError,
				gin.H{errorKey: "failed to filter browse results"},
			)

			return
		}
	}

	mediafs.ApplyPage(&response, parsePageOpts(c))

	if userOK && h.access != nil {
		response = h.enrichBrowsePermissions(c, user, response)
		response = h.enrichBrowseWatched(c, user, response)
	}

	c.JSON(http.StatusOK, response)
}

// List readable libraries.
//
//	@Summary		List readable libraries
//	@Description	Returns libraries the current user may read, including type metadata.
//	@Tags			media
//	@Produce		json
//	@Success		200	{object}	LibrariesResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/libraries [get]
func (h *handler) listReadableLibraries(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusOK, LibrariesResponse{Libraries: []access.Library{}})

		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusOK, LibrariesResponse{Libraries: []access.Library{}})

		return
	}

	libraries, err := h.access.ListReadableLibraries(c.Request.Context(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errListLibrariesFailed})

		return
	}

	c.JSON(http.StatusOK, LibrariesResponse{Libraries: libraries})
}

// Download a media file.
//
//	@Summary		Download media file
//	@Description	Downloads file content with content-disposition attachment.
//	@Tags			media
//	@Produce		application/octet-stream
//	@Param			path	path	string	true	"Path under media root"
//	@Success		200
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/download/{path} [get]
func (h *handler) download(c *gin.Context) {
	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	file, info, err := h.media.OpenFile(pathParam)
	if err != nil {
		h.handlePathError(c, err, "failed to open file", "file not found", "path is not a file")

		return
	}
	defer func() { _ = file.Close() }()

	c.Header("Content-Disposition", `attachment; filename="`+info.Name()+`"`)
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
}

// stream serves the original media file for Direct Play (Range, inline disposition).
//
//	@Summary		Stream media file (Direct Play)
//	@Description	Serves original file bytes with Range support for progressive playback. Not a download.
//	@Tags			media
//	@Produce		application/octet-stream
//	@Param			path	path	string	true	"Path under media root"
//	@Success		200
//	@Success		206
//	@Failure		400	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/stream/{path} [get]
func (h *handler) stream(c *gin.Context) {
	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	_, info, err := h.openVideoFile(pathParam)
	if err != nil {
		h.handlePathError(c, err, "failed to open file", "file not found", "path is not a file")

		return
	}

	file, openInfo, err := h.media.OpenFile(pathParam)
	if err != nil {
		h.handlePathError(c, err, "failed to open file", "file not found", "path is not a file")

		return
	}
	defer func() { _ = file.Close() }()

	ext := filepath.Ext(openInfo.Name())
	filePath, pathErr := h.media.FilePath(pathParam)
	mimeType := "video/mp4"
	if pathErr == nil {
		detected := mediafs.DetectMimeType(filePath, ext, false)
		if detected != "" && detected != "application/octet-stream" {
			mimeType = detected
		}
	}

	mediaPath := strings.TrimPrefix(pathParam, "/")
	librarySlug, libraryType := h.libraryLabels(c, mediaPath)
	observability.RecordVideoPlayerOpen(libraryType)
	observability.IncPlaybackStream(string(playback.MethodDirectPlay), "web")
	defer observability.DecPlaybackStream(string(playback.MethodDirectPlay), "web")
	observability.LogAction(
		c,
		"media.stream",
		slog.String("path", mediaPath),
		slog.String("library_slug", librarySlug),
	)

	c.Header("Content-Type", mimeType)
	c.Header("Content-Disposition", `inline; filename="`+info.Name()+`"`)
	c.Header("Accept-Ranges", "bytes")

	counter := &countingHTTPWriter{ResponseWriter: c.Writer}
	http.ServeContent(counter, c.Request, info.Name(), info.ModTime(), file)
	observability.AddPlaybackStreamBytes(string(playback.MethodDirectPlay), "web", counter.n)
}

// Create a thumbnail.
//
//	@Summary		Get thumbnail
//	@Description	Serves a cached video poster (WebP; maintenance warm only) or resizes an image on the fly.
//	@Tags			media
//	@Produce		image/jpeg,image/webp
//	@Param			path	path	string	true	"Path under media root"
//	@Param			w		query	int		false	"Thumbnail width (images only)"		minimum(32)	maximum(1024)	default(320)
//	@Param			h		query	int		false	"Thumbnail height (images only)"	minimum(32)	maximum(1024)	default(320)
//	@Success		200
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/thumbnail/{path} [get]
func (h *handler) thumbnail(c *gin.Context) {
	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	filePath, err := h.media.FilePath(pathParam)
	if err != nil {
		h.handlePathError(c, err, "failed to open media", "file not found", "path is not a file")

		return
	}

	width := clampInt(c.Query("w"), defaultThumbnailSize, minThumbnailSize, maxThumbnailSize)
	height := clampInt(c.Query("h"), defaultThumbnailSize, minThumbnailSize, maxThumbnailSize)

	info, err := os.Stat(filePath)
	if err != nil {
		h.handlePathError(c, err, "failed to stat media", "file not found", "path is not a file")

		return
	}

	mimeType := mediafs.DetectMimeType(filePath, filepath.Ext(filePath), false)

	if strings.HasPrefix(mimeType, "video/") || mediafs.IsVideoExtension(filepath.Ext(filePath)) {
		h.serveVideoThumbnail(c, pathParam, info)

		return
	}

	h.serveImageThumbnail(c, filePath, width, height)
}

func (h *handler) serveVideoThumbnail(
	c *gin.Context,
	mediaPath string,
	info os.FileInfo,
) {
	if h.videoThumb == nil {
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: "video thumbnails disabled"})

		return
	}

	mtime := info.ModTime().Unix()
	size := info.Size()

	// Video posters are cache-only: generated by thumbnails.warm, never on GET.
	thumbPath, err := h.videoThumb.OpenCached(mediaPath, mtime, size)
	if errors.Is(err, thumbnail.ErrNotCached) {
		c.JSON(http.StatusNotFound, gin.H{errorKey: "video thumbnail not available"})

		return
	}
	if err != nil {
		observability.LogAction(
			c,
			"media.thumbnail.error",
			slog.String("path", mediaPath),
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: "failed to open video thumbnail"})

		return
	}

	c.Header("Content-Type", "image/webp")
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(c.Writer, c.Request, thumbPath)

	observability.LogAction(c, "media.thumbnail", slog.String("path", mediaPath))
}

func (h *handler) serveImageThumbnail(c *gin.Context, filePath string, width, height int) {
	img, err := imaging.Open(filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorKey: supportedImageError})

		return
	}

	thumb := imaging.Thumbnail(img, width, height, imaging.Lanczos)

	c.Header("Content-Type", "image/jpeg")
	err = imaging.Encode(c.Writer, thumb, imaging.JPEG)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: thumbnailEncodeError})

		return
	}
}

func (h *handler) enrichBrowsePermissions(
	c *gin.Context,
	user auth.PublicUser,
	response mediafs.BrowseResponse,
) mediafs.BrowseResponse {
	response.Folder.Children = h.enrichItemsPermissions(c, user, response.Folder.Children)

	return response
}

func (h *handler) enrichBrowseWatched(
	c *gin.Context,
	user auth.PublicUser,
	response mediafs.BrowseResponse,
) mediafs.BrowseResponse {
	if h.watch == nil {
		return response
	}

	response.Folder.Children = h.enrichItemsWatched(
		c,
		user,
		response.Path,
		response.Folder.Children,
	)
	response.Folder.Children = h.enrichItemsFavorited(
		c,
		user,
		response.Path,
		response.Folder.Children,
	)

	return response
}

func (h *handler) enrichItemsWatched(
	c *gin.Context,
	user auth.PublicUser,
	folderPath string,
	items []mediafs.Item,
) []mediafs.Item {
	paths := make([]string, 0, len(items))
	for index := range items {
		if items[index].IsDir || items[index].Path == "" {
			continue
		}

		paths = append(paths, items[index].Path)
	}

	flags, err := h.watch.WatchedForPaths(c.Request.Context(), user.ID, folderPath, paths)
	if err != nil {
		return items
	}

	for index := range items {
		if items[index].IsDir || items[index].Path == "" {
			continue
		}

		watched := flags[items[index].Path]
		items[index].Watched = &watched
	}

	return items
}

func (h *handler) enrichItemsFavorited(
	c *gin.Context,
	user auth.PublicUser,
	folderPath string,
	items []mediafs.Item,
) []mediafs.Item {
	if h.favorite == nil {
		return items
	}

	paths := make([]string, 0, len(items))
	for index := range items {
		if items[index].IsDir || items[index].Path == "" {
			continue
		}

		paths = append(paths, items[index].Path)
	}

	flags, err := h.favorite.FavoritedForPaths(c.Request.Context(), user.ID, folderPath, paths)
	if err != nil {
		return items
	}

	for index := range items {
		if items[index].IsDir || items[index].Path == "" {
			continue
		}

		favorited := flags[items[index].Path]
		items[index].Favorited = &favorited
	}

	return items
}

func (h *handler) enrichItemsPermissions(
	c *gin.Context,
	user auth.PublicUser,
	items []mediafs.Item,
) []mediafs.Item {
	if h.access == nil {
		return items
	}

	for index := range items {
		canDelete, err := h.access.CanDelete(c.Request.Context(), user, items[index].Path)
		if err == nil && canDelete {
			items[index].Actions.CanDelete = true
		}

		if len(items[index].Children) > 0 {
			items[index].Children = h.enrichItemsPermissions(c, user, items[index].Children)
		}
	}

	return items
}

func (h *handler) handlePathError(
	c *gin.Context,
	err error,
	internalMsg, missingMsg, invalidTypeMsg string,
) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{errorKey: missingMsg})
	case errors.Is(err, mediafs.ErrPathOutsideRoot), errors.Is(err, mediafs.ErrInvalidPath):
		c.JSON(http.StatusBadRequest, gin.H{errorKey: invalidPathError})
	case errors.Is(err, fs.ErrInvalid):
		c.JSON(http.StatusBadRequest, gin.H{errorKey: invalidTypeMsg})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: internalMsg})
	}
}

func clampInt(raw string, fallback, minValue, maxValue int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}

	return value
}

func (h *handler) ensureBrowseAccess(c *gin.Context, rawPath string) bool {
	return h.ensurePermission(c, rawPath, h.access.CanBrowse)
}

func (h *handler) ensureReadAccess(c *gin.Context, rawPath string) bool {
	return h.ensurePermission(c, rawPath, h.access.CanRead)
}

func (h *handler) ensurePermission(
	c *gin.Context,
	rawPath string,
	check func(context.Context, auth.PublicUser, string) (bool, error),
) bool {
	user, ok := currentUser(c)
	if !ok || h.access == nil {
		return true
	}

	canonical, err := access.CanonicalRelPath(rawPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorKey: invalidPathError})

		return false
	}
	if canonical != "" && canonical != access.NormalizeRelPath(rawPath) {
		observability.ObserveACLPathCanonicalized()
		observability.LogAction(
			c,
			"access.path_canonicalized",
			slog.String("raw", rawPath),
			slog.String("canonical", canonical),
		)
	}

	allowed, err := check(c.Request.Context(), user, canonical)
	if err != nil {
		if errors.Is(err, access.ErrInvalidPath) {
			c.JSON(http.StatusBadRequest, gin.H{errorKey: invalidPathError})

			return false
		}
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: "access check failed"})

		return false
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{errorKey: "access denied"})

		return false
	}

	return true
}

func (h *handler) libraryLabels(c *gin.Context, rawPath string) (string, string) {
	libraryType := libraryTypeUnknown
	if h.access == nil {
		return firstMediaPathSegment(rawPath), libraryType
	}

	library, ok, err := h.access.LibraryForRelPath(c.Request.Context(), rawPath)
	if err != nil || !ok {
		return firstMediaPathSegment(rawPath), libraryType
	}

	slug := library.Slug
	if slug == "" {
		slug = library.RelPath
	}

	return slug, string(library.Type)
}

func firstMediaPathSegment(rawPath string) string {
	trimmed := strings.Trim(rawPath, "/")
	if trimmed == "" {
		return ""
	}

	return strings.Split(trimmed, "/")[0]
}

func isVideoFilename(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov", ".m4v", ".wmv", ".mpg", ".mpeg":
		return true
	default:
		return false
	}
}

func registerInventoryMetrics(
	transcodeSvc *transcode.Service,
	accessService *access.Service,
	metadataService *metadata.Service,
	authService *auth.Service,
	trashSvc *trash.Service,
) {
	observability.RegisterInventoryFuncs(observability.InventoryFuncs{
		HLSCacheStats: func() (int, int64) {
			if transcodeSvc == nil {
				return 0, 0
			}

			return transcodeSvc.CacheDirStats()
		},
		LibraryFiles: func() map[string]int64 {
			libs, err := accessService.ListLibraries(context.Background())
			if err != nil || metadataService == nil {
				return nil
			}
			out := make(map[string]int64, len(libs))
			for _, lib := range libs {
				paths, listErr := metadataService.ListIndexedPaths(context.Background(), lib.ID)
				if listErr != nil {
					continue
				}
				out[lib.Slug] = int64(len(paths))
			}

			return out
		},
		TrashItems: func() int64 {
			if trashSvc == nil {
				return 0
			}
			items, err := trashSvc.List(context.Background())
			if err != nil {
				return 0
			}

			return int64(len(items))
		},
		AuthSessions: func() int64 {
			n, err := authService.CountSessions(context.Background())
			if err != nil {
				return 0
			}

			return n
		},
	})
}

// countingHTTPWriter counts bytes written while preserving http.ResponseWriter.
type countingHTTPWriter struct {
	http.ResponseWriter

	n int64
}

func (w *countingHTTPWriter) Write(p []byte) (int, error) {
	written, err := w.ResponseWriter.Write(p)
	w.n += int64(written)
	if err != nil {
		return written, fmt.Errorf("write stream response: %w", err)
	}

	return written, nil
}
