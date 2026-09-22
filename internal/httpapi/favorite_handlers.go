package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sudoStream/internal/favorite"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"

	"github.com/gin-gonic/gin"
)

// FavoriteResponse is the JSON body for favorite endpoints.
type FavoriteResponse struct {
	Favorited bool `json:"favorited"`
}

// FavoritePatchRequest updates favorite state for the current user.
type FavoritePatchRequest struct {
	Favorited bool `json:"favorited"`
}

// getFavorite returns favorite status for the current user.
//
//	@Summary		Get favorite status
//	@Description	Returns whether the current user has favorited a media file
//	@Tags			media
//	@Produce		json
//	@Param			path	path		string	true	"Media file path"
//	@Success		200		{object}	FavoriteResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/favorite/{path} [get]
func (h *handler) getFavorite(c *gin.Context) {
	if h.favorite == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "favorite unavailable"})

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

	state, err := h.favorite.Get(c.Request.Context(), user.ID, pathParam)
	if err != nil {
		h.handleFavoriteError(c, pathParam, err)

		return
	}

	c.JSON(http.StatusOK, FavoriteResponse{Favorited: state.Favorited})
}

// patchFavorite updates favorite status for the current user.
//
//	@Summary		Update favorite status
//	@Description	Marks or clears favorite for a media file for the current user
//	@Tags			media
//	@Accept			json
//	@Produce		json
//	@Param			path	path		string					true	"Media file path"
//	@Param			body	body		FavoritePatchRequest	true	"Favorite flag"
//	@Success		200		{object}	FavoriteResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/favorite/{path} [patch]
func (h *handler) patchFavorite(c *gin.Context) {
	if h.favorite == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "favorite unavailable"})

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

	var body FavoritePatchRequest
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	state, err := h.favorite.Set(c.Request.Context(), user.ID, pathParam, body.Favorited)
	if err != nil {
		h.handleFavoriteError(c, pathParam, err)

		return
	}

	observability.LogAction(
		c,
		"media.favorite",
		slog.String("path", pathParam),
		slog.Bool("favorited", body.Favorited),
	)

	c.JSON(http.StatusOK, FavoriteResponse{Favorited: state.Favorited})
}

func (h *handler) handleFavoriteError(c *gin.Context, pathParam string, err error) {
	switch {
	case errors.Is(err, favorite.ErrUnknownLibrary),
		errors.Is(err, mediafs.ErrInvalidPath),
		errors.Is(err, mediafs.ErrPathOutsideRoot):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidPathError})
	default:
		observability.LogAction(
			c,
			"media.favorite.error",
			slog.String("path", pathParam),
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "favorite update failed"})
	}
}
