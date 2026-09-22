package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
	"sudoStream/internal/transcode"
	"sudoStream/internal/usersub"

	"github.com/gin-gonic/gin"
)

const (
	errSubtitleTooLarge     = "subtitle too large"
	errFailedLoadSubtitle   = "failed to load subtitle"
	errUserSubtitleNotFound = "user subtitle not found"
)

// UserSubtitleTrack is a per-user uploaded caption exposed on playback.
type UserSubtitleTrack struct {
	Lang  string `json:"lang"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

// putUserSubtitle uploads or replaces a VTT/SRT caption for the current user+path (TTL 1d).
//
//	@Summary		Upload user subtitle
//	@Description	Accepts VTT or SRT (multipart field "file"); replaces existing upload. TTL 1d.
//	@Tags			media
//	@Accept			mpfd
//	@Produce		json
//	@Param			path	path		string	true	"Path under media root"
//	@Param			lang	formData	string	false	"ISO 639-1 language code"
//	@Param			label	formData	string	false	"Display label"
//	@Param			file	formData	file	true	"Subtitle file (.vtt or .srt)"
//	@Success		200		{object}	UserSubtitleTrack
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		413		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/user-subtitle/{path} [put]
func (h *handler) putUserSubtitle(c *gin.Context) {
	user, userOK := currentUser(c)
	if !userOK {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: catalogUnauthorized})

		return
	}
	if h.usersub == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "user subtitles unavailable"})

		return
	}

	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	raw, filename, readOK := readUserSubtitleUpload(c)
	if !readOK {
		return
	}

	vtt, err := transcode.NormalizeSubtitleUpload(raw, filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid subtitle file"})

		return
	}

	relPath := access.NormalizeRelPath(pathParam)
	track, err := h.usersub.Put(user.ID, relPath, c.PostForm("lang"), c.PostForm("label"), vtt)
	if errors.Is(err, usersub.ErrTooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: errSubtitleTooLarge})

		return
	}
	if err != nil {
		observability.LogAction(
			c,
			"media.userSubtitle.error",
			slog.String("path", relPath),
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to store subtitle"})

		return
	}

	observability.LogAction(c, "media.userSubtitle.put", slog.String("path", relPath))
	c.JSON(http.StatusOK, UserSubtitleTrack{
		Lang:  track.Lang,
		Label: track.Label,
		URL:   userSubtitleURL(relPath),
	})
}

func readUserSubtitleUpload(c *gin.Context) ([]byte, string, bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "file is required"})

		return nil, "", false
	}
	if fileHeader.Size > usersub.MaxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: errSubtitleTooLarge})

		return nil, "", false
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to read upload"})

		return nil, "", false
	}
	defer file.Close() //nolint:errcheck // best-effort close

	raw, err := io.ReadAll(io.LimitReader(file, usersub.MaxUploadBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "failed to read upload"})

		return nil, "", false
	}
	if len(raw) > usersub.MaxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{Error: errSubtitleTooLarge})

		return nil, "", false
	}

	return raw, fileHeader.Filename, true
}

// getUserSubtitle serves the current user's uploaded WebVTT for a path.
//
//	@Summary		Get user subtitle
//	@Description	Serves the per-user uploaded WebVTT for a media path when present and not expired
//	@Tags			media
//	@Produce		text/vtt
//	@Param			path	path	string	true	"Path under media root"
//	@Success		200
//	@Failure		403	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Router			/api/user-subtitle/{path} [get]
func (h *handler) getUserSubtitle(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: catalogUnauthorized})

		return
	}
	if h.usersub == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: errUserSubtitleNotFound})

		return
	}

	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	relPath := access.NormalizeRelPath(pathParam)
	_, absPath, err := h.usersub.Get(user.ID, relPath)
	if errors.Is(err, usersub.ErrNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: errUserSubtitleNotFound})

		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errFailedLoadSubtitle})

		return
	}

	c.Header("Content-Type", "text/vtt; charset=utf-8")
	c.Header("Cache-Control", "private, max-age=3600")
	http.ServeFile(c.Writer, c.Request, absPath)
}

// deleteUserSubtitle removes the current user's uploaded caption for a path.
//
//	@Summary		Delete user subtitle
//	@Description	Removes the per-user uploaded subtitle for a media path
//	@Tags			media
//	@Param			path	path	string	true	"Path under media root"
//	@Success		204
//	@Failure		403	{object}	ErrorResponse
//	@Router			/api/user-subtitle/{path} [delete]
func (h *handler) deleteUserSubtitle(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: catalogUnauthorized})

		return
	}
	if h.usersub == nil {
		c.Status(http.StatusNoContent)

		return
	}

	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	relPath := access.NormalizeRelPath(pathParam)
	_ = h.usersub.Delete(user.ID, relPath)
	observability.LogAction(c, "media.userSubtitle.delete", slog.String("path", relPath))
	c.Status(http.StatusNoContent)
}

func (h *handler) userSubtitleTrack(c *gin.Context, relPath string) *UserSubtitleTrack {
	if h.usersub == nil {
		return nil
	}
	user, ok := currentUser(c)
	if !ok {
		return nil
	}

	track, _, err := h.usersub.Get(user.ID, access.NormalizeRelPath(relPath))
	if err != nil {
		return nil
	}

	return &UserSubtitleTrack{
		Lang:  track.Lang,
		Label: track.Label,
		URL:   userSubtitleURL(relPath),
	}
}

func userSubtitleURL(relPath string) string {
	return "/api/user-subtitle/" + mediafs.EscapePathSegments(strings.TrimPrefix(relPath, "/"))
}
