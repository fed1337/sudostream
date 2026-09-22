package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"sudoStream/internal/access"
	"sudoStream/internal/maintenance"

	"github.com/gin-gonic/gin"
)

const (
	errMaintenanceUnavailable     = "maintenance unavailable"
	errLoadLibraryFailed          = "load library failed"
	errLoadMaintenanceFailed      = "load maintenance failed"
	errLoadLibraryMaintFailed     = "load library maintenance failed"
	errSaveMaintenanceSchedFailed = "save maintenance schedules failed"
	errSaveLibrarySchedFailed     = "save library schedules failed"
)

// MaintenanceStatusResponse is the JSON body for maintenance status endpoints.
type MaintenanceStatusResponse struct {
	Actions    []maintenance.ActionStatus `json:"actions"`
	CacheBytes *int64                     `json:"cacheBytes,omitempty"`
}

// PatchMaintenanceRequest upserts one or more schedules.
type PatchMaintenanceRequest struct {
	Schedules []maintenance.ScheduleInput `json:"schedules"`
}

// MaintenanceRunResponse is returned from Run now.
type MaintenanceRunResponse struct {
	Run maintenance.Run `json:"run"`
}

// MaintenanceRunsResponse lists recent runs.
type MaintenanceRunsResponse struct {
	Runs []maintenance.Run `json:"runs"`
}

// GetMaintenance returns global maintenance action status.
//
//	@Summary		Get global maintenance
//	@Description	Schedules and latest runs for libraries.scan and playback.cache.purge.
//	@Tags			admin
//	@Produce		json
//	@Success		200	{object}	MaintenanceStatusResponse
//	@Failure		503	{object}	ErrorResponse
//	@Router			/api/admin/maintenance [get]
func (h *adminHandler) getMaintenance(c *gin.Context) {
	if h.maintenance == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMaintenanceUnavailable})

		return
	}

	actions, err := h.maintenance.GetGlobalMaintenance(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadMaintenanceFailed})

		return
	}

	cacheBytes := h.maintenance.CacheDirBytes()
	c.JSON(http.StatusOK, MaintenanceStatusResponse{
		Actions:    actions,
		CacheBytes: &cacheBytes,
	})
}

// PatchMaintenance updates global schedules.
//
//	@Summary	Patch global maintenance schedules
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		PatchMaintenanceRequest	true	"Schedules"
//	@Success	200		{object}	MaintenanceStatusResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	503		{object}	ErrorResponse
//	@Router		/api/admin/maintenance [patch]
func (h *adminHandler) patchMaintenance(c *gin.Context) {
	if h.maintenance == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMaintenanceUnavailable})

		return
	}

	var body PatchMaintenanceRequest
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	err = h.maintenance.PatchGlobalSchedules(c.Request.Context(), body.Schedules)
	if err != nil {
		if errors.Is(err, maintenance.ErrInvalidCron) ||
			errors.Is(err, maintenance.ErrInvalidAction) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})

			return
		}
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: errSaveMaintenanceSchedFailed},
		)

		return
	}

	actions, err := h.maintenance.GetGlobalMaintenance(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadMaintenanceFailed})

		return
	}

	cacheBytes := h.maintenance.CacheDirBytes()
	c.JSON(http.StatusOK, MaintenanceStatusResponse{
		Actions:    actions,
		CacheBytes: &cacheBytes,
	})
}

// GetLibraryMaintenance returns per-library maintenance status.
//
//	@Summary	Get library maintenance
//	@Tags		admin
//	@Produce	json
//	@Param		id	path		string	true	"Library ID"
//	@Success	200	{object}	MaintenanceStatusResponse
//	@Failure	404	{object}	ErrorResponse
//	@Failure	503	{object}	ErrorResponse
//	@Router		/api/admin/libraries/{id}/maintenance [get]
func (h *adminHandler) getLibraryMaintenance(c *gin.Context) {
	if h.maintenance == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMaintenanceUnavailable})

		return
	}
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	libraryID := c.Param("id")
	_, err := h.access.GetLibrary(c.Request.Context(), libraryID)
	if err != nil {
		if errors.Is(err, access.ErrLibraryNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: libraryNotFoundMessage})

			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadLibraryFailed})

		return
	}

	actions, err := h.maintenance.GetLibraryMaintenance(c.Request.Context(), libraryID)
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: errLoadLibraryMaintFailed},
		)

		return
	}

	c.JSON(http.StatusOK, MaintenanceStatusResponse{Actions: actions})
}

