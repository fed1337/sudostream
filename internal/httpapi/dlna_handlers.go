package httpapi

import (
	"errors"
	"net/http"
	"sudoStream/internal/auth"
	"sudoStream/internal/dlna"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetDLNASettings returns DLNA Media Server settings.
//
//	@Summary	Get DLNA settings
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	dlna.Settings
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/admin/dlna/settings [get]
func (h *adminHandler) getDLNASettings(c *gin.Context) {
	if h.dlnaSettings == nil {
		c.JSON(http.StatusOK, dlna.DefaultSettings())

		return
	}

	settings, err := h.dlnaSettings.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load dlna settings failed"})

		return
	}

	c.JSON(http.StatusOK, settings)
}

// PatchDLNASettings updates DLNA enable flag and TV principal.
//
//	@Summary	Update DLNA settings
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dlna.Settings	true	"DLNA settings"
//	@Success	200		{object}	dlna.Settings
//	@Failure	400		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/admin/dlna/settings [patch]
func (h *adminHandler) patchDLNASettings( //nolint:cyclop,funlen // validate TV user + reload
	c *gin.Context,
) {
	if h.dlnaSettings == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "dlna settings unavailable"})

		return
	}

	var body dlna.Settings
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	existing, err := h.dlnaSettings.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load dlna settings failed"})

		return
	}

	if body.UDN == "" {
		body.UDN = existing.UDN
	}
	if body.UDN == "" {
		body.UDN = "uuid:" + uuid.NewString()
	}

	normalized, err := dlna.NormalizeSettings(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})

		return
	}

	if normalized.Enabled {
		user, userErr := h.auth.GetUserByID(c.Request.Context(), normalized.UserID)
		if userErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "tv user not found"})

			return
		}
		if user.Role != auth.RoleTV {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: dlna.ErrInvalidTVUser.Error()})

			return
		}
	}

	err = h.dlnaSettings.SaveSettings(c.Request.Context(), normalized)
	if err != nil {
		if errors.Is(err, dlna.ErrTVUserRequired) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})

			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "save dlna settings failed"})

		return
	}

	if h.dlnaController != nil {
		reloadErr := h.dlnaController.Reload(c.Request.Context())
		if reloadErr != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "dlna reload failed: " + reloadErr.Error()})

			return
		}
	}

	c.JSON(http.StatusOK, normalized)
}
