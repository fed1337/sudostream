package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
	"sudoStream/internal/watch"

	"github.com/gin-gonic/gin"
)

// WatchResponse is the JSON body for watch endpoints.
type WatchResponse struct {
	Watched         bool     `json:"watched"`
	PositionSeconds *float64 `json:"positionSeconds,omitempty"`
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
}

// WatchPatchRequest updates watched state and/or playback progress for the current user.
type WatchPatchRequest struct {
	Watched         *bool    `json:"watched"`
	PositionSeconds *float64 `json:"positionSeconds"`
	DurationSeconds *float64 `json:"durationSeconds"`
}

// getWatch returns watched status for the current user.
//
//	@Summary		Get watched status
//	@Description	Returns whether the current user has marked a media file as watched
//	@Tags			media
//	@Produce		json
//	@Param			path	path		string	true	"Media file path"
//	@Success		200		{object}	WatchResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/watch/{path} [get]
func (h *handler) getWatch(c *gin.Context) {
	if h.watch == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "watch state unavailable"})

		return
	}

	pathParam := strings.TrimPrefix(c.Param("path"), "/")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	state, err := h.watch.Get(c.Request.Context(), user.ID, pathParam)
	if err != nil {
		h.handleWatchError(c, pathParam, err)

		return
	}

	c.JSON(http.StatusOK, watchResponse(state))
}

// patchWatch updates watched status for the current user.
//
//	@Summary		Update watched status
//	@Description	Marks watched state or saves playback progress for a media file
//	@Tags			media
//	@Accept			json
//	@Produce		json
//	@Param			path	path		string				true	"Media file path"
//	@Param			body	body		WatchPatchRequest	true	"Watched flag and optional progress"
//	@Success		200		{object}	WatchResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/watch/{path} [patch]
func (h *handler) patchWatch(c *gin.Context) {
	if h.watch == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "watch state unavailable"})

		return
	}

	pathParam := strings.TrimPrefix(c.Param("path"), "/")
	if !h.ensureReadAccess(c, pathParam) {
		return
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	var body WatchPatchRequest
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	if body.Watched == nil && body.PositionSeconds == nil && body.DurationSeconds == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	state, err := h.watch.Patch(c.Request.Context(), user.ID, pathParam, watch.Patch{
		Watched:         body.Watched,
		PositionSeconds: body.PositionSeconds,
		DurationSeconds: body.DurationSeconds,
	})
	if err != nil {
		h.handleWatchError(c, pathParam, err)

		return
	}

	if body.Watched != nil {
		observability.LogAction(
			c,
			"media.watch",
			slog.String("path", pathParam),
			slog.Bool("watched", *body.Watched),
		)
	}

	c.JSON(http.StatusOK, watchResponse(state))
}

func watchResponse(state watch.State) WatchResponse {
	return WatchResponse{
		Watched:         state.Watched,
		PositionSeconds: state.PositionSeconds,
		DurationSeconds: state.DurationSeconds,
	}
}

func (h *handler) handleWatchError(c *gin.Context, pathParam string, err error) {
	switch {
	case errors.Is(err, watch.ErrUnknownLibrary),
		errors.Is(err, mediafs.ErrInvalidPath):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidPathError})
	case errors.Is(err, mediafs.ErrPathOutsideRoot):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidPathError})
	default:
		observability.LogAction(
			c,
			"media.watch.error",
			slog.String("path", pathParam),
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "watch update failed"})
	}
}
