package postgres

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/datatypes"
)

type metadataModel struct {
	LibraryID      string         `gorm:"column:library_id;primaryKey;type:uuid"`
	RelPath        string         `gorm:"column:rel_path;primaryKey"`
	OriginalRaw    datatypes.JSON `gorm:"column:original_fields"`
	OverrideRaw    datatypes.JSON `gorm:"column:override_fields"`
	FileMtime      *time.Time     `gorm:"column:file_mtime"`
	FileSize       *int64         `gorm:"column:file_size"`
	ProbedAt       *time.Time     `gorm:"column:probed_at"`
	OverrideAt     *time.Time     `gorm:"column:override_updated_at"`
	OverriddenBy   *string        `gorm:"column:overridden_by"`
	SearchDocument string         `gorm:"column:search_document"`
}

func (metadataModel) TableName() string {
	return "media_metadata"
}

func encodeFields(raw any) (datatypes.JSON, error) {
	if raw == nil {
		return datatypes.JSON([]byte("{}")), nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal json fields: %w", err)
	}

	return datatypes.JSON(encoded), nil
}
