package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sudoStream/internal/metadata"

	"gorm.io/gorm"
)

type searchCapabilities struct {
	pgTrgm   bool
	unaccent bool
}

func (s *Store) refreshSearchDocument(ctx context.Context, libraryID, relPath string) error {
	row, err := s.Get(ctx, libraryID, relPath)
	if errors.Is(err, metadata.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	document := metadata.BuildSearchDocument(relPath, row.Original, row.Override.VideoFields)
	err = s.db.WithContext(ctx).
		Model(&metadataModel{}).
		Where("library_id = ? AND rel_path = ?", libraryID, relPath).
		Update("search_document", document).Error
	if err != nil {
		return fmt.Errorf("update search document: %w", err)
	}

	return nil
}

func (s *Store) searchCaps(ctx context.Context) searchCapabilities {
	s.capsOnce.Do(func() {
		s.pgTrgm = extensionInstalled(s.db.WithContext(ctx), "pg_trgm")
		s.unaccent = extensionInstalled(s.db.WithContext(ctx), "unaccent")
		if !s.pgTrgm {
			slog.Warn("pg_trgm extension missing; search uses ILIKE without trigram ranking")
		}
	})

	return searchCapabilities{pgTrgm: s.pgTrgm, unaccent: s.unaccent}
}

func extensionInstalled(db *gorm.DB, name string) bool {
	var count int64
	err := db.Raw("SELECT count(*) FROM pg_extension WHERE extname = ?", name).Scan(&count).Error
	if err != nil {
		slog.Warn("detect postgres extension failed", "extension", name, "err", err)

		return false
	}

	return count > 0
}

// Search returns metadata rows matching all query tokens.
func (s *Store) Search(
	ctx context.Context,
	libraryIDs []string,
	tokens []string,
	rawQuery string,
	limit, offset int,
) ([]metadata.SearchRow, int, error) {
	if len(libraryIDs) == 0 || len(tokens) == 0 {
		return nil, 0, nil
	}

	caps := s.searchCaps(ctx)
	if caps.unaccent {
		tokens = s.unaccentTokens(ctx, tokens)
		rawQuery = s.unaccentValue(ctx, rawQuery)
	}

	base := s.db.WithContext(ctx).
		Model(&metadataModel{}).
		Where("library_id IN ?", libraryIDs).
		Where("rel_path <> ? AND rel_path NOT LIKE ?", ".trash", ".trash/%").
		Where(`NOT EXISTS (
			SELECT 1 FROM trash_items t
			WHERE media_metadata.rel_path = t.original_rel_path
			   OR media_metadata.rel_path LIKE t.original_rel_path || '/%'
		)`)

	for _, token := range tokens {
		base = base.Where(
			"search_document ILIKE ? ESCAPE '\\'",
			"%"+metadata.EscapeILIKEToken(token)+"%",
		)
	}

	var total int64
	err := base.Session(&gorm.Session{}).Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("count search: %w", err)
	}

	query := base.Session(&gorm.Session{})
	if caps.pgTrgm && rawQuery != "" {
		query = query.Order(gorm.Expr("public.similarity(search_document, ?) DESC", rawQuery))
	} else {
		query = query.Order("rel_path ASC")
	}

	var models []metadataModel
	err = query.Limit(limit).Offset(offset).
		Select("library_id", "rel_path").
		Find(&models).Error
	if err != nil {
		return nil, 0, fmt.Errorf("search metadata: %w", err)
	}

	rows := make([]metadata.SearchRow, 0, len(models))
	for _, model := range models {
		rows = append(rows, metadata.SearchRow{LibraryID: model.LibraryID, RelPath: model.RelPath})
	}

	return rows, int(total), nil
}

func (s *Store) unaccentTokens(ctx context.Context, tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		folded := s.unaccentValue(ctx, token)
		if folded != "" {
			out = append(out, folded)
		}
	}

	return out
}

func (s *Store) unaccentValue(ctx context.Context, value string) string {
	if value == "" {
		return ""
	}

	var folded string
	err := s.db.WithContext(ctx).Raw("SELECT public.unaccent(?)", value).Scan(&folded).Error
	if err != nil || folded == "" {
		return value
	}

	return folded
}
