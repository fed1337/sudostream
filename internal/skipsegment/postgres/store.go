// Package postgres persists skip-intro segments.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sudoStream/internal/skipsegment"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store persists skip intro rows in PostgreSQL.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a skip-intro store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Upsert writes or replaces one intro segment for a media path.
func (s *Store) Upsert(ctx context.Context, segment skipsegment.StoredSegment) error {
	if segment.Kind == "" {
		segment.Kind = skipsegment.KindIntro
	}
	now := segment.DetectedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	model := skipIntroSegmentModel{
		LibraryID:     segment.LibraryID,
		RelPath:       segment.RelPath,
		Kind:          segment.Kind,
		StartMs:       segment.StartMs,
		EndMs:         segment.EndMs,
		Source:        segment.Source,
		Confidence:    segment.Confidence,
		EngineVersion: segment.EngineVersion,
		ShowKey:       segment.ShowKey,
		Season:        segment.Season,
		DetectedAt:    now,
	}

	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "library_id"},
			{Name: "rel_path"},
			{Name: "kind"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"start_ms",
			"end_ms",
			"source",
			"confidence",
			"engine_version",
			"show_key",
			"season",
			"detected_at",
		}),
	}).Create(&model).Error
	if err != nil {
		return fmt.Errorf("upsert skip intro: %w", err)
	}

	return nil
}

// Get returns a stored intro segment when present.
func (s *Store) Get(
	ctx context.Context,
	libraryID, relPath string,
) (skipsegment.StoredSegment, bool, error) {
	var model skipIntroSegmentModel
	err := s.db.WithContext(ctx).
		Where(
			"library_id = ? AND rel_path = ? AND kind = ?",
			libraryID,
			relPath,
			skipsegment.KindIntro,
		).
		Take(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return skipsegment.StoredSegment{}, false, nil
		}

		return skipsegment.StoredSegment{}, false, fmt.Errorf("get skip intro: %w", err)
	}

	return skipsegment.StoredSegment{
		LibraryID:     model.LibraryID,
		RelPath:       model.RelPath,
		Kind:          model.Kind,
		StartMs:       model.StartMs,
		EndMs:         model.EndMs,
		Source:        model.Source,
		Confidence:    model.Confidence,
		EngineVersion: model.EngineVersion,
		ShowKey:       model.ShowKey,
		Season:        model.Season,
		DetectedAt:    model.DetectedAt,
	}, true, nil
}

// DeleteForPath removes stored segments for a media path.
func (s *Store) DeleteForPath(ctx context.Context, libraryID, relPath string) error {
	err := s.db.WithContext(ctx).
		Where("library_id = ? AND rel_path = ?", libraryID, relPath).
		Delete(&skipIntroSegmentModel{}).Error
	if err != nil {
		return fmt.Errorf("delete skip intro: %w", err)
	}

	return nil
}
