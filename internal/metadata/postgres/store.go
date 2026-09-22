// Package postgres persists cached video metadata.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sudoStream/internal/metadata"
	"sudoStream/internal/watch"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store persists metadata rows in PostgreSQL.
type Store struct {
	db       *gorm.DB
	pgTrgm   bool
	unaccent bool
	capsOnce sync.Once
}

// NewStore constructs a metadata store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Get returns a metadata row for a library path.
func (s *Store) Get(ctx context.Context, libraryID, relPath string) (metadata.Row, error) {
	var model metadataModel

	err := s.db.WithContext(ctx).
		Where("library_id = ? AND rel_path = ?", libraryID, relPath).
		First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return metadata.Row{}, metadata.ErrNotFound
		}

		return metadata.Row{}, fmt.Errorf("get metadata: %w", err)
	}

	return modelToRow(model)
}

// UpsertOriginal inserts or updates cached original metadata for a file.
func (s *Store) UpsertOriginal(
	ctx context.Context,
	libraryID, relPath string,
	original metadata.VideoFields,
	fileMtime time.Time,
	fileSize int64,
	probedAt time.Time,
) error {
	originalRaw, err := encodeFields(original)
	if err != nil {
		return fmt.Errorf("marshal original fields: %w", err)
	}

	overrideRaw, err := encodeFields(nil)
	if err != nil {
		return fmt.Errorf("marshal override fields: %w", err)
	}

	model := metadataModel{
		LibraryID:   libraryID,
		RelPath:     relPath,
		OriginalRaw: originalRaw,
		OverrideRaw: overrideRaw,
		FileMtime:   &fileMtime,
		FileSize:    &fileSize,
		ProbedAt:    &probedAt,
	}

	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "library_id"}, {Name: "rel_path"}},
		DoUpdates: clause.Assignments(map[string]any{
			"original_fields": originalRaw,
			"file_mtime":      fileMtime,
			"file_size":       fileSize,
			"probed_at":       probedAt,
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("upsert original metadata: %w", err)
	}

	return s.refreshSearchDocument(ctx, libraryID, relPath)
}

// UpdateOverride replaces override fields for a file and records the actor.
func (s *Store) UpdateOverride(
	ctx context.Context,
	libraryID, relPath string,
	override metadata.StoredOverride,
	updatedAt time.Time,
	overriddenBy string,
) error {
	overrideRaw, err := encodeFields(override)
	if err != nil {
		return fmt.Errorf("marshal override fields: %w", err)
	}

	originalRaw, err := encodeFields(nil)
	if err != nil {
		return fmt.Errorf("marshal original fields: %w", err)
	}

	model := metadataModel{
		LibraryID:    libraryID,
		RelPath:      relPath,
		OriginalRaw:  originalRaw,
		OverrideRaw:  overrideRaw,
		OverrideAt:   &updatedAt,
		OverriddenBy: &overriddenBy,
	}

	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "library_id"}, {Name: "rel_path"}},
		DoUpdates: clause.Assignments(map[string]any{
			"override_fields":     overrideRaw,
			"override_updated_at": updatedAt,
			"overridden_by":       overriddenBy,
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("update override metadata: %w", err)
	}

	return s.refreshSearchDocument(ctx, libraryID, relPath)
}

// ListIndexedPaths returns rel_paths currently indexed for a library (including trash originals).
func (s *Store) ListIndexedPaths(ctx context.Context, libraryID string) ([]string, error) {
	var paths []string

	err := s.db.WithContext(ctx).
		Model(&metadataModel{}).
		Where("library_id = ?", libraryID).
		Pluck("rel_path", &paths).Error
	if err != nil {
		return nil, fmt.Errorf("list indexed paths: %w", err)
	}

	return paths, nil
}

// ListActiveIndexedPaths returns indexed rel_paths for a library, excluding recycle-bin items.
func (s *Store) ListActiveIndexedPaths(ctx context.Context, libraryID string) ([]string, error) {
	var paths []string

	err := s.db.WithContext(ctx).
		Model(&metadataModel{}).
		Where("library_id = ?", libraryID).
		Where("rel_path <> ? AND rel_path NOT LIKE ?", ".trash", ".trash/%").
		Where(`NOT EXISTS (
			SELECT 1 FROM trash_items t
			WHERE media_metadata.rel_path = t.original_rel_path
			   OR media_metadata.rel_path LIKE t.original_rel_path || '/%'
		)`).
		Order("rel_path ASC").
		Pluck("rel_path", &paths).Error
	if err != nil {
		return nil, fmt.Errorf("list active indexed paths: %w", err)
	}

	return paths, nil
}

// CountByLibraries returns indexed media counts by library.
func (s *Store) CountByLibraries(
	ctx context.Context,
	libraryIDs []string,
) (map[string]int64, error) {
	counts := make(map[string]int64, len(libraryIDs))
	if len(libraryIDs) == 0 {
		return counts, nil
	}

	type countRow struct {
		LibraryID string
		Count     int64
	}

	var rows []countRow
	err := s.db.WithContext(ctx).
		Model(&metadataModel{}).
		Select("library_id, COUNT(*) AS count").
		Where("library_id IN ?", libraryIDs).
		Group("library_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count indexed metadata: %w", err)
	}

	for _, row := range rows {
		counts[row.LibraryID] = row.Count
	}

	return counts, nil
}

