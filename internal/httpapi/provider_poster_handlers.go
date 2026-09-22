package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sudoStream/internal/access"
	"sudoStream/internal/observability"
	"sudoStream/internal/provider"

	"github.com/gin-gonic/gin"
)

// posterCache serves cached provider posters and tells the catalog which paths have one.
type posterCache struct {
	artifacts provider.ArtifactStore
	cache     *provider.Cache
}

// PosterPaths returns the media paths in a library that own a cached provider poster. Used by
// the catalog to prefer a provider poster over the generated video thumbnail.
func (p *posterCache) PosterPaths(
	ctx context.Context,
	libraryID string,
) (map[string]struct{}, error) {
	if p == nil || p.artifacts == nil {
		return map[string]struct{}{}, nil
	}

	rows, err := p.artifacts.ListArtifacts(ctx, libraryID, provider.ArtifactKindPoster)
	if err != nil {
		return nil, fmt.Errorf("list provider posters: %w", err)
	}

	paths := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		paths[row.RelPath] = struct{}{}
	}

	return paths, nil
}

// Serve a cached provider poster.
//
//	@Summary		Get provider poster
//	@Description	Serves a locally cached poster from providers.posters. 404 when none cached; use /api/thumbnail.
//	@Tags			media
//	@Produce		image/jpeg,image/webp,image/png
//	@Param			path	path	string	true	"Path under media root"
//	@Success		200
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/provider-poster/{path} [get]
func (h *handler) providerPoster(c *gin.Context) {
	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	if h.posters == nil || h.posters.artifacts == nil || h.posters.cache == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "provider poster not available"})

		return
	}

	relPath := access.NormalizeRelPath(pathParam)

	artifact, err := h.posters.artifacts.GetArtifactByPath(
		c.Request.Context(),
		relPath,
		provider.ArtifactKindPoster,
		nil,
	)
	if errors.Is(err, provider.ErrNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "provider poster not available"})

		return
	}
	if err != nil {
		observability.LogAction(
			c,
			"media.providerPoster.error",
			slog.String("path", relPath),
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to load poster"})

		return
	}

	absPath, err := h.posters.cache.Path(artifact.CachePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to load poster"})

		return
	}

	c.Header("Content-Type", provider.PosterContentType(artifact.CachePath))
	c.Header("Cache-Control", "public, max-age=86400")
	http.ServeFile(c.Writer, c.Request, absPath)

	observability.LogAction(c, "media.providerPoster", slog.String("path", relPath))
}
