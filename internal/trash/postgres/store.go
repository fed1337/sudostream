package postgres

import (
	"context"
	"errors"
	"fmt"
	"sudoStream/internal/trash"

	"gorm.io/gorm"
)

// Store persists trash_items in PostgreSQL.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a trash store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Insert writes a trash item row.
func (s *Store) Insert(ctx context.Context, item trash.Item) error {
	err := s.db.WithContext(ctx).Create(itemToModel(item)).Error
	if err != nil {
		return fmt.Errorf("insert trash item: %w", err)
	}

	return nil
}

// Get returns one trash item.
func (s *Store) Get(ctx context.Context, id string) (trash.Item, error) {
	var model trashItemModel
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return trash.Item{}, trash.ErrNotFound
	}
	if err != nil {
		return trash.Item{}, fmt.Errorf("get trash item: %w", err)
	}

	return modelToItem(model), nil
}

// List returns all trash items, newest first.
func (s *Store) List(ctx context.Context) ([]trash.Item, error) {
	var models []trashItemModel
	err := s.db.WithContext(ctx).Order("deleted_at DESC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list trash items: %w", err)
	}

	items := make([]trash.Item, 0, len(models))
	for _, model := range models {
		items = append(items, modelToItem(model))
	}

	return items, nil
}

// Delete removes a trash item row.
func (s *Store) Delete(ctx context.Context, id string) error {
	err := s.db.WithContext(ctx).Where("id = ?", id).Delete(&trashItemModel{}).Error
	if err != nil {
		return fmt.Errorf("delete trash item: %w", err)
	}

	return nil
}

// OriginalPrefixes returns original_rel_path values for indexer freeze.
func (s *Store) OriginalPrefixes(ctx context.Context) ([]string, error) {
	var paths []string
	err := s.db.WithContext(ctx).
		Model(&trashItemModel{}).
		Pluck("original_rel_path", &paths).Error
	if err != nil {
		return nil, fmt.Errorf("list trash prefixes: %w", err)
	}

	return paths, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func itemToModel(item trash.Item) *trashItemModel {
	return &trashItemModel{
		ID:              item.ID,
		OriginalRelPath: item.OriginalRelPath,
		TrashRelPath:    item.TrashRelPath,
		LibraryID:       optionalString(item.LibraryID),
		DeletedAt:       item.DeletedAt,
		DeletedBy:       optionalString(item.DeletedBy),
	}
}

func modelToItem(model trashItemModel) trash.Item {
	return trash.Item{
		ID:              model.ID,
		OriginalRelPath: model.OriginalRelPath,
		TrashRelPath:    model.TrashRelPath,
		LibraryID:       derefString(model.LibraryID),
		DeletedAt:       model.DeletedAt,
		DeletedBy:       derefString(model.DeletedBy),
	}
}
