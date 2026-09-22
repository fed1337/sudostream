package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/dlna"
	"sudoStream/internal/maintenance"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/network"
	"sudoStream/internal/observability"
	"sudoStream/internal/thumbnail"
	"sudoStream/internal/transcode"
	"sudoStream/internal/trash"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	jsonKeyLibraries              = "libraries"
	jsonKeyGrants                 = "grants"
	invalidInviteTokenMessage     = "invalid invite token"
	invalidRoleMessage            = "invalid role"
	librarySyncUnavailableMessage = "library sync unavailable"
	libraryNotFoundMessage        = "library not found"
	loadNetworkSettingsFailed     = "load network settings failed"
)

// LibrariesResponse is the JSON body for library list endpoints.
type LibrariesResponse struct {
	Libraries []access.Library `json:"libraries"`
}

// LibraryResponse is the JSON body for a single library.
type LibraryResponse struct {
	Library access.Library `json:"library"`
}

// PatchLibraryRequest updates library display name and/or type.
type PatchLibraryRequest struct {
	Name *string `json:"name"`
	Type *string `json:"type"`
}

// CreateLibraryRequest creates an empty library registry row.
type CreateLibraryRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Slug string `json:"slug"`
}

// AddLibraryRootRequest attaches a media-relative folder to a library.
type AddLibraryRootRequest struct {
	RelPath string `json:"relPath"`
}

// FolderTreeEntry is one directory in the admin folder picker.
type FolderTreeEntry struct {
	Name              string `json:"name"`
	RelPath           string `json:"relPath"`
	AssignedLibraryID string `json:"assignedLibraryId,omitempty"`
}

// FolderTreeResponse lists directories under a media path.
type FolderTreeResponse struct {
	Path    string            `json:"path"`
	Entries []FolderTreeEntry `json:"entries"`
}

// GrantsResponse is the JSON body for user library grant endpoints.
type GrantsResponse struct {
	Grants []access.UserGrantView `json:"grants"`
}

// PutGrantsRequest replaces all grants for a user.
type PutGrantsRequest struct {
	Grants []access.GrantInput `json:"grants"`
}

type adminHandler struct {
	auth              *auth.Service
	access            *access.Service
	media             *mediafs.Service
	mediaRoot         string
	metadata          *metadata.Service
	videoThumb        *thumbnail.VideoThumbnailer
	transcodeSettings transcode.SettingsStore
	networkSettings   network.SettingsStore
	networkLive       *network.Live
	maintenance       maintenanceRunner
	providers         providerSettingsService
	trash             *trash.Service
	dlnaSettings      dlna.SettingsStore
	dlnaController    *dlna.Controller
}

// maintenanceRunner is the admin-facing maintenance API surface.
type maintenanceRunner interface {
	GetGlobalMaintenance(ctx context.Context) ([]maintenance.ActionStatus, error)
	GetLibraryMaintenance(ctx context.Context, libraryID string) ([]maintenance.ActionStatus, error)
	PatchGlobalSchedules(ctx context.Context, inputs []maintenance.ScheduleInput) error
	PatchLibrarySchedules(
		ctx context.Context,
		libraryID string,
		inputs []maintenance.ScheduleInput,
	) error
	RunNow(ctx context.Context, action, libraryID, trigger string) (maintenance.Run, error)
	ListRuns(ctx context.Context, limit int) ([]maintenance.Run, error)
	CacheDirBytes() int64
}

// ListUsers returns all users for administration.
//
//	@Summary	List users
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	UsersResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/users [get]
func (h *adminHandler) listUsers(c *gin.Context) {
	users, err := h.auth.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "list users failed"})

		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// CreateUser creates a user account directly.
//
//	@Summary	Create user
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		CreateUserRequest	true	"New user"
//	@Success	201		{object}	UserResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	409		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/users [post]
func (h *adminHandler) createUser(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	user, err := h.auth.CreateUser(c.Request.Context(), auth.CreateUserInput{
		Email:    body.Email,
		Password: body.Password,
		Role:     body.Role,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUserExists):
			c.JSON(http.StatusConflict, ErrorResponse{Error: userExistsMessage})
		case errors.Is(err, auth.ErrInvalidRole):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRoleMessage})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create user failed"})
		}

		return
	}

	c.JSON(http.StatusCreated, gin.H{responseUserKey: user})
}

