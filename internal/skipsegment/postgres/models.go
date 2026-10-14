package postgres

import "time"

type skipIntroSegmentModel struct {
	ID            string    `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	LibraryID     string    `gorm:"column:library_id;type:uuid"`
	RelPath       string    `gorm:"column:rel_path"`
	Kind          string    `gorm:"column:kind"`
	StartMs       int64     `gorm:"column:start_ms"`
	EndMs         int64     `gorm:"column:end_ms"`
	Source        string    `gorm:"column:source"`
	Confidence    float64   `gorm:"column:confidence"`
	EngineVersion int       `gorm:"column:engine_version"`
	ShowKey       string    `gorm:"column:show_key"`
	Season        int       `gorm:"column:season"`
	DetectedAt    time.Time `gorm:"column:detected_at"`
}

func (skipIntroSegmentModel) TableName() string {
	return "skip_intro_segments"
}
