package httpapi

import (
	"net/http"
	"sudoStream/internal/version"

	"github.com/gin-gonic/gin"
)

// HealthResponse describes service health and release metadata.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Healthcheck reports service readiness and the current release version.
//
//	@Summary		Healthcheck
//	@Description	Returns service health and the current application version.
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	HealthResponse
//	@Failure		503	{object}	HealthResponse
//	@Router			/api/health [get]
func (h *handler) health(c *gin.Context) {
	response := HealthResponse{
		Status:  "ok",
		Version: version.Version,
	}

	err := h.media.Health()
	if err != nil {
		response.Status = healthStatusDegraded
		c.JSON(http.StatusServiceUnavailable, response)

		return
	}

	if h.dbPing != nil {
		err = h.dbPing(c.Request.Context())
		if err != nil {
			response.Status = healthStatusDegraded
			c.JSON(http.StatusServiceUnavailable, response)

			return
		}
	}

	c.JSON(http.StatusOK, response)
}
