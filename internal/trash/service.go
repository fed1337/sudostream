// Package trash implements the /media/.trash recycle bin.
package trash

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"time"

	"github.com/google/uuid"
)

// Service moves media into /media/.trash and restores or purges it.
type Service struct {
	media   *mediafs.Service
	store   Store
	access  *access.Service
	kv      SettingsKV
	cleaner Cleaner
}

// NewService constructs a recycle-bin service.
func NewService(
	media *mediafs.Service,
	store Store,
	accessService *access.Service,
	kv SettingsKV,
) *Service {
	return &Service{
		media:  media,
		store:  store,
		access: accessService,
		kv:     kv,
	}
}

// SetCleaner wires metadata/cache cleanup used on permanent delete and purge.
func (s *Service) SetCleaner(cleaner Cleaner) {
	if s == nil {
		return
	}
	s.cleaner = cleaner
}

// Move relocates a media path into trash and records the item.
func (s *Service) Move(ctx context.Context, rawPath, deletedBy string) (Item, error) {
	if s == nil || s.media == nil || s.store == nil {
		return Item{}, ErrUnavailable
	}

	itemID, err := uuid.NewRandom()
	if err != nil {
		return Item{}, fmt.Errorf("trash id: %w", err)
	}

	originalRel, trashRel, err := s.media.MoveToTrash(rawPath, itemID.String())
	if err != nil {
		return Item{}, fmt.Errorf("move to trash: %w", err)
	}

	item := Item{
		ID:              itemID.String(),
		OriginalRelPath: originalRel,
		TrashRelPath:    trashRel,
		DeletedAt:       time.Now().UTC(),
		DeletedBy:       deletedBy,
	}
	s.attachLibrary(ctx, &item)

	info := mediafs.TrashInfo{
		ID:              item.ID,
		OriginalRelPath: item.OriginalRelPath,
		TrashRelPath:    item.TrashRelPath,
		LibraryID:       item.LibraryID,
		DeletedAt:       item.DeletedAt.Format(time.RFC3339Nano),
		DeletedBy:       item.DeletedBy,
	}
	err = s.media.WriteTrashInfo(item.ID, info)
	if err != nil {
		slog.Error("write trash info.json failed", "id", item.ID, "err", err)
	}

	err = s.store.Insert(ctx, item)
	if err != nil {
		_ = s.media.RestoreFromTrash(item.OriginalRelPath, item.TrashRelPath, item.ID)

		return Item{}, fmt.Errorf("insert trash item: %w", err)
	}

	slog.Info("media moved to trash",
		slog.String("action", "trash.move"),
		slog.String("id", item.ID),
		slog.String("path", item.OriginalRelPath),
	)

	return item, nil
}

// List returns recycle-bin items, recovering rows from info.json when needed.
func (s *Service) List(ctx context.Context) ([]Item, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}

	s.reconcileDisk(ctx)

	items, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list trash: %w", err)
	}

	return items, nil
}

// Restore moves an item back to its original path.
func (s *Service) Restore(ctx context.Context, itemID string) (Item, error) {
	if s == nil || s.media == nil || s.store == nil {
		return Item{}, ErrUnavailable
	}

	item, err := s.store.Get(ctx, itemID)
	if err != nil {
		return Item{}, fmt.Errorf("get trash item: %w", err)
	}

	err = s.media.RestoreFromTrash(item.OriginalRelPath, item.TrashRelPath, item.ID)
	if err != nil {
		return Item{}, fmt.Errorf("restore from trash: %w", err)
	}

	err = s.store.Delete(ctx, item.ID)
	if err != nil {
		return Item{}, fmt.Errorf("delete trash row: %w", err)
	}

	slog.Info("media restored from trash",
		slog.String("action", "trash.restore"),
		slog.String("id", item.ID),
		slog.String("path", item.OriginalRelPath),
	)

	return item, nil
}

// DeleteForever removes the trash tree and runs cleaners for the original path.
func (s *Service) DeleteForever(ctx context.Context, itemID string) error {
	if s == nil || s.media == nil || s.store == nil {
		return ErrUnavailable
	}

	item, err := s.store.Get(ctx, itemID)
	if err != nil {
		return fmt.Errorf("get trash item: %w", err)
	}

	if s.cleaner != nil {
		cleanErr := s.cleaner.OnPermanentDelete(ctx, item)
		if cleanErr != nil {
			slog.Warn("trash permanent cleanup failed",
				slog.String("id", item.ID),
				slog.String("error", cleanErr.Error()),
			)
		}
	}

	err = s.media.RemoveTrashItem(item.ID)
	if err != nil {
		return fmt.Errorf("remove trash item: %w", err)
	}

	err = s.store.Delete(ctx, item.ID)
	if err != nil {
		return fmt.Errorf("delete trash row: %w", err)
	}

	slog.Info("trash item deleted forever",
		slog.String("action", "trash.delete_forever"),
		slog.String("id", item.ID),
		slog.String("path", item.OriginalRelPath),
	)

	return nil
}

