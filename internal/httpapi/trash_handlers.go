package httpapi

import (
	"context"
	"errors"
	"fmt"
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
	"time"

	"github.com/gin-gonic/gin"
)

const (
	errTrashUnavailableMsg = "trash unavailable"
	maxTrashBulkIDs        = 100
)

// TrashItemView is one recycle-bin row for admin UI (enriched list fields).
type TrashItemView struct {
	ID              string    `json:"id"`
	OriginalRelPath string    `json:"originalRelPath"`
	TrashRelPath    string    `json:"trashRelPath"`
	LibraryID       string    `json:"libraryId,omitempty"`
	DeletedAt       time.Time `json:"deletedAt"`
	DeletedBy       string    `json:"deletedBy,omitempty"`
	DeletedByEmail  string    `json:"deletedByEmail,omitempty"`
	IsDir           bool      `json:"isDir"`
	Size            int64     `json:"size"`
}

// TrashListResponse is GET /api/admin/trash.
type TrashListResponse struct {
	Items []TrashItemView `json:"items"`
}

// TrashSettingsResponse is GET/PATCH /api/admin/trash/settings.
type TrashSettingsResponse struct {
	Enabled       bool `json:"enabled"`
	RetentionDays int  `json:"retentionDays"`
}

// TrashBulkRequest is POST /api/admin/trash/restore|purge.
type TrashBulkRequest struct {
	IDs []string `json:"ids"`
}

// TrashBulkFailure is one failed id in a bulk response.
type TrashBulkFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// TrashBulkResponse is the partial-success payload for bulk restore/purge.
type TrashBulkResponse struct {
	OK     []string           `json:"ok"`
	Failed []TrashBulkFailure `json:"failed"`
}

type trashCleanup struct {
	media     *mediafs.Service
	metadata  *metadata.Service
	watch     *watch.Service
	favorite  *favorite.Service
	thumbs    *thumbnail.VideoThumbnailer
	transcode *transcode.Service
}

func (c trashCleanup) OnSoftDelete(ctx context.Context, item trash.Item) error {
	if c.metadata == nil {
		return nil
	}

	err := c.metadata.MarkTrashedForPath(ctx, item.OriginalRelPath, item.DeletedAt)
	if err != nil {
		return fmt.Errorf("mark metadata trashed: %w", err)
	}

	return nil
}

func (c trashCleanup) OnRestore(ctx context.Context, item trash.Item) error {
	if c.metadata == nil {
		return nil
	}

	err := c.metadata.ClearTrashedForPath(ctx, item.OriginalRelPath)
	if err != nil {
		return fmt.Errorf("clear metadata trashed: %w", err)
	}

	return nil
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

// ListTrash returns recycle-bin items (admin only).
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

	emails := h.trashDeletedByEmails(c.Request.Context(), items)
	views := make([]TrashItemView, 0, len(items))
	for _, item := range items {
		views = append(views, h.toTrashItemView(item, emails[item.DeletedBy]))
	}

	c.JSON(http.StatusOK, TrashListResponse{Items: views})
}

// RestoreTrash restores a deleted item to its original path (admin only).
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

// RestoreTrashBulk restores selected trash items (admin only).
//
//	@Summary	Restore many trash items
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		TrashBulkRequest	true	"Trash item ids"
//	@Success	200		{object}	TrashBulkResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	503		{object}	ErrorResponse
//	@Router		/api/admin/trash/restore [post]
func (h *adminHandler) restoreTrashBulk(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	ids, ok := h.readTrashBulkIDs(c)
	if !ok {
		return
	}

	result := h.trash.RestoreMany(c.Request.Context(), ids)
	observability.LogAction(
		c,
		"trash.restore_many",
		slog.Int("ok", len(result.OK)),
		slog.Int("failed", len(result.Failed)),
	)
	c.JSON(http.StatusOK, toTrashBulkResponse(result))
}

// RestoreTrashAll restores every trash item (admin only).
//
//	@Summary	Restore all trash items
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	TrashBulkResponse
//	@Failure	500	{object}	ErrorResponse
//	@Failure	503	{object}	ErrorResponse
//	@Router		/api/admin/trash/restore-all [post]
func (h *adminHandler) restoreTrashAll(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	result, err := h.trash.RestoreAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "trash operation failed"})

		return
	}

	observability.LogAction(
		c,
		"trash.restore_all",
		slog.Int("ok", len(result.OK)),
		slog.Int("failed", len(result.Failed)),
	)
	c.JSON(http.StatusOK, toTrashBulkResponse(result))
}

// DeleteTrashForever permanently deletes one trash item (admin only).
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

