package postgres

import "time"

type watchStateModel struct {
	UserID          string    `gorm:"column:user_id;primaryKey;type:uuid"`
	LibraryID       string    `gorm:"column:library_id;primaryKey;type:uuid"`
	RelPath         string    `gorm:"column:rel_path;primaryKey"`
	Watched         bool      `gorm:"column:watched"`
	PositionSeconds *float64  `gorm:"column:position_seconds"`
	DurationSeconds *float64  `gorm:"column:duration_seconds"`
	WatchedAt       time.Time `gorm:"column:watched_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (watchStateModel) TableName() string {
	return "watch_state"
}
