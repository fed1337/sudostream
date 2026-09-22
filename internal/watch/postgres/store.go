// Package postgres persists per-user watched state.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sudoStream/internal/watch"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store persists watch rows in PostgreSQL.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a watch store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Get returns watch state for a user and media path. Missing rows are zero State.
func (s *Store) Get(ctx context.Context, userID, libraryID, relPath string) (watch.State, error) {
	var model watchStateModel

	err := s.db.WithContext(ctx).
		Where("user_id = ? AND library_id = ? AND rel_path = ?", userID, libraryID, relPath).
		Take(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return watch.State{}, nil
		}

		return watch.State{}, fmt.Errorf("get watch state: %w", err)
	}

	return modelToState(model), nil
}

// Upsert writes watch state for a user and media path.
func (s *Store) Upsert(
	ctx context.Context,
	userID, libraryID, relPath string,
	state watch.State,
) error {
	now := time.Now().UTC()
	model := watchStateModel{
		UserID:          userID,
		LibraryID:       libraryID,
		RelPath:         relPath,
		Watched:         state.Watched,
		PositionSeconds: state.PositionSeconds,
		DurationSeconds: state.DurationSeconds,
		WatchedAt:       now,
		UpdatedAt:       now,
	}
	updates := map[string]any{
		"watched":          state.Watched,
		"position_seconds": state.PositionSeconds,
		"duration_seconds": state.DurationSeconds,
		"updated_at":       now,
	}
	if state.Watched {
		updates["watched_at"] = now
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "library_id"},
			{Name: "rel_path"},
		},
		DoUpdates: clause.Assignments(updates),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("upsert watch state: %w", err)
	}

	return nil
}

// Clear removes watch state for a user and media path.
func (s *Store) Clear(ctx context.Context, userID, libraryID, relPath string) error {
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND library_id = ? AND rel_path = ?", userID, libraryID, relPath).
		Delete(&watchStateModel{}).Error
	if err != nil {
		return fmt.Errorf("clear watch state: %w", err)
	}

	return nil
}

// ListForPaths returns watched flags for the given paths in one library.
func (s *Store) ListForPaths(
	ctx context.Context,
	userID, libraryID string,
	relPaths []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(relPaths))
	if len(relPaths) == 0 {
		return result, nil
	}

	var rows []watchStateModel
	err := s.db.WithContext(ctx).
		Where(
			"user_id = ? AND library_id = ? AND watched = TRUE AND rel_path IN ?",
			userID,
			libraryID,
			relPaths,
		).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list watch state: %w", err)
	}

	for _, row := range rows {
		result[row.RelPath] = row.Watched
	}

	return result, nil
}

// ListForUser returns the user's most recently completed watches in the given libraries.
func (s *Store) ListForUser(
	ctx context.Context,
	userID string,
	libraryIDs []string,
	limit, offset int,
) ([]watch.Row, error) {
	return s.listRows(
		ctx,
		userID,
		libraryIDs,
		limit,
		offset,
		"watched = TRUE",
		"watched_at DESC",
	)
}

// CountForUser returns how many completed watches the user has in the given libraries.
func (s *Store) CountForUser(
	ctx context.Context,
	userID string,
	libraryIDs []string,
) (int64, error) {
	return s.countRows(ctx, userID, libraryIDs, "watched = TRUE")
}

// ListContinue returns in-progress titles for the Continue watching shelf.
func (s *Store) ListContinue(
	ctx context.Context,
	userID string,
	libraryIDs []string,
	limit, offset int,
) ([]watch.Row, error) {
	return s.listRows(
		ctx,
		userID,
		libraryIDs,
		limit,
		offset,
		"watched = FALSE AND COALESCE(position_seconds, 0) >= ?",
		"updated_at DESC",
		watch.MinProgressSeconds,
	)
}

// CountContinue returns how many in-progress titles the user has in the given libraries.
func (s *Store) CountContinue(
	ctx context.Context,
	userID string,
	libraryIDs []string,
) (int64, error) {
	return s.countRows(
		ctx,
		userID,
		libraryIDs,
		"watched = FALSE AND COALESCE(position_seconds, 0) >= ?",
		watch.MinProgressSeconds,
	)
}

// CountByLibraries returns watched media counts by library for one user.
func (s *Store) CountByLibraries(
	ctx context.Context,
	userID string,
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
		Model(&watchStateModel{}).
		Select("library_id, COUNT(*) AS count").
		Where("user_id = ? AND library_id IN ? AND watched = TRUE", userID, libraryIDs).
		Group("library_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count watch state: %w", err)
	}

	for _, row := range rows {
		counts[row.LibraryID] = row.Count
	}

	return counts, nil
}

// DeleteForPath removes watch rows for a deleted media path.
func (s *Store) DeleteForPath(ctx context.Context, libraryID, relPath string) error {
	err := s.db.WithContext(ctx).
		Where("library_id = ? AND (rel_path = ? OR rel_path LIKE ?)", libraryID, relPath, relPath+"/%").
		Delete(&watchStateModel{}).Error
	if err != nil {
		return fmt.Errorf("delete watch state: %w", err)
	}

	return nil
}

// Ping verifies the store can reach the database.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return watch.ErrStoreUnavailable
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("watch store db handle: %w", err)
	}

	err = sqlDB.PingContext(ctx)
	if err != nil {
		return fmt.Errorf("watch store ping: %w", err)
	}

	return nil
}

func modelToState(model watchStateModel) watch.State {
	return watch.State{
		Watched:         model.Watched,
		PositionSeconds: model.PositionSeconds,
		DurationSeconds: model.DurationSeconds,
	}
}

func (s *Store) listRows(
	ctx context.Context,
	userID string,
	libraryIDs []string,
	limit, offset int,
	filter string,
	order string,
	args ...any,
) ([]watch.Row, error) {
	if len(libraryIDs) == 0 || limit <= 0 {
		return []watch.Row{}, nil
	}
	if offset < 0 {
		offset = 0
	}

	queryArgs := append([]any{userID, libraryIDs}, args...)
	var models []watchStateModel
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND library_id IN ? AND "+filter, queryArgs...).
		Order(order).
		Limit(limit).
		Offset(offset).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list user watch state: %w", err)
	}

	rows := make([]watch.Row, 0, len(models))
	for _, model := range models {
		rows = append(rows, watch.Row{
			LibraryID:       model.LibraryID,
			RelPath:         model.RelPath,
			WatchedAt:       model.WatchedAt,
			PositionSeconds: model.PositionSeconds,
			DurationSeconds: model.DurationSeconds,
		})
	}

	return rows, nil
}

func (s *Store) countRows(
	ctx context.Context,
	userID string,
	libraryIDs []string,
	filter string,
	args ...any,
) (int64, error) {
	if len(libraryIDs) == 0 {
		return 0, nil
	}

	queryArgs := append([]any{userID, libraryIDs}, args...)
	var total int64
	err := s.db.WithContext(ctx).
		Model(&watchStateModel{}).
		Where("user_id = ? AND library_id IN ? AND "+filter, queryArgs...).
		Count(&total).Error
	if err != nil {
		return 0, fmt.Errorf("count watch state: %w", err)
	}

	return total, nil
}