// UpdateUser patches user role or enabled state.
//
//	@Summary	Update user
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string				true	"User ID"
//	@Param		body	body		UpdateUserRequest	true	"User patch"
//	@Success	200		{object}	AdminUserResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	403		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/users/{id} [patch]
func (h *adminHandler) updateUser(c *gin.Context) {
	var body struct {
		Enabled *bool   `json:"enabled"`
		Role    *string `json:"role"`
		Email   *string `json:"email"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	user, err := h.auth.UpdateUser(c.Request.Context(), c.Param("id"), auth.UserPatch{
		Enabled: body.Enabled,
		Role:    body.Role,
		Email:   body.Email,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrForbidden):
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "cannot modify last admin"})
		case errors.Is(err, auth.ErrUserExists):
			c.JSON(http.StatusConflict, ErrorResponse{Error: userExistsMessage})
		case errors.Is(err, auth.ErrMissingEmail):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "email is required"})
		case errors.Is(err, auth.ErrInvalidRole):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRoleMessage})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "update user failed"})
		}

		return
	}

	c.JSON(http.StatusOK, gin.H{responseUserKey: user})
}

// DeleteUser permanently removes a disabled user account.
//
//	@Summary	Delete disabled user
//	@Tags		admin
//	@Param		id	path	string	true	"User ID"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	403	{object}	ErrorResponse
//	@Failure	404	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/users/{id} [delete]
func (h *adminHandler) deleteUser(c *gin.Context) {
	err := h.auth.DeleteUser(c.Request.Context(), c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUserNotDisabled):
			c.JSON(
				http.StatusBadRequest,
				ErrorResponse{Error: "user must be disabled before delete"},
			)
		case errors.Is(err, auth.ErrForbidden):
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "cannot delete last admin"})
		case errors.Is(err, auth.ErrManagedUserNotFound):
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "user not found"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "delete user failed"})
		}

		return
	}

	c.Status(http.StatusNoContent)
}

// GetSettings returns auth policy settings.
//
//	@Summary	Get auth settings
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	auth.Settings
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/settings [get]
func (h *adminHandler) getSettings(c *gin.Context) {
	settings, err := h.auth.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load settings failed"})

		return
	}

	c.JSON(http.StatusOK, settings)
}

// PatchSettings updates auth policy settings.
//
//	@Summary	Update auth settings
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		auth.Settings	true	"Settings"
//	@Success	200		{object}	auth.Settings
//	@Failure	400		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/settings [patch]
func (h *adminHandler) patchSettings(c *gin.Context) {
	var body auth.Settings

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	settings, err := h.auth.UpdateSettings(c.Request.Context(), body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "update settings failed"})

		return
	}

	c.JSON(http.StatusOK, settings)
}

// GetTranscodeSettings returns hardware encoder preference.
//
//	@Summary	Get transcode settings
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	transcode.TranscodeSettings
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/transcode/settings [get]
func (h *adminHandler) getTranscodeSettings(c *gin.Context) {
	if h.transcodeSettings == nil {
		c.JSON(http.StatusOK, transcode.DefaultTranscodeSettings())

		return
	}

	settings, err := h.transcodeSettings.GetTranscodeSettings(c.Request.Context())
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: "load transcode settings failed"},
		)

		return
	}

	c.JSON(http.StatusOK, settings)
}

// PatchTranscodeSettings updates hardware encoder preference.
//
//	@Summary	Update transcode settings
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		transcode.TranscodeSettings	true	"Transcode settings"
//	@Success	200		{object}	transcode.TranscodeSettings
//	@Failure	400		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/transcode/settings [patch]
func (h *adminHandler) patchTranscodeSettings(c *gin.Context) {
	if h.transcodeSettings == nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: "transcode settings unavailable"},
		)

		return
	}

	var body transcode.TranscodeSettings
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	normalized, err := transcode.NormalizeTranscodeSettings(body)
	if err != nil {
		msg := "invalid transcode settings"
		switch {
		case errors.Is(err, transcode.ErrInvalidHwAccel):
			msg = "invalid hwAccel value"
		case errors.Is(err, transcode.ErrInvalidDownmix):
			msg = "invalid downmixAlgorithm value"
		case errors.Is(err, transcode.ErrInvalidToneMappingAlgorithm):
			msg = "invalid toneMappingAlgorithm value"
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: msg})

		return
	}

	err = h.transcodeSettings.SaveTranscodeSettings(c.Request.Context(), normalized)
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: "save transcode settings failed"},
		)

		return
	}

	c.JSON(http.StatusOK, normalized)
}

// GetNetworkSettings returns provider outbound proxy settings.
//
//	@Summary	Get network (provider proxy) settings
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	network.Settings
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/network/settings [get]
func (h *adminHandler) getNetworkSettings(c *gin.Context) {
	if h.networkSettings == nil {
		c.JSON(http.StatusOK, network.PublicView(network.DefaultSettings()))

		return
	}

	settings, err := h.networkSettings.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: loadNetworkSettingsFailed},
		)

		return
	}

	c.JSON(http.StatusOK, network.PublicView(settings))
}

// PatchNetworkSettings updates provider outbound proxy settings.
//
//	@Summary	Update network (provider proxy) settings
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		network.Settings	true	"Network settings"
//	@Success	200		{object}	network.Settings
//	@Failure	400		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/network/settings [patch]
func (h *adminHandler) patchNetworkSettings(c *gin.Context) {
	if h.networkSettings == nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: "network settings unavailable"},
		)

		return
	}

	var body network.Settings
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	existing, err := h.networkSettings.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: loadNetworkSettingsFailed},
		)

		return
	}

	merged := network.MergePassword(body, existing)
	normalized, err := network.NormalizeSettings(merged)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})

		return
	}

	err = h.networkSettings.SaveSettings(c.Request.Context(), normalized)
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: "save network settings failed"},
		)

		return
	}

	if h.networkLive != nil {
		h.networkLive.Store(normalized)
	}

	c.JSON(http.StatusOK, network.PublicView(normalized))
}

// TestNetworkSettings probes egress through draft provider proxy settings (no save).
//
//	@Summary	Test network (provider proxy) connection
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		network.Settings	true	"Draft network settings"
//	@Success	200		{object}	network.ProbeResult
//	@Failure	400		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/network/settings/test [post]
func (h *adminHandler) testNetworkSettings(c *gin.Context) {
	var body network.Settings
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	existing := network.DefaultSettings()
	if h.networkSettings != nil {
		existing, err = h.networkSettings.GetSettings(c.Request.Context())
		if err != nil {
			c.JSON(
				http.StatusInternalServerError,
				ErrorResponse{Error: loadNetworkSettingsFailed},
			)

			return
		}
	}

	merged := network.MergePassword(body, existing)
	normalized, err := network.NormalizeSettings(merged)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})

		return
	}

	result := network.TestConnection(c.Request.Context(), normalized, network.ProbeOptions{})
	c.JSON(http.StatusOK, result)
}

// CreateInvite emails an invite link to a new user.
//
//	@Summary	Create invite
//	@Tags		admin
//	@Accept		json
//	@Param		body	body	CreateInviteRequest	true	"Invite"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	409	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/invites [post]
func (h *adminHandler) createInvite(c *gin.Context) {
	var body struct {
		Email          string `json:"email"`
		Role           string `json:"role"`
		ExpiresInHours *int   `json:"expiresInHours"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	adminUser, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	expiresIn := time.Duration(h.auth.DefaultInviteTTL()) * time.Hour
	if body.ExpiresInHours != nil && *body.ExpiresInHours > 0 {
		expiresIn = time.Duration(*body.ExpiresInHours) * time.Hour
	}

	err = h.auth.CreateInvite(
		c.Request.Context(),
		body.Email,
		body.Role,
		adminUser.ID,
		expiresIn,
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUserExists):
			c.JSON(http.StatusConflict, ErrorResponse{Error: userExistsMessage})
		case errors.Is(err, auth.ErrInvalidRole):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRoleMessage})
		case errors.Is(err, auth.ErrTVInviteForbidden):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "tv accounts cannot be invited"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "create invite failed"})
		}

		return
	}

	c.Status(http.StatusNoContent)
}

