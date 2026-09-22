package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/observability"

	"github.com/gin-gonic/gin"
)

const errMetadataUnavailable = "metadata unavailable"

// GetMetadata returns cached metadata for a video file.
//
//	@Summary		Get video metadata
//	@Description	Returns cached original and override metadata for a video path.
//	@Tags			media
//	@Produce		json
//	@Param			path	path		string	true	"Path under media root"
//	@Success		200		{object}	metadata.MetadataResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/metadata/{path} [get]
func (h *handler) getMetadata(c *gin.Context) {
	if h.metadata == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMetadataUnavailable})

		return
	}

	pathParam := c.Param("path")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	response, err := h.metadata.Get(c.Request.Context(), pathParam)
	if err != nil {
		h.handleMetadataError(c, pathParam, err, "metadata.read")

		return
	}

	observability.LogAction(
		c,
		"metadata.read",
		slog.String("path", pathParam),
		slog.String("library_type", string(response.LibraryType)),
	)

	c.JSON(http.StatusOK, response)
}

// PatchMetadata updates override metadata for a video file.
//
//	@Summary		Patch video metadata
//	@Description	Updates per-file override metadata or writes embedded file tags via ffmpeg.
//	@Tags			media
//	@Accept			json
//	@Produce		json
//	@Param			path	path		string					true	"Path under media root"
//	@Param			body	body		metadata.PatchRequest	true	"Metadata patch"
//	@Success		200		{object}	metadata.MetadataResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		422		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/metadata/{path} [patch]
func (h *handler) patchMetadata(c *gin.Context) {
	if h.metadata == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMetadataUnavailable})

		return
	}

	pathParam := c.Param("path")
	if !h.ensureUpdateAccess(c, pathParam) {
		return
	}

	var request metadata.PatchRequest
	err := c.ShouldBindJSON(&request)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "authentication required"})

		return
	}

	response, err := h.metadata.Patch(c.Request.Context(), pathParam, request, user.ID)
	if err != nil {
		h.handleMetadataError(c, pathParam, err, "metadata.write")

		return
	}

	observability.LogAction(
		c,
		"metadata.write",
		slog.String("path", pathParam),
		slog.String("target", string(request.Target)),
		slog.String("user_id", user.ID),
	)

	c.JSON(http.StatusOK, response)
}

func (h *handler) ensureUpdateAccess(c *gin.Context, rawPath string) bool {
	return h.ensurePermission(c, rawPath, h.access.CanUpdate)
}

func (h *handler) handleMetadataError(c *gin.Context, pathParam string, err error, action string) {
	switch {
	case errors.Is(err, metadata.ErrNotVideo):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "path is not a supported video file"})
	case errors.Is(err, mediafs.ErrInvalidPath), errors.Is(err, mediafs.ErrPathOutsideRoot):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidPathError})
	case errors.Is(err, metadata.ErrFileTargetUnsupported):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	case errors.Is(err, metadata.ErrFileWriteFailed):
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: err.Error()})
	case errors.Is(err, metadata.ErrInvalidTarget), errors.Is(err, metadata.ErrInvalidPatchValue):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		observability.LogAction(
			c,
			action+".error",
			slog.String("path", pathParam),
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "metadata request failed"})
	}
}
