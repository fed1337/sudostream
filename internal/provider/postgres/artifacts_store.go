package postgres

import (
	"context"
	"errors"
	"fmt"
	"sudoStream/internal/provider"
	"time"

	"gorm.io/gorm"
)

// ArtifactStore persists provider_artifacts rows (the local provider cache index).
type ArtifactStore struct {
	db *gorm.DB
}

// NewArtifactStore constructs a provider artifact store.
func NewArtifactStore(db *gorm.DB) *ArtifactStore {
	return &ArtifactStore{db: db}
}

type artifactModel struct {
	ID          string    `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	LibraryID   string    `gorm:"column:library_id;type:uuid"`
	RelPath     string    `gorm:"column:rel_path"`
	Kind        string    `gorm:"column:kind"`
	Lang        *string   `gorm:"column:lang"`
	ProviderKey string    `gorm:"column:provider_key"`
	CachePath   string    `gorm:"column:cache_path"`
	ExternalID  *string   `gorm:"column:external_id"`
	FetchedAt   time.Time `gorm:"column:fetched_at"`
}

func (artifactModel) TableName() string { return "provider_artifacts" }

// GetArtifactByPath returns provider.ErrNotFound when no row matches the lookup tuple.
func (s *ArtifactStore) GetArtifactByPath(
	ctx context.Context,
	relPath, kind string,
	lang *string,
) (provider.Artifact, error) {
	var model artifactModel

	err := s.db.WithContext(ctx).
		Where("rel_path = ? AND kind = ?", relPath, kind).
		Where("COALESCE(lang, '') = ?", langValue(lang)).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return provider.Artifact{}, provider.ErrNotFound
	}
	if err != nil {
		return provider.Artifact{}, fmt.Errorf("get provider artifact: %w", err)
	}

	return artifactFromModel(model), nil
}

// ListArtifacts returns every artifact of kind for a library, ordered by path.
func (s *ArtifactStore) ListArtifacts(
	ctx context.Context,
	libraryID, kind string,
) ([]provider.Artifact, error) {
	var models []artifactModel

	err := s.db.WithContext(ctx).
		Where("library_id = ? AND kind = ?", libraryID, kind).
		Order("rel_path, COALESCE(lang, '')").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list provider artifacts: %w", err)
	}

	artifacts := make([]provider.Artifact, 0, len(models))
	for _, model := range models {
		artifacts = append(artifacts, artifactFromModel(model))
	}

	return artifacts, nil
}

// ListArtifactsByPath returns every artifact of kind for a media path (typically all subtitle langs).
func (s *ArtifactStore) ListArtifactsByPath(
	ctx context.Context,
	relPath, kind string,
) ([]provider.Artifact, error) {
	var models []artifactModel

	err := s.db.WithContext(ctx).
		Where("rel_path = ? AND kind = ?", relPath, kind).
		Order("COALESCE(lang, '')").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list provider artifacts by path: %w", err)
	}

	artifacts := make([]provider.Artifact, 0, len(models))
	for _, model := range models {
		artifacts = append(artifacts, artifactFromModel(model))
	}

	return artifacts, nil
}

// UpsertArtifact inserts or replaces the row for (library, path, kind, lang). The unique index
// is on COALESCE(lang, ”), so the conflict target is spelled out in SQL rather than via GORM's
// column-name inference.
func (s *ArtifactStore) UpsertArtifact(
	ctx context.Context,
	artifact provider.Artifact,
) (provider.Artifact, error) {
	var model artifactModel

	err := s.db.WithContext(ctx).Raw(`
		INSERT INTO provider_artifacts
			(library_id, rel_path, kind, lang, provider_key, cache_path, external_id, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (library_id, rel_path, kind, COALESCE(lang, ''))
		DO UPDATE SET
			provider_key = EXCLUDED.provider_key,
			cache_path = EXCLUDED.cache_path,
			external_id = EXCLUDED.external_id,
			fetched_at = EXCLUDED.fetched_at
		RETURNING id, library_id, rel_path, kind, lang, provider_key, cache_path,
			external_id, fetched_at`,
		artifact.LibraryID,
		artifact.RelPath,
		artifact.Kind,
		artifact.Lang,
		artifact.ProviderKey,
		artifact.CachePath,
		artifact.ExternalID,
		fetchedAtOrNow(artifact.FetchedAt),
	).Scan(&model).Error
	if err != nil {
		return provider.Artifact{}, fmt.Errorf("upsert provider artifact: %w", err)
	}

	return artifactFromModel(model), nil
}

// DeleteArtifact removes one row by id. Missing rows are not an error.
func (s *ArtifactStore) DeleteArtifact(ctx context.Context, id string) error {
	err := s.db.WithContext(ctx).Where("id = ?", id).Delete(&artifactModel{}).Error
	if err != nil {
		return fmt.Errorf("delete provider artifact: %w", err)
	}

	return nil
}

func artifactFromModel(model artifactModel) provider.Artifact {
	return provider.Artifact{
		ID:          model.ID,
		LibraryID:   model.LibraryID,
		RelPath:     model.RelPath,
		Kind:        model.Kind,
		Lang:        model.Lang,
		ProviderKey: model.ProviderKey,
		CachePath:   model.CachePath,
		ExternalID:  model.ExternalID,
		FetchedAt:   model.FetchedAt,
	}
}

func langValue(lang *string) string {
	if lang == nil {
		return ""
	}

	return *lang
}

func fetchedAtOrNow(fetchedAt time.Time) time.Time {
	if fetchedAt.IsZero() {
		return time.Now().UTC()
	}

	return fetchedAt
}