// List libraries.
//
//	@Summary		List libraries
//	@Description	Returns registered media libraries for ACL management.
//	@Tags			admin
//	@Produce		json
//	@Success		200	{object}	LibrariesResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/admin/libraries [get]
func (h *adminHandler) listLibraries(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusOK, gin.H{jsonKeyLibraries: []access.Library{}})

		return
	}

	libraries, err := h.access.ListLibraries(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errListLibrariesFailed})

		return
	}

	c.JSON(http.StatusOK, gin.H{jsonKeyLibraries: libraries})
}

// Create library.
//
//	@Summary		Create library
//	@Description	Creates an empty library. Attach folders with POST /roots.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CreateLibraryRequest	true	"Library fields"
//	@Success		201		{object}	LibraryResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/admin/libraries [post]
func (h *adminHandler) createLibrary(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	var body CreateLibraryRequest
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	library, err := h.access.CreateLibrary(c.Request.Context(), body.Name, body.Slug, body.Type)
	if err != nil {
		writeLibraryUpdateError(c, err)

		return
	}

	observability.LogAction(
		c,
		"library.created",
		slog.String("library_id", library.ID),
		slog.String("slug", library.Slug),
	)

	c.JSON(http.StatusCreated, LibraryResponse{Library: library})
}

// Delete library registry row (does not delete files).
//
//	@Summary		Delete library
//	@Description	Removes the library registry row and grants. Media files are not deleted.
//	@Tags			admin
//	@Param			id	path	string	true	"Library ID"
//	@Success		204
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/admin/libraries/{id} [delete]
func (h *adminHandler) deleteLibrary(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	err := h.access.DeleteLibrary(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, access.ErrLibraryNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: libraryNotFoundMessage})

			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "delete library failed"})

		return
	}

	observability.LogAction(c, "library.deleted", slog.String("library_id", c.Param("id")))
	c.Status(http.StatusNoContent)
}