// EmptyAll permanently deletes every trash item.
func (s *Service) EmptyAll(ctx context.Context) (int, error) {
	items, err := s.List(ctx)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for _, item := range items {
		delErr := s.DeleteForever(ctx, item.ID)
		if delErr != nil {
			return deleted, delErr
		}
		deleted++
	}

	return deleted, nil
}

// OriginalPrefixes returns original rel paths of trash items (indexer freeze).
func (s *Service) OriginalPrefixes(ctx context.Context) ([]string, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}

	prefixes, err := s.store.OriginalPrefixes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list trash prefixes: %w", err)
	}

	return prefixes, nil
}

// PurgeExpired permanently deletes items older than retentionDays. No-op when 0.
func (s *Service) PurgeExpired(ctx context.Context) (int, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return 0, err
	}
	if settings.RetentionDays <= 0 {
		return 0, nil
	}

	items, err := s.List(ctx)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -settings.RetentionDays)
	purged := 0
	for _, item := range items {
		if item.DeletedAt.After(cutoff) {
			continue
		}
		delErr := s.DeleteForever(ctx, item.ID)
		if delErr != nil {
			return purged, delErr
		}
		purged++
	}

	return purged, nil
}

// GetSettings returns recycle_bin KV or defaults.
func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	if s == nil || s.kv == nil {
		return DefaultSettings(), nil
	}

	raw, err := s.kv.GetSettingValue(ctx, settingsKey)
	if err != nil {
		//nolint:nilerr // missing row is the normal first-run case
		return DefaultSettings(), nil
	}

	var settings Settings
	err = json.Unmarshal(raw, &settings)
	if err != nil {
		//nolint:nilerr // corrupt value falls back to safe defaults
		return DefaultSettings(), nil
	}

	return NormalizeSettings(settings)
}

// SaveSettings validates and upserts recycle_bin settings.
func (s *Service) SaveSettings(ctx context.Context, settings Settings) (Settings, error) {
	if s == nil || s.kv == nil {
		return Settings{}, ErrSettingsUnavailable
	}

	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return Settings{}, err
	}

	raw, err := json.Marshal(normalized)
	if err != nil {
		return Settings{}, fmt.Errorf("encode trash settings: %w", err)
	}

	err = s.kv.SaveSettingValue(ctx, settingsKey, raw)
	if err != nil {
		return Settings{}, fmt.Errorf("save trash settings: %w", err)
	}

	return normalized, nil
}

// NormalizeSettings clamps retentionDays to [0, maxRetentionDays].
func NormalizeSettings(settings Settings) (Settings, error) {
	if settings.RetentionDays < 0 || settings.RetentionDays > maxRetentionDays {
		return Settings{}, ErrInvalidRetention
	}

	return settings, nil
}

func (s *Service) attachLibrary(ctx context.Context, item *Item) {
	if s.access == nil {
		return
	}

	library, ok, lookupErr := s.access.LibraryForRelPath(ctx, item.OriginalRelPath)
	if lookupErr != nil {
		slog.Warn("trash library lookup failed", "path", item.OriginalRelPath, "err", lookupErr)

		return
	}
	if ok {
		item.LibraryID = library.ID
	}
}

func (s *Service) reconcileDisk(ctx context.Context) {
	if s.media == nil {
		return
	}

	ids, err := s.media.ListTrashItemIDs()
	if err != nil {
		slog.Warn("list trash dirs failed", "err", err)

		return
	}

	for _, itemID := range ids {
		_, getErr := s.store.Get(ctx, itemID)
		if getErr == nil {
			continue
		}
		info, readErr := s.media.ReadTrashInfo(itemID)
		if readErr != nil {
			continue
		}
		item := itemFromInfo(info)
		insErr := s.store.Insert(ctx, item)
		if insErr != nil {
			slog.Warn("recover trash row failed", "id", itemID, "err", insErr)
		}
	}
}

func itemFromInfo(info mediafs.TrashInfo) Item {
	item := Item{
		ID:              info.ID,
		OriginalRelPath: info.OriginalRelPath,
		TrashRelPath:    info.TrashRelPath,
		LibraryID:       info.LibraryID,
		DeletedBy:       info.DeletedBy,
	}
	if info.DeletedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, info.DeletedAt)
		if err == nil {
			item.DeletedAt = parsed
		}
	}
	if item.DeletedAt.IsZero() {
		item.DeletedAt = time.Now().UTC()
	}

	return item
}