// PatchLibraryMaintenance updates per-library schedules.
//
//	@Summary	Patch library maintenance schedules
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		id		path		string					true	"Library ID"
//	@Param		body	body		PatchMaintenanceRequest	true	"Schedules"
//	@Success	200		{object}	MaintenanceStatusResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	404		{object}	ErrorResponse
//	@Router		/api/admin/libraries/{id}/maintenance [patch]
func (h *adminHandler) patchLibraryMaintenance(c *gin.Context) {
	if h.maintenance == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMaintenanceUnavailable})

		return
	}
	if h.access == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errAccessControlUnavailable})

		return
	}

	libraryID := c.Param("id")
	_, err := h.access.GetLibrary(c.Request.Context(), libraryID)
	if err != nil {
		if errors.Is(err, access.ErrLibraryNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: libraryNotFoundMessage})

			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errLoadLibraryFailed})

		return
	}

	var body PatchMaintenanceRequest
	err = c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	err = h.maintenance.PatchLibrarySchedules(c.Request.Context(), libraryID, body.Schedules)
	if err != nil {
		if errors.Is(err, maintenance.ErrInvalidCron) ||
			errors.Is(err, maintenance.ErrInvalidAction) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})

			return
		}
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: errSaveLibrarySchedFailed},
		)

		return
	}

	actions, err := h.maintenance.GetLibraryMaintenance(c.Request.Context(), libraryID)
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			ErrorResponse{Error: errLoadLibraryMaintFailed},
		)

		return
	}

	c.JSON(http.StatusOK, MaintenanceStatusResponse{Actions: actions})
}

// RunMaintenance starts a maintenance action now.
//
//	@Summary	Run maintenance action
//	@Tags		admin
//	@Produce	json
//	@Param		action		path		string	true	"Action key"
//	@Param		libraryId	query		string	false	"Library ID (required for per-library actions)"
//	@Success	200			{object}	MaintenanceRunResponse
//	@Failure	400			{object}	ErrorResponse
//	@Failure	409			{object}	ErrorResponse
//	@Router		/api/admin/maintenance/{action}/run [post]
func (h *adminHandler) runMaintenance(c *gin.Context) {
	if h.maintenance == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMaintenanceUnavailable})

		return
	}

	action := c.Param("action")
	libraryID := c.Query("libraryId")
	run, err := h.maintenance.RunNow(
		c.Request.Context(),
		action,
		libraryID,
		maintenance.TriggerManual,
	)
	if err != nil {
		switch {
		case errors.Is(err, maintenance.ErrConflict):
			c.JSON(http.StatusConflict, ErrorResponse{Error: "action already running"})
		case errors.Is(err, maintenance.ErrLibraryRequired),
			errors.Is(err, maintenance.ErrInvalidAction):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "run failed to start"})
		}

		return
	}

	c.JSON(http.StatusOK, MaintenanceRunResponse{Run: run})
}

// ListMaintenanceRuns returns recent runs.
//
//	@Summary	List maintenance runs
//	@Tags		admin
//	@Produce	json
//	@Param		limit	query		int	false	"Max rows (default 50)"
//	@Success	200		{object}	MaintenanceRunsResponse
//	@Router		/api/admin/maintenance/runs [get]
func (h *adminHandler) listMaintenanceRuns(c *gin.Context) {
	if h.maintenance == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errMaintenanceUnavailable})

		return
	}

	limit := 50
	if raw := c.Query("limit"); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}

	runs, err := h.maintenance.ListRuns(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "list runs failed"})

		return
	}

	c.JSON(http.StatusOK, MaintenanceRunsResponse{Runs: runs})
}