// Add library root.
//
//	@Summary		Add library folder
//	@Description	Attaches a media-relative folder to the library. Exclusive across libraries.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Library ID"
//	@Param			body	body		AddLibraryRootRequest	true	"Folder"
//	@Success		200		{object}	LibraryResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/admin/libraries/{id}/roots [post]
func (h *adminHandler) addLibraryRoot(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	var body AddLibraryRootRequest
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	relPath := access.NormalizeRelPath(body.RelPath)
	if relPath == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: access.ErrInvalidLibraryRoot.Error()})

		return
	}

	if h.media != nil {
		_, dirErr := h.media.DirPath(relPath)
		if dirErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "folder not found"})

			return
		}
	}

	library, err := h.access.AddRoot(c.Request.Context(), c.Param("id"), relPath)
	if err != nil {
		writeLibraryRootError(c, err)

		return
	}

	observability.LogAction(
		c,
		"library.root.added",
		slog.String("library_id", library.ID),
		slog.String("rel_path", relPath),
	)

	c.JSON(http.StatusOK, LibraryResponse{Library: library})
}

// Remove library root.
//
//	@Summary		Remove library folder
//	@Description	Detaches a folder from the library. Files stay on disk.
//	@Tags			admin
//	@Produce		json
//	@Param			id		path		string	true	"Library ID"
//	@Param			path	query		string	true	"Media-relative folder"
//	@Success		200		{object}	LibraryResponse
//	@Success		204
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/admin/libraries/{id}/roots [delete]
func (h *adminHandler) removeLibraryRoot(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	relPath := access.NormalizeRelPath(c.Query("path"))
	if relPath == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: access.ErrInvalidLibraryRoot.Error()})

		return
	}

	library, err := h.access.RemoveRoot(c.Request.Context(), c.Param("id"), relPath)
	if err != nil {
		if errors.Is(err, access.ErrLibraryNotFound) {
			c.Status(http.StatusNoContent)

			return
		}
		writeLibraryRootError(c, err)

		return
	}

	observability.LogAction(
		c,
		"library.root.removed",
		slog.String("library_id", library.ID),
		slog.String("rel_path", relPath),
	)

	c.JSON(http.StatusOK, LibraryResponse{Library: library})
}

