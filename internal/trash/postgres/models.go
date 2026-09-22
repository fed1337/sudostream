// Package postgres persists recycle-bin rows.
package postgres

import "time"

type trashItemModel struct {
	ID              string    `gorm:"column:id;primaryKey;type:uuid"`
	OriginalRelPath string    `gorm:"column:original_rel_path"`
	TrashRelPath    string    `gorm:"column:trash_rel_path"`
	LibraryID       *string   `gorm:"column:library_id;type:uuid"`
	DeletedAt       time.Time `gorm:"column:deleted_at"`
	DeletedBy       *string   `gorm:"column:deleted_by;type:uuid"`
}

func (trashItemModel) TableName() string {
	return "trash_items"
}
