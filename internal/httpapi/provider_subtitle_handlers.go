package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
	"sudoStream/internal/provider"

	"github.com/gin-gonic/gin"
)

// ProviderSubtitleTrack is a cached provider subtitle exposed on playback.
type ProviderSubtitleTrack struct {
	Lang  string `json:"lang"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Serve a cached provider subtitle (WebVTT).
//
//	@Summary		Get provider subtitle
//	@Description	Serves a locally cached subtitle from providers.subtitles. Query lang is ISO 639-1.
//	@Tags			media
//	@Produce		text/vtt
//	@Param			path	path		string	true	"Path under media root"
//	@Param			lang	query		string	true	"ISO 639-1 language code"
//	@Success		200
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/provider-subtitle/{path} [get]
func (h *handler) providerSubtitle(c *gin.Context) {
	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	lang := strings.ToLower(strings.TrimSpace(c.Query("lang")))
	if !provider.IsSubtitleLanguage(lang) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "lang must be ISO 639-1 alpha-2"})

		return
	}

	if h.posters == nil || h.posters.artifacts == nil || h.posters.cache == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "provider subtitle not available"})

		return
	}

	relPath := access.NormalizeRelPath(pathParam)
	artifact, err := h.posters.artifacts.GetArtifactByPath(
		c.Request.Context(),
		relPath,
		provider.ArtifactKindSubtitle,
		&lang,
	)
	if errors.Is(err, provider.ErrNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "provider subtitle not available"})

		return
	}
	if err != nil {
		observability.LogAction(
			c,
			"media.providerSubtitle.error",
			slog.String("path", relPath),
			slog.String("lang", lang),
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errFailedLoadSubtitle})

		return
	}

	absPath, err := h.posters.cache.Path(artifact.CachePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errFailedLoadSubtitle})

		return
	}

	c.Header("Content-Type", "text/vtt; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=86400")
	http.ServeFile(c.Writer, c.Request, absPath)

	observability.LogAction(
		c,
		"media.providerSubtitle",
		slog.String("path", relPath),
		slog.String("lang", lang),
	)
}

func (h *handler) providerSubtitleTracks(
	ctx context.Context,
	relPath string,
) []ProviderSubtitleTrack {
	if h.posters == nil || h.posters.artifacts == nil {
		return nil
	}

	rows, err := h.posters.artifacts.ListArtifactsByPath(
		ctx,
		access.NormalizeRelPath(relPath),
		provider.ArtifactKindSubtitle,
	)
	if err != nil || len(rows) == 0 {
		return nil
	}

	tracks := make([]ProviderSubtitleTrack, 0, len(rows))
	for _, row := range rows {
		if row.Lang == nil || *row.Lang == "" {
			continue
		}
		lang := *row.Lang
		tracks = append(tracks, ProviderSubtitleTrack{
			Lang:  lang,
			Label: lang,
			URL: "/api/provider-subtitle/" + mediafs.EscapePathSegments(row.RelPath) +
				"?lang=" + lang,
		})
	}

	return tracks
}