// Folder tree for admin picker.
//
//	@Summary		List media folders
//	@Description	Shallow directory listing for the library folder picker. Skips dot-directories.
//	@Tags			admin
//	@Produce		json
//	@Param			path	query		string	false	"Parent path under media root"
//	@Success		200		{object}	FolderTreeResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/admin/folder-tree [get]
func (h *adminHandler) folderTree(c *gin.Context) { //nolint:cyclop // listing + assignment lookup
	if h.media == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "media unavailable"})

		return
	}

	parent := access.NormalizeRelPath(c.Query("path"))
	dirPath, err := h.media.DirPath(parent)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "folder not found"})

		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "read folder failed"})

		return
	}

	libraries := []access.Library{}
	if h.access != nil {
		libraries, err = h.access.ListLibraries(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errListLibrariesFailed})

			return
		}
	}

	out := make([]FolderTreeEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		relPath := entry.Name()
		if parent != "" {
			relPath = parent + "/" + entry.Name()
		}

		item := FolderTreeEntry{Name: entry.Name(), RelPath: relPath}
		if library, ok := access.MatchLibrary(libraries, relPath); ok {
			item.AssignedLibraryID = library.ID
		}
		out = append(out, item)
	}

	c.JSON(http.StatusOK, FolderTreeResponse{Path: parent, Entries: out})
}

func writeLibraryRootError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, access.ErrLibraryNotFound), errors.Is(err, access.ErrLibraryRootNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, access.ErrLibraryRootConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	case errors.Is(err, access.ErrInvalidLibraryRoot),
		errors.Is(err, access.ErrInvalidLibraryName),
		errors.Is(err, access.ErrInvalidLibraryType):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "update library roots failed"})
	}
}

// Sync libraries from media root.
//
//	@Summary		Sync libraries
//	@Description	Prunes library roots whose folders are missing on disk. Does not auto-create libraries.
//	@Tags			admin
//	@Produce		json
//	@Success		200	{object}	LibrariesResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/admin/libraries/sync [post]
func (h *adminHandler) syncLibraries(c *gin.Context) {
	if h.access == nil || strings.TrimSpace(h.mediaRoot) == "" {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: librarySyncUnavailableMessage})

		return
	}

	result, err := h.access.SyncLibraries(c.Request.Context(), h.mediaRoot)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "sync libraries failed"})

		return
	}

	observability.LogAction(
		c,
		"library.registry.sync",
		slog.Int("count", len(result.Libraries)),
		slog.Int("added", result.Added),
		slog.Int("removed", result.Removed),
		slog.Int("kept", result.Kept),
	)

	c.JSON(http.StatusOK, gin.H{
		jsonKeyLibraries: result.Libraries,
		"added":          result.Added,
		"removed":        result.Removed,
		"kept":           result.Kept,
	})
}

// SyncLibrary re-indexes metadata for a single registered library.
//
//	@Summary		Sync library metadata
//	@Description	Re-probes video metadata for one library folder.
//	@Tags			admin
//	@Produce		json
//	@Param			id	path		string	true	"Library ID"
//	@Success		200	{object}	LibraryResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/admin/libraries/{id}/sync [post]
func (h *adminHandler) syncLibrary(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: librarySyncUnavailableMessage})

		return
	}

	library, err := h.access.GetLibrary(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, access.ErrLibraryNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: libraryNotFoundMessage})

			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load library failed"})

		return
	}

	observability.LogAction(
		c,
		"library.sync",
		slog.String("library_id", library.ID),
		slog.String("library_slug", library.Slug),
	)

	if h.maintenance != nil {
		run, runErr := h.maintenance.RunNow(
			c.Request.Context(),
			maintenance.ActionMetadataScan,
			library.ID,
			maintenance.TriggerManual,
		)
		if runErr != nil {
			if errors.Is(runErr, maintenance.ErrConflict) {
				c.JSON(http.StatusConflict, ErrorResponse{Error: "metadata scan already running"})

				return
			}
			c.JSON(
				http.StatusInternalServerError,
				ErrorResponse{Error: "metadata scan failed to start"},
			)

			return
		}
		c.JSON(http.StatusOK, gin.H{"library": library, "run": run})

		return
	}

	if h.metadata != nil && h.metadata.Indexer() != nil {
		h.metadata.Indexer().IndexLibraryAsync(c.Request.Context(), library)
	}

	c.JSON(http.StatusOK, LibraryResponse{Library: library})
}

