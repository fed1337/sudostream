// Package postgres persists per-user favorites.
package postgres

import (
	"context"
	"fmt"
	"sudoStream/internal/favorite"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store persists favorite rows in PostgreSQL.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a favorite store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Get returns whether the user has favorited the path.
func (s *Store) Get(ctx context.Context, userID, libraryID, relPath string) (bool, error) {
	var count int64

	err := s.db.WithContext(ctx).
		Model(&favoriteModel{}).
		Where("user_id = ? AND library_id = ? AND rel_path = ?", userID, libraryID, relPath).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("count favorite: %w", err)
	}

	return count > 0, nil
}

// Set upserts or clears a favorite for a user and media path.
func (s *Store) Set(ctx context.Context, userID, libraryID, relPath string, favorited bool) error {
	if !favorited {
		err := s.db.WithContext(ctx).
			Where("user_id = ? AND library_id = ? AND rel_path = ?", userID, libraryID, relPath).
			Delete(&favoriteModel{}).Error
		if err != nil {
			return fmt.Errorf("clear favorite: %w", err)
		}

		return nil
	}

	now := time.Now().UTC()
	model := favoriteModel{
		UserID:    userID,
		LibraryID: libraryID,
		RelPath:   relPath,
		CreatedAt: now,
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "library_id"},
			{Name: "rel_path"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"created_at": now,
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("upsert favorite: %w", err)
	}

	return nil
}

// ListForPaths returns favorite flags for the given paths in one library.
func (s *Store) ListForPaths(
	ctx context.Context,
	userID, libraryID string,
	relPaths []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(relPaths))
	if len(relPaths) == 0 {
		return result, nil
	}

	var rows []favoriteModel
	err := s.db.WithContext(ctx).
		Where(
			"user_id = ? AND library_id = ? AND rel_path IN ?",
			userID,
			libraryID,
			relPaths,
		).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}

	for _, row := range rows {
		result[row.RelPath] = true
	}

	return result, nil
}

// ListForUser returns the user's newest favorites in the given libraries.
func (s *Store) ListForUser(
	ctx context.Context,
	userID string,
	libraryIDs []string,
	limit, offset int,
) ([]favorite.Row, error) {
	if len(libraryIDs) == 0 || limit <= 0 {
		return []favorite.Row{}, nil
	}
	if offset < 0 {
		offset = 0
	}

	var models []favoriteModel
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND library_id IN ?", userID, libraryIDs).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list user favorites: %w", err)
	}

	rows := make([]favorite.Row, 0, len(models))
	for _, model := range models {
		rows = append(rows, favorite.Row{
			LibraryID: model.LibraryID,
			RelPath:   model.RelPath,
			CreatedAt: model.CreatedAt,
		})
	}

	return rows, nil
}

// CountForUser returns how many favorites the user has in the given libraries.
func (s *Store) CountForUser(
	ctx context.Context,
	userID string,
	libraryIDs []string,
) (int64, error) {
	if len(libraryIDs) == 0 {
		return 0, nil
	}

	var total int64
	err := s.db.WithContext(ctx).
		Model(&favoriteModel{}).
		Where("user_id = ? AND library_id IN ?", userID, libraryIDs).
		Count(&total).Error
	if err != nil {
		return 0, fmt.Errorf("count user favorites: %w", err)
	}

	return total, nil
}

// DeleteForPath removes favorite rows for a deleted media path.
func (s *Store) DeleteForPath(ctx context.Context, libraryID, relPath string) error {
	err := s.db.WithContext(ctx).
		Where("library_id = ? AND (rel_path = ? OR rel_path LIKE ?)", libraryID, relPath, relPath+"/%").
		Delete(&favoriteModel{}).Error
	if err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}

	return nil
}

// Ping verifies the store can reach the database.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return favorite.ErrStoreUnavailable
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("favorite store db handle: %w", err)
	}

	err = sqlDB.PingContext(ctx)
	if err != nil {
		return fmt.Errorf("favorite store ping: %w", err)
	}

	return nil
}
