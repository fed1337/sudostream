package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sudoStream/internal/access"
	"sudoStream/internal/observability"
	"sudoStream/internal/provider"

	"github.com/gin-gonic/gin"
)

const (
	errProvidersUnavailable       = "providers unavailable"
	errProviderLibraryTypeMessage = "providers only apply to film and series libraries"
	errLoadProviderSettingsFailed = "load provider settings failed"
	errSaveProviderSettingsFailed = "update provider settings failed"
)

// providerSettingsService is the admin-facing FI-1 provider settings API surface.
type providerSettingsService interface {
	GetSettings(ctx context.Context, libraryID string) (provider.Settings, error)
	PatchSettings(ctx context.Context, libraryID string, input provider.Settings) (provider.Settings, error)
	Registry() *provider.Registry
}

// ProviderAvailability lists registry-backed provider keys valid for each slot (FI-1 L11).
type ProviderAvailability struct {
	Metadata []string `json:"metadata"`
	Poster   []string `json:"poster"`
	Subtitle []string `json:"subtitle"`
}

// ProviderSettingsResponse is the JSON body for library provider settings endpoints.
type ProviderSettingsResponse struct {
	Settings  provider.Settings    `json:"settings"`
	Available ProviderAvailability `json:"available"`
}

// PatchProviderSettingsRequest replaces a library's provider configuration.
type PatchProviderSettingsRequest struct {
	MetadataProvider          *string  `json:"metadataProvider"`
	PosterProvider            *string  `json:"posterProvider"`
	SubtitleProvider          *string  `json:"subtitleProvider"`
	SubtitleLanguages         []string `json:"subtitleLanguages"`
	AllowOverrideUserMetadata bool     `json:"allowOverrideUserMetadata"`
	MetadataApplyMode         string   `json:"metadataApplyMode"`
	MetadataWriteTarget       string   `json:"metadataWriteTarget"`
}

// GetLibraryProviders returns provider settings and registry-backed availability.
//
//	@Summary		Get library providers
//	@Description	Admin only. Metadata/poster/subtitle provider settings for film and series libraries. Never mandatory.
//	@Tags			admin
//	@Produce		json
//	@Param			id	path		string	true	"Library ID"
//	@Success		200	{object}	ProviderSettingsResponse
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		503	{object}	ErrorResponse
//	@Router			/api/admin/libraries/{id}/providers [get]
func (h *adminHandler) getLibraryProviders(c *gin.Context) {
	if h.providers == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errProvidersUnavailable})

		return
	}

	library, ok := h.requireProviderLibrary(c)
	if !ok {
		return
	}

	settings, err := h.providers.GetSettings(c.Request.Context(), library.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadProviderSettingsFailed})

		return
	}

	c.JSON(http.StatusOK, ProviderSettingsResponse{
		Settings:  settings,
		Available: providerAvailability(h.providers.Registry()),
	})
}

// PatchLibraryProviders replaces a library's provider configuration.
//
//	@Summary		Update library providers
//	@Description	Admin only. At most one provider per slot (metadata/poster/subtitle); all slots optional.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string							true	"Library ID"
//	@Param			body	body		PatchProviderSettingsRequest	true	"Provider settings"
//	@Success		200		{object}	ProviderSettingsResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		503		{object}	ErrorResponse
//	@Router			/api/admin/libraries/{id}/providers [patch]
func (h *adminHandler) patchLibraryProviders(c *gin.Context) {
	if h.providers == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errProvidersUnavailable})

		return
	}

	library, ok := h.requireProviderLibrary(c)
	if !ok {
		return
	}

	var body PatchProviderSettingsRequest

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	settings, err := h.providers.PatchSettings(c.Request.Context(), library.ID, provider.Settings{
		MetadataProvider:          body.MetadataProvider,
		PosterProvider:            body.PosterProvider,
		SubtitleProvider:          body.SubtitleProvider,
		SubtitleLanguages:         body.SubtitleLanguages,
		AllowOverrideUserMetadata: body.AllowOverrideUserMetadata,
		MetadataApplyMode:         body.MetadataApplyMode,
		MetadataWriteTarget:       body.MetadataWriteTarget,
	})
	if err != nil {
		writeProviderSettingsError(c, err)

		return
	}

	observability.LogAction(
		c,
		"library.providers.updated",
		slog.String("library_id", library.ID),
	)

	c.JSON(http.StatusOK, ProviderSettingsResponse{
		Settings:  settings,
		Available: providerAvailability(h.providers.Registry()),
	})
}

// requireProviderLibrary loads and validates the path library exists and is film/series
// (FI-1 L1: scope). Writes the error response itself when returning false.
func (h *adminHandler) requireProviderLibrary(c *gin.Context) (access.Library, bool) {
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return access.Library{}, false
	}

	library, err := h.access.GetLibrary(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, access.ErrLibraryNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: libraryNotFoundMessage})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadLibraryFailed})
		}

		return access.Library{}, false
	}

	if library.Type != access.LibraryTypeFilm && library.Type != access.LibraryTypeSeries {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: errProviderLibraryTypeMessage})

		return access.Library{}, false
	}

	return library, true
}

func providerAvailability(registry *provider.Registry) ProviderAvailability {
	if registry == nil {
		return ProviderAvailability{Metadata: []string{}, Poster: []string{}, Subtitle: []string{}}
	}

	return ProviderAvailability{
		Metadata: registry.Metadata(),
		Poster:   registry.Poster(),
		Subtitle: registry.Subtitle(),
	}
}

func writeProviderSettingsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, provider.ErrInvalidMetadataProvider),
		errors.Is(err, provider.ErrInvalidPosterProvider),
		errors.Is(err, provider.ErrInvalidSubtitleProvider),
		errors.Is(err, provider.ErrInvalidApplyMode),
		errors.Is(err, provider.ErrInvalidWriteTarget),
		errors.Is(err, provider.ErrInvalidSubtitleLanguage),
		errors.Is(err, provider.ErrSubtitleLanguagesRequired):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errSaveProviderSettingsFailed})
	}
}