// Patch library display fields.
//
//	@Summary		Update library
//	@Description	Sets display name and/or UI type for a registered library. Name is display-only.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Library ID"
//	@Param			body	body		PatchLibraryRequest	true	"Library fields"
//	@Success		200		{object}	LibraryResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/admin/libraries/{id} [patch]
func (h *adminHandler) patchLibrary(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	var body PatchLibraryRequest

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	library, err := h.access.UpdateLibrary(c.Request.Context(), c.Param("id"), body.Name, body.Type)
	if err != nil {
		writeLibraryUpdateError(c, err)

		return
	}

	observability.LogAction(
		c,
		"library.updated",
		slog.String("library_id", library.ID),
		slog.String("name", library.Name),
		slog.String("type", string(library.Type)),
	)

	c.JSON(http.StatusOK, LibraryResponse{Library: library})
}

func writeLibraryUpdateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, access.ErrLibraryNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: libraryNotFoundMessage})
	case errors.Is(err, access.ErrInvalidLibraryType),
		errors.Is(err, access.ErrInvalidLibraryName),
		errors.Is(err, access.ErrInvalidLibraryPatch),
		errors.Is(err, access.ErrLibrarySlugConflict):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "update library failed"})
	}
}

// Get user library grants.
//
//	@Summary		Get user library grants
//	@Description	Returns all libraries with the user's CRUD permissions.
//	@Tags			admin
//	@Produce		json
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{object}	GrantsResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/admin/users/{id}/libraries [get]
func (h *adminHandler) getUserLibraries(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusOK, gin.H{jsonKeyGrants: []access.UserGrantView{}})

		return
	}

	grants, err := h.access.GetUserGrants(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadGrantsFailed})

		return
	}

	c.JSON(http.StatusOK, gin.H{jsonKeyGrants: grants})
}