// PurgeTrashBulk permanently deletes selected trash items (admin only).
//
//	@Summary	Delete many trash items forever
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		TrashBulkRequest	true	"Trash item ids"
//	@Success	200		{object}	TrashBulkResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	503		{object}	ErrorResponse
//	@Router		/api/admin/trash/purge [post]
func (h *adminHandler) purgeTrashBulk(c *gin.Context) {
	if h.trash == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: errTrashUnavailableMsg})

		return
	}

	ids, ok := h.readTrashBulkIDs(c)
	if !ok {
		return
	}

	result := h.trash.DeleteForeverMany(c.Request.Context(), ids)
	observability.LogAction(
		c,
		"trash.purge_many",
		slog.Int("ok", len(result.OK)),
		slog.Int("failed", len(result.Failed)),
	)
	c.JSON(http.StatusOK, toTrashBulkResponse(result))
}

// EmptyTrash permanently deletes every trash item (admin only).
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

// GetTrashSettings returns recycle_bin retention (admin only).
//
//	@Summary	Get recycle bin settings
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	TrashSettingsResponse
//	@Router		/api/admin/trash/settings [get]
func (h *adminHandler) getTrashSettings(c *gin.Context) {
	if h.trash == nil {
		defaults := trash.DefaultSettings()
		c.JSON(http.StatusOK, TrashSettingsResponse{
			Enabled:       defaults.Enabled,
			RetentionDays: defaults.RetentionDays,
		})

		return
	}

	settings, err := h.trash.GetSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "load trash settings failed"})

		return
	}

	c.JSON(http.StatusOK, TrashSettingsResponse{
		Enabled:       settings.Enabled,
		RetentionDays: settings.RetentionDays,
	})
}

// PatchTrashSettings updates recycle_bin retention (admin only).
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
		Enabled:       body.Enabled,
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

	c.JSON(http.StatusOK, TrashSettingsResponse{
		Enabled:       settings.Enabled,
		RetentionDays: settings.RetentionDays,
	})
}

func (h *adminHandler) readTrashBulkIDs(c *gin.Context) ([]string, bool) {
	var body TrashBulkRequest
	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return nil, false
	}
	if len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ids required"})

		return nil, false
	}
	if len(body.IDs) > maxTrashBulkIDs {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "too many ids"})

		return nil, false
	}

	return body.IDs, true
}

func (h *adminHandler) toTrashItemView(item trash.Item, email string) TrashItemView {
	view := TrashItemView{
		ID:              item.ID,
		OriginalRelPath: item.OriginalRelPath,
		TrashRelPath:    item.TrashRelPath,
		LibraryID:       item.LibraryID,
		DeletedAt:       item.DeletedAt,
		DeletedBy:       item.DeletedBy,
		DeletedByEmail:  email,
	}
	if h.media == nil {
		return view
	}

	fileAbs, err := h.media.FilePath(item.TrashRelPath)
	if err == nil {
		info, statErr := os.Stat(fileAbs)
		if statErr == nil {
			view.Size = info.Size()
		}

		return view
	}
	_, err = h.media.DirPath(item.TrashRelPath)
	if err == nil {
		view.IsDir = true
	}

	return view
}

func (h *adminHandler) trashDeletedByEmails(
	ctx context.Context,
	items []trash.Item,
) map[string]string {
	emails := make(map[string]string)
	if h.auth == nil || len(items) == 0 {
		return emails
	}

	need := make(map[string]struct{})
	for _, item := range items {
		if item.DeletedBy != "" {
			need[item.DeletedBy] = struct{}{}
		}
	}
	if len(need) == 0 {
		return emails
	}

	users, err := h.auth.ListUsers(ctx)
	if err != nil {
		slog.Warn("trash list resolve deletedBy emails failed", slog.String("error", err.Error()))

		return emails
	}
	for _, user := range users {
		if _, ok := need[user.ID]; ok {
			emails[user.ID] = user.Email
		}
	}

	return emails
}

func toTrashBulkResponse(result trash.BatchResult) TrashBulkResponse {
	failed := make([]TrashBulkFailure, 0, len(result.Failed))
	for _, failure := range result.Failed {
		failed = append(failed, TrashBulkFailure{
			ID:    failure.ID,
			Error: trashErrorMessage(failure.Err),
		})
	}
	ok := result.OK
	if ok == nil {
		ok = []string{}
	}

	return TrashBulkResponse{OK: ok, Failed: failed}
}

func trashErrorMessage(err error) string {
	switch {
	case errors.Is(err, trash.ErrNotFound), errors.Is(err, mediafs.ErrTrashItemMissing):
		return "trash item not found"
	case errors.Is(err, mediafs.ErrRestoreConflict):
		return "restore destination exists"
	case errors.Is(err, mediafs.ErrDeleteReadOnly):
		return "media volume is not writable"
	default:
		return "trash operation failed"
	}
}

func (h *adminHandler) writeTrashError(c *gin.Context, err error) {
	msg := trashErrorMessage(err)
	switch msg {
	case "trash item not found":
		c.JSON(http.StatusNotFound, ErrorResponse{Error: msg})
	case "restore destination exists":
		c.JSON(http.StatusConflict, ErrorResponse{Error: msg})
	case "media volume is not writable":
		c.JSON(http.StatusForbidden, ErrorResponse{Error: msg})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: msg})
	}
}