// ListUnwatched returns indexed media that is neither completed nor in-progress.
func (s *Store) ListUnwatched(
	ctx context.Context,
	userID string,
	libraryIDs []string,
	limit, offset int,
) ([]metadata.ShelfRow, error) {
	if len(libraryIDs) == 0 || limit <= 0 {
		return []metadata.ShelfRow{}, nil
	}
	if offset < 0 {
		offset = 0
	}

	var rows []metadata.ShelfRow
	err := s.unwatchedQuery(ctx, userID, libraryIDs).
		Select(
			`metadata.library_id, metadata.rel_path,
			COALESCE(
				NULLIF(metadata.override_fields->>'title', ''),
				NULLIF(metadata.original_fields->>'title', ''),
				''
			) AS title`,
		).
		Order("metadata.probed_at DESC NULLS LAST, metadata.rel_path ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list unwatched metadata: %w", err)
	}

	return rows, nil
}

// CountUnwatched returns indexed media that is neither completed nor in-progress.
func (s *Store) CountUnwatched(
	ctx context.Context,
	userID string,
	libraryIDs []string,
) (int64, error) {
	if len(libraryIDs) == 0 {
		return 0, nil
	}

	var total int64
	err := s.unwatchedQuery(ctx, userID, libraryIDs).Count(&total).Error
	if err != nil {
		return 0, fmt.Errorf("count unwatched metadata: %w", err)
	}

	return total, nil
}

// ListShelfItems returns titles for the requested media paths.
func (s *Store) ListShelfItems(
	ctx context.Context,
	libraryIDs []string,
	relPaths []string,
) ([]metadata.ShelfRow, error) {
	if len(libraryIDs) == 0 || len(relPaths) == 0 {
		return []metadata.ShelfRow{}, nil
	}

	var rows []metadata.ShelfRow
	err := s.db.WithContext(ctx).
		Model(&metadataModel{}).
		Select(
			`library_id, rel_path,
			COALESCE(
				NULLIF(override_fields->>'title', ''),
				NULLIF(original_fields->>'title', ''),
				''
			) AS title`,
		).
		Where("library_id IN ? AND rel_path IN ?", libraryIDs, relPaths).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list shelf items: %w", err)
	}

	return rows, nil
}

// DeletePath removes a metadata row.
func (s *Store) DeletePath(ctx context.Context, libraryID, relPath string) error {
	err := s.db.WithContext(ctx).
		Where("library_id = ? AND (rel_path = ? OR rel_path LIKE ?)", libraryID, relPath, relPath+"/%").
		Delete(&metadataModel{}).Error
	if err != nil {
		return fmt.Errorf("delete metadata path: %w", err)
	}

	return nil
}

// NeedsProbe reports whether file stat differs from the cached row or identity
// hashes are still missing (backfill without waiting for mtime change).
func (s *Store) NeedsProbe(
	ctx context.Context,
	libraryID, relPath string,
	fileMtime time.Time,
	fileSize int64,
) (bool, error) {
	var model metadataModel

	err := s.db.WithContext(ctx).
		Select("file_mtime", "file_size", "original_fields").
		Where("library_id = ? AND rel_path = ?", libraryID, relPath).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("load cached file stat: %w", err)
	}

	if model.FileMtime == nil || model.FileSize == nil {
		return true, nil
	}

	if !model.FileMtime.Equal(fileMtime) || *model.FileSize != fileSize {
		return true, nil
	}

	original, err := decodeFields(model.OriginalRaw)
	if err != nil {
		return false, err
	}

	return metadata.IdentityHashesIncomplete(original, fileSize), nil
}

func (s *Store) unwatchedQuery(ctx context.Context, userID string, libraryIDs []string) *gorm.DB {
	return s.db.WithContext(ctx).
		Table("media_metadata AS metadata").
		Joins(
			"LEFT JOIN watch_state AS watch ON watch.library_id = metadata.library_id "+
				"AND watch.rel_path = metadata.rel_path AND watch.user_id = ?",
			userID,
		).
		Where(
			"metadata.library_id IN ? AND (watch.user_id IS NULL OR "+
				"(watch.watched = FALSE AND COALESCE(watch.position_seconds, 0) < ?))",
			libraryIDs,
			watch.MinProgressSeconds,
		)
}

func modelToRow(model metadataModel) (metadata.Row, error) {
	row := metadata.Row{
		LibraryID:    model.LibraryID,
		RelPath:      model.RelPath,
		FileMtime:    model.FileMtime,
		FileSize:     model.FileSize,
		ProbedAt:     model.ProbedAt,
		OverrideAt:   model.OverrideAt,
		OverriddenBy: model.OverriddenBy,
	}

	var err error

	row.Original, err = decodeFields(model.OriginalRaw)
	if err != nil {
		return metadata.Row{}, err
	}

	row.Override, err = decodeOverride(model.OverrideRaw)
	if err != nil {
		return metadata.Row{}, err
	}

	return row, nil
}

func decodeFields(raw []byte) (metadata.VideoFields, error) {
	if len(raw) == 0 {
		return metadata.VideoFields{}, nil
	}

	var fields metadata.VideoFields
	err := json.Unmarshal(raw, &fields)
	if err != nil {
		return metadata.VideoFields{}, fmt.Errorf("decode metadata fields: %w", err)
	}

	return fields, nil
}

func decodeOverride(raw []byte) (metadata.StoredOverride, error) {
	if len(raw) == 0 {
		return metadata.StoredOverride{}, nil
	}

	var override metadata.StoredOverride
	err := json.Unmarshal(raw, &override)
	if err != nil {
		return metadata.StoredOverride{}, fmt.Errorf("decode override fields: %w", err)
	}

	return override, nil
}
