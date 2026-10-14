package httpapi

import (
	"errors"
	"net/http"
	"sudoStream/internal/auth"

	"github.com/gin-gonic/gin"
)

// PlaybackPreferencesResponse is returned by GET /api/me/playback-preferences.
type PlaybackPreferencesResponse struct {
	AudioLanguages []string `json:"audioLanguages"`
}

// PatchPlaybackPreferencesRequest updates user audio language priority.
type PatchPlaybackPreferencesRequest struct {
	AudioLanguages []string `json:"audioLanguages"`
}

// getPlaybackPreferences returns the current user's audio language priority list.
//
//	@Summary		Get playback preferences
//	@Description	Returns ordered audio language codes for default track selection (max 3).
//	@Tags			me
//	@Produce		json
//	@Success		200	{object}	PlaybackPreferencesResponse
//	@Failure		401	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/me/playback-preferences [get]
func (h *handler) getPlaybackPreferences(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	if h.auth == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "auth unavailable"})

		return
	}

	langs, err := h.auth.PlaybackAudioLanguages(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load playback preferences failed"})

		return
	}

	if langs == nil {
		langs = []string{}
	}

	c.JSON(http.StatusOK, PlaybackPreferencesResponse{AudioLanguages: langs})
}

// patchPlaybackPreferences stores up to three ordered audio language codes.
//
//	@Summary		Update playback preferences
//	@Description	Sets ordered audio language codes for default track selection (max 3).
//	@Tags			me
//	@Accept			json
//	@Produce		json
//	@Param			body	body		PatchPlaybackPreferencesRequest	true	"Playback preferences"
//	@Success		200		{object}	PlaybackPreferencesResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/me/playback-preferences [patch]
func (h *handler) patchPlaybackPreferences(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	if h.auth == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "auth unavailable"})

		return
	}

	var body PatchPlaybackPreferencesRequest
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	saved, err := h.auth.UpdatePlaybackPreferences(
		c.Request.Context(),
		user.ID,
		auth.PlaybackPreferences{AudioLanguages: body.AudioLanguages},
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidAudioLanguagePrefs):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "audio language list exceeds maximum of 3"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "save playback preferences failed"})
		}

		return
	}

	c.JSON(http.StatusOK, PlaybackPreferencesResponse{AudioLanguages: saved.AudioLanguages})
}
