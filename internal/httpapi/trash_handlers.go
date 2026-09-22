package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sudoStream/internal/favorite"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/observability"
	"sudoStream/internal/thumbnail"
	"sudoStream/internal/transcode"
	"sudoStream/internal/trash"
	"sudoStream/internal/watch"

	"github.com/gin-gonic/gin"
)

const errTrashUnavailableMsg = "trash unavailable"

// TrashListResponse is GET /api/admin/trash.
type TrashListResponse struct {
	Items []trash.Item `json:"items"`
}

// TrashSettingsResponse is GET/PATCH /api/admin/trash/settings.
type TrashSettingsResponse struct {
	RetentionDays int `json:"retentionDays"`
}

type trashCleanup struct {
	media     *mediafs.Service
	metadata  *metadata.Service
	watch     *watch.Service
	favorite  *favorite.Service
	thumbs    *thumbnail.VideoThumbnailer
	transcode *transcode.Service
}

func (c trashCleanup) OnPermanentDelete(ctx context.Context, item trash.Item) error {
	if c.metadata != nil {
		err := c.metadata.DeleteForPath(ctx, item.OriginalRelPath)
		if err != nil {
			slog.Warn("trash metadata cleanup failed", "path", item.OriginalRelPath, "err", err)
		}
	}
	if c.watch != nil {
		err := c.watch.DeleteForPath(ctx, item.OriginalRelPath)
		if err != nil {
			slog.Warn("trash watch cleanup failed", "path", item.OriginalRelPath, "err", err)
		}
	}
	if c.favorite != nil {
		err := c.favorite.DeleteForPath(ctx, item.OriginalRelPath)
		if err != nil {
			slog.Warn("trash favorite cleanup failed", "path", item.OriginalRelPath, "err", err)
		}
	}
	c.removeCaches(item)

	return nil
}

func (c trashCleanup) removeCaches(item trash.Item) {
	if c.media == nil || (c.thumbs == nil && c.transcode == nil) {
		return
	}

	trashAbs, err := c.media.FilePath(item.TrashRelPath)
	if err != nil {
		return
	}
	info, err := os.Stat(trashAbs)
	if err != nil || info.IsDir() {
		return
	}

	if c.thumbs != nil {
		c.thumbs.Remove(item.OriginalRelPath, info.ModTime().Unix(), info.Size())
	}
	if c.transcode != nil {
		origAbs := filepath.Join(c.media.Root(), filepath.FromSlash(item.OriginalRelPath))
		cacheKey := c.transcode.CacheKey(origAbs, info.ModTime().Unix(), info.Size())
		c.transcode.RemoveCache(cacheKey)
	}
}

// ListTrash returns recycle-bin items.
//
//	@Summary	List deleted items
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	TrashListResponse
//	@Failure	503	{object}	ErrorResponse
//	@Router		/api/admin/trash [get]
func (h *adminHandler) listTrash(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	items, err := h.trash.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "list trash failed"})

		return
	}

	c.JSON(http.StatusOK, TrashListResponse{Items: items})
}

// RestoreTrash restores a deleted item to its original path.
//
//	@Summary	Restore a deleted item
//	@Tags		admin
//	@Produce	json
//	@Param		id	path		string	true	"Trash item id"
//	@Success	200	{object}	trash.Item
//	@Failure	404	{object}	ErrorResponse
//	@Failure	409	{object}	ErrorResponse
//	@Router		/api/admin/trash/{id}/restore [post]
func (h *adminHandler) restoreTrash(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	item, err := h.trash.Restore(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeTrashError(c, err)

		return
	}

	observability.LogAction(
		c,
		"trash.restore",
		slog.String("id", item.ID),
		slog.String("path", item.OriginalRelPath),
	)
	c.JSON(http.StatusOK, item)
}

// DeleteTrashForever permanently deletes one trash item.
//
//	@Summary	Delete a trash item forever
//	@Tags		admin
//	@Param		id	path	string	true	"Trash item id"
//	@Success	204
//	@Failure	404	{object}	ErrorResponse
//	@Router		/api/admin/trash/{id} [delete]
func (h *adminHandler) deleteTrashForever(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	itemID := c.Param("id")
	err := h.trash.DeleteForever(c.Request.Context(), itemID)
	if err != nil {
		h.writeTrashError(c, err)

		return
	}

	observability.LogAction(c, "trash.delete_forever", slog.String("id", itemID))
	c.Status(http.StatusNoContent)
}

// EmptyTrash permanently deletes every trash item.
//
//	@Summary	Empty recycle bin
//	@Tags		admin
//	@Success	204
//	@Failure	500	{object}	ErrorResponse
//	@Failure	503	{object}	ErrorResponse
//	@Router		/api/admin/trash [delete]
func (h *adminHandler) emptyTrash(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	n, err := h.trash.EmptyAll(c.Request.Context())
	if err != nil {
		h.writeTrashError(c, err)

		return
	}

	observability.LogAction(c, "trash.empty", slog.Int("count", n))
	c.Status(http.StatusNoContent)
}

// GetTrashSettings returns recycle_bin retention.
//
//	@Summary	Get recycle bin settings
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	TrashSettingsResponse
//	@Router		/api/admin/trash/settings [get]
func (h *adminHandler) getTrashSettings(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusOK, TrashSettingsResponse{RetentionDays: 0})

		return
	}

	settings, err := h.trash.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load trash settings failed"})

		return
	}

	c.JSON(http.StatusOK, TrashSettingsResponse{RetentionDays: settings.RetentionDays})
}

// PatchTrashSettings updates recycle_bin retention.
//
//	@Summary	Update recycle bin settings
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		TrashSettingsResponse	true	"Recycle bin settings"
//	@Success	200		{object}	TrashSettingsResponse
//	@Failure	400		{object}	ErrorResponse
//	@Router		/api/admin/trash/settings [patch]
func (h *adminHandler) patchTrashSettings(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	var body TrashSettingsResponse
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	settings, err := h.trash.SaveSettings(c.Request.Context(), trash.Settings{
		RetentionDays: body.RetentionDays,
	})
	if err != nil {
		if errors.Is(err, trash.ErrInvalidRetention) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid retentionDays"})

			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "save trash settings failed"})

		return
	}

	c.JSON(http.StatusOK, TrashSettingsResponse{RetentionDays: settings.RetentionDays})
}

func (h *adminHandler) writeTrashError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, trash.ErrNotFound), errors.Is(err, mediafs.ErrTrashItemMissing):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "trash item not found"})
	case errors.Is(err, mediafs.ErrRestoreConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "restore destination exists"})
	case errors.Is(err, mediafs.ErrDeleteReadOnly):
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "media volume is not writable"})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "trash operation failed"})
	}
}
