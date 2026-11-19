package httpapi

import (
	"errors"
	"net/http"
	"sudoStream/internal/auth"

	"github.com/gin-gonic/gin"
)

// PlaybackPreferencesResponse is returned by GET /api/me/playback-preferences.
type PlaybackPreferencesResponse struct {
	AudioLanguages  []string `json:"audioLanguages"`
	AutoSkipIntro   bool     `json:"autoSkipIntro"`
	AutoNextEpisode bool     `json:"autoNextEpisode"`
}

// PatchPlaybackPreferencesRequest updates user playback preferences (partial).
type PatchPlaybackPreferencesRequest struct {
	AudioLanguages  *[]string `json:"audioLanguages"`
	AutoSkipIntro   *bool     `json:"autoSkipIntro"`
	AutoNextEpisode *bool     `json:"autoNextEpisode"`
}

// getPlaybackPreferences returns the current user's playback preferences.
//
//	@Summary		Get playback preferences
//	@Description	Returns audio language priority (max 3) and auto-skip / auto-next toggles.
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

	prefs, err := h.auth.GetPlaybackPreferences(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load playback preferences failed"})

		return
	}

	c.JSON(http.StatusOK, playbackPreferencesResponse(prefs))
}

// patchPlaybackPreferences updates playback preferences (partial merge).
//
//	@Summary		Update playback preferences
//	@Description	Sets audio language priority and/or auto-skip intro / auto-next episode toggles.
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

	existing, err := h.auth.GetPlaybackPreferences(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load playback preferences failed"})

		return
	}

	saved, err := h.auth.UpdatePlaybackPreferences(
		c.Request.Context(),
		user.ID,
		mergePlaybackPreferences(existing, body),
	)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidAudioLanguagePrefs) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "audio language list exceeds maximum of 3"})

			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "save playback preferences failed"})

		return
	}

	c.JSON(http.StatusOK, playbackPreferencesResponse(saved))
}

func mergePlaybackPreferences(
	existing auth.PlaybackPreferences,
	body PatchPlaybackPreferencesRequest,
) auth.PlaybackPreferences {
	merged := existing
	if body.AudioLanguages != nil {
		merged.AudioLanguages = *body.AudioLanguages
	}
	if body.AutoSkipIntro != nil {
		merged.AutoSkipIntro = *body.AutoSkipIntro
	}
	if body.AutoNextEpisode != nil {
		merged.AutoNextEpisode = *body.AutoNextEpisode
	}

	return merged
}

func playbackPreferencesResponse(prefs auth.PlaybackPreferences) PlaybackPreferencesResponse {
	langs := prefs.AudioLanguages
	if langs == nil {
		langs = []string{}
	}

	return PlaybackPreferencesResponse{
		AudioLanguages:  langs,
		AutoSkipIntro:   prefs.AutoSkipIntro,
		AutoNextEpisode: prefs.AutoNextEpisode,
	}
}
