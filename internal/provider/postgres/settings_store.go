// Package postgres implements provider.Store with GORM/PostgreSQL.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sudoStream/internal/provider"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SettingsStore persists library_provider_settings rows.
type SettingsStore struct {
	db *gorm.DB
}

// NewSettingsStore constructs a provider settings store.
func NewSettingsStore(db *gorm.DB) *SettingsStore {
	return &SettingsStore{db: db}
}

type settingsModel struct {
	LibraryID                 string         `gorm:"column:library_id;primaryKey;type:uuid"`
	MetadataProvider          *string        `gorm:"column:metadata_provider"`
	PosterProvider            *string        `gorm:"column:poster_provider"`
	SubtitleProvider          *string        `gorm:"column:subtitle_provider"`
	SubtitleLanguages         datatypes.JSON `gorm:"column:subtitle_languages"`
	AllowOverrideUserMetadata bool           `gorm:"column:allow_override_user_metadata"`
	MetadataApplyMode         string         `gorm:"column:metadata_apply_mode"`
	MetadataWriteTarget       string         `gorm:"column:metadata_write_target"`
	UpdatedAt                 time.Time      `gorm:"column:updated_at"`
}

func (settingsModel) TableName() string { return "library_provider_settings" }

// GetSettings returns provider.ErrNotFound when libraryID has no stored row.
func (s *SettingsStore) GetSettings(
	ctx context.Context,
	libraryID string,
) (provider.Settings, error) {
	var model settingsModel

	err := s.db.WithContext(ctx).Where("library_id = ?", libraryID).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return provider.Settings{}, provider.ErrNotFound
	}
	if err != nil {
		return provider.Settings{}, fmt.Errorf("get provider settings: %w", err)
	}

	return settingsFromModel(model)
}

// UpsertSettings inserts or replaces the row for settings.LibraryID.
func (s *SettingsStore) UpsertSettings(
	ctx context.Context,
	settings provider.Settings,
) (provider.Settings, error) {
	langsRaw, err := json.Marshal(settings.SubtitleLanguages)
	if err != nil {
		return provider.Settings{}, fmt.Errorf("marshal subtitle languages: %w", err)
	}

	now := time.Now().UTC()
	model := settingsModel{
		LibraryID:                 settings.LibraryID,
		MetadataProvider:          settings.MetadataProvider,
		PosterProvider:            settings.PosterProvider,
		SubtitleProvider:          settings.SubtitleProvider,
		SubtitleLanguages:         datatypes.JSON(langsRaw),
		AllowOverrideUserMetadata: settings.AllowOverrideUserMetadata,
		MetadataApplyMode:         settings.MetadataApplyMode,
		MetadataWriteTarget:       settings.MetadataWriteTarget,
		UpdatedAt:                 now,
	}

	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "library_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"metadata_provider",
			"poster_provider",
			"subtitle_provider",
			"subtitle_languages",
			"allow_override_user_metadata",
			"metadata_apply_mode",
			"metadata_write_target",
			"updated_at",
		}),
	}).Create(&model).Error
	if err != nil {
		return provider.Settings{}, fmt.Errorf("upsert provider settings: %w", err)
	}

	return settingsFromModel(model)
}

func settingsFromModel(model settingsModel) (provider.Settings, error) {
	langs := []string{}
	if len(model.SubtitleLanguages) > 0 {
		err := json.Unmarshal(model.SubtitleLanguages, &langs)
		if err != nil {
			return provider.Settings{}, fmt.Errorf("unmarshal subtitle languages: %w", err)
		}
	}

	return provider.Settings{
		LibraryID:                 model.LibraryID,
		MetadataProvider:          model.MetadataProvider,
		PosterProvider:            model.PosterProvider,
		SubtitleProvider:          model.SubtitleProvider,
		SubtitleLanguages:         langs,
		AllowOverrideUserMetadata: model.AllowOverrideUserMetadata,
		MetadataApplyMode:         model.MetadataApplyMode,
		MetadataWriteTarget:       model.MetadataWriteTarget,
		UpdatedAt:                 model.UpdatedAt,
	}, nil
}
