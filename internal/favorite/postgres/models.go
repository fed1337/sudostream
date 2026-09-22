package postgres

import "time"

type favoriteModel struct {
	UserID    string    `gorm:"column:user_id;primaryKey;type:uuid"`
	LibraryID string    `gorm:"column:library_id;primaryKey;type:uuid"`
	RelPath   string    `gorm:"column:rel_path;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (favoriteModel) TableName() string {
	return "favorites"
}