// Replace user library grants.
//
//	@Summary		Set user library grants
//	@Description	Replaces all library CRUD grants for a non-admin user.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"User ID"
//	@Param			body	body		PutGrantsRequest	true	"Grant list"
//	@Success		200		{object}	GrantsResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/admin/users/{id}/libraries [put]
func (h *adminHandler) putUserLibraries(c *gin.Context) {
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	var body struct {
		Grants []access.GrantInput `json:"grants"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	target, err := h.auth.GetUserByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "user not found"})

		return
	}

	err = h.access.SetUserGrants(
		c.Request.Context(),
		c.Param("id"),
		target.Role,
		body.Grants,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})

		return
	}

	observability.LogAction(
		c,
		"library.grants_updated",
		slog.String("target_user_id", c.Param("id")),
		slog.Int("grant_count", len(body.Grants)),
	)

	grants, err := h.access.GetUserGrants(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadGrantsFailed})

		return
	}

	c.JSON(http.StatusOK, gin.H{jsonKeyGrants: grants})
}

// ResendConfirmEmail resends the email confirmation link for a user.
//
//	@Summary	Resend email confirmation
//	@Tags		admin
//	@Param		id	path	string	true	"User ID"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/users/{id}/resend-confirm [post]
func (h *adminHandler) resendConfirmEmail(c *gin.Context) {
	err := h.auth.ResendConfirmEmail(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, auth.ErrResendConfirmUnavailable) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "confirmation resend unavailable"})

			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "resend confirmation failed"})

		return
	}

	c.Status(http.StatusNoContent)
}

// ListInvites returns pending invites, optionally filtered by email.
//
//	@Summary	List invites
//	@Tags		admin
//	@Produce	json
//	@Param		email	query		string	false	"Filter by email"
//	@Success	200		{object}	InvitesResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/invites [get]
func (h *adminHandler) listInvites(c *gin.Context) {
	invites, err := h.auth.ListPendingInvites(c.Request.Context(), c.Query("email"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "list invites failed"})

		return
	}

	c.JSON(http.StatusOK, gin.H{"invites": invites})
}

// ResendInvite re-sends an existing invite email.
//
//	@Summary	Resend invite
//	@Tags		admin
//	@Accept		json
//	@Param		id		path	string				true	"Invite ID"
//	@Param		body	body	ResendInviteRequest	false	"Optional TTL override"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	409	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/invites/{id}/resend [post]
func (h *adminHandler) resendInvite(c *gin.Context) {
	var body struct {
		ExpiresInHours *int `json:"expiresInHours"`
	}

	_ = c.ShouldBindJSON(&body)

	adminUser, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	expiresIn := time.Duration(h.auth.DefaultInviteTTL()) * time.Hour
	if body.ExpiresInHours != nil && *body.ExpiresInHours > 0 {
		expiresIn = time.Duration(*body.ExpiresInHours) * time.Hour
	}

	err := h.auth.ResendInvite(c.Request.Context(), c.Param("id"), adminUser.ID, expiresIn)
	if err != nil {
		if errors.Is(err, auth.ErrUserExists) {
			c.JSON(http.StatusConflict, ErrorResponse{Error: userExistsMessage})

			return
		}
		if errors.Is(err, auth.ErrInvalidToken) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidInviteTokenMessage})

			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "resend invite failed"})

		return
	}

	c.Status(http.StatusNoContent)
}

// RevokeInvite invalidates a pending invite.
//
//	@Summary	Revoke invite
//	@Tags		admin
//	@Param		id	path	string	true	"Invite ID"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/invites/{id} [delete]
func (h *adminHandler) revokeInvite(c *gin.Context) {
	err := h.auth.RevokeInvite(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidInviteTokenMessage})

			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "revoke invite failed"})

		return
	}

	c.Status(http.StatusNoContent)
}

// ListUserSessions returns sessions for any user (admin).
//
//	@Summary	List user sessions
//	@Tags		admin
//	@Produce	json
//	@Param		id	path		string	true	"User ID"
//	@Success	200	{object}	SessionsResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/users/{id}/sessions [get]
func (h *adminHandler) listUserSessions(c *gin.Context) {
	sessions, err := h.auth.AdminListUserSessions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errListSessionsFailed})

		return
	}

	c.JSON(http.StatusOK, gin.H{jsonKeySessions: sessions})
}

// RevokeSession revokes any session by ID (admin).
//
//	@Summary	Revoke session (admin)
//	@Tags		admin
//	@Param		id	path	string	true	"Session ID"
//	@Success	204
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/sessions/{id} [delete]
func (h *adminHandler) revokeSession(c *gin.Context) {
	err := h.auth.AdminRevokeSession(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errRevokeSessionFailed})

		return
	}

	c.Status(http.StatusNoContent)
}

// RevokeAllSessions revokes every session for a user (admin).
//
//	@Summary	Revoke all user sessions
//	@Tags		admin
//	@Param		id	path	string	true	"User ID"
//	@Success	204
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/users/{id}/sessions/revoke-all [post]
func (h *adminHandler) revokeAllSessions(c *gin.Context) {
	err := h.auth.AdminRevokeAllSessions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "revoke sessions failed"})

		return
	}

	c.Status(http.StatusNoContent)
}

// ResetUserTwoFactor clears 2FA for a user (admin).
//
//	@Summary	Reset user 2FA
//	@Tags		admin
//	@Param		id	path	string	true	"User ID"
//	@Success	204
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/users/{id}/2fa/reset [post]
func (h *adminHandler) resetUserTwoFactor(c *gin.Context) {
	err := h.auth.AdminResetTwoFactor(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "reset two factor failed"})

		return
	}

	c.Status(http.StatusNoContent)
}
