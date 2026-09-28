package postgres

import (
	"context"
	"fmt"
	"sudoStream/internal/metadata"

	"gorm.io/gorm"
)

func (s *Store) activeMetadataScope(db *gorm.DB, libraryID string) *gorm.DB {
	return db.Model(&metadataModel{}).
		Where("library_id = ?", libraryID).
		Where("rel_path <> ? AND rel_path NOT LIKE ?", ".trash", ".trash/%").
		Where(`NOT EXISTS (
			SELECT 1 FROM trash_items t
			WHERE media_metadata.rel_path = t.original_rel_path
			   OR media_metadata.rel_path LIKE t.original_rel_path || '/%'
		)`)
}

// ListCatalogMoviesPage returns one film page ordered by sort title.
func (s *Store) ListCatalogMoviesPage(
	ctx context.Context,
	libraryID string,
	limit, offset int,
) ([]metadata.CatalogMovieRow, int, error) {
	base := s.activeMetadataScope(s.db.WithContext(ctx), libraryID)

	var total int64
	err := base.Session(&gorm.Session{}).Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("count catalog movies: %w", err)
	}

	var models []metadataModel
	err = base.Session(&gorm.Session{}).
		Select("rel_path", "catalog_sort_title", "catalog_year").
		Order("LOWER(catalog_sort_title) ASC, rel_path ASC").
		Limit(limit).
		Offset(offset).
		Find(&models).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list catalog movies: %w", err)
	}

	rows := make([]metadata.CatalogMovieRow, 0, len(models))
	for _, model := range models {
		rows = append(rows, metadata.CatalogMovieRow{
			RelPath: model.RelPath,
			Title:   model.CatalogSortTitle,
			Year:    model.CatalogYear,
		})
	}

	return rows, int(total), nil
}

// ListCatalogShowsPage returns one series shows page grouped by catalog_show_key.
func (s *Store) ListCatalogShowsPage(
	ctx context.Context,
	libraryID string,
	limit, offset int,
) ([]metadata.CatalogShowRow, int, error) {
	base := s.activeMetadataScope(s.db.WithContext(ctx), libraryID).
		Where("catalog_show_key <> ''")

	var total int64
	err := base.Session(&gorm.Session{}).
		Select("COUNT(DISTINCT catalog_show_key)").
		Scan(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("count catalog shows: %w", err)
	}

	type showAggRow struct {
		ShowKey      string
		Name         string
		SeasonCount  int
		EpisodeCount int
		PosterPath   string
	}

	var aggs []showAggRow
	err = base.Session(&gorm.Session{}).
		Select(`catalog_show_key AS show_key,
			COALESCE(NULLIF(MIN(catalog_sort_title), ''), MIN(catalog_show_key)) AS name,
			COUNT(DISTINCT COALESCE(catalog_season, 0)) AS season_count,
			COUNT(*) AS episode_count,
			MIN(rel_path) AS poster_path`).
		Group("catalog_show_key").
		Order("LOWER(COALESCE(NULLIF(MIN(catalog_sort_title), ''), MIN(catalog_show_key))) ASC, catalog_show_key ASC").
		Limit(limit).
		Offset(offset).
		Scan(&aggs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list catalog shows: %w", err)
	}

	rows := make([]metadata.CatalogShowRow, 0, len(aggs))
	for _, agg := range aggs {
		rows = append(rows, metadata.CatalogShowRow{
			ShowKey:      agg.ShowKey,
			Name:         agg.Name,
			SeasonCount:  agg.SeasonCount,
			EpisodeCount: agg.EpisodeCount,
			PosterPath:   agg.PosterPath,
		})
	}

	return rows, int(total), nil
}

// GetCatalogShowAgg returns season counts for one show_key.
func (s *Store) GetCatalogShowAgg(
	ctx context.Context,
	libraryID, showKey string,
) (metadata.CatalogShowAgg, bool, error) {
	if showKey == "" {
		return metadata.CatalogShowAgg{}, false, nil
	}

	base := s.activeMetadataScope(s.db.WithContext(ctx), libraryID).
		Where("catalog_show_key = ?", showKey)

	type headerRow struct {
		Name       string
		PosterPath string
		Count      int64
	}
	var header headerRow
	err := base.Session(&gorm.Session{}).
		Select(`COALESCE(NULLIF(MIN(catalog_sort_title), ''), ?) AS name,
			MIN(rel_path) AS poster_path,
			COUNT(*) AS count`, showKey).
		Scan(&header).Error
	if err != nil {
		return metadata.CatalogShowAgg{}, false, fmt.Errorf("get catalog show header: %w", err)
	}
	if header.Count == 0 {
		return metadata.CatalogShowAgg{}, false, nil
	}

	type seasonRow struct {
		Season       int
		EpisodeCount int
	}
	var seasons []seasonRow
	err = base.Session(&gorm.Session{}).
		Select("COALESCE(catalog_season, 0) AS season, COUNT(*) AS episode_count").
		Group("COALESCE(catalog_season, 0)").
		Order("season ASC").
		Scan(&seasons).Error
	if err != nil {
		return metadata.CatalogShowAgg{}, false, fmt.Errorf("get catalog show seasons: %w", err)
	}

	out := metadata.CatalogShowAgg{
		ShowKey:    showKey,
		Name:       header.Name,
		PosterPath: header.PosterPath,
		Seasons:    make([]metadata.CatalogSeasonCount, 0, len(seasons)),
	}
	for _, season := range seasons {
		out.Seasons = append(out.Seasons, metadata.CatalogSeasonCount{
			Season:       season.Season,
			EpisodeCount: season.EpisodeCount,
		})
	}

	return out, true, nil
}

// CatalogShowExists reports whether any active row has the show_key.
func (s *Store) CatalogShowExists(
	ctx context.Context,
	libraryID, showKey string,
) (bool, error) {
	if showKey == "" {
		return false, nil
	}

	var count int64
	err := s.activeMetadataScope(s.db.WithContext(ctx), libraryID).
		Where("catalog_show_key = ?", showKey).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("catalog show exists: %w", err)
	}

	return count > 0, nil
}

// ListCatalogSeasonEpisodes returns episodes for one show season.
func (s *Store) ListCatalogSeasonEpisodes(
	ctx context.Context,
	libraryID, showKey string,
	season, limit, offset int,
) ([]metadata.CatalogEpisodeRow, int, error) {
	base := s.activeMetadataScope(s.db.WithContext(ctx), libraryID).
		Where("catalog_show_key = ?", showKey).
		Where("COALESCE(catalog_season, 0) = ?", season)

	var total int64
	err := base.Session(&gorm.Session{}).Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("count catalog season episodes: %w", err)
	}

	query := base.Session(&gorm.Session{}).
		Select(
			"rel_path",
			"catalog_display_name",
			"catalog_season",
			"catalog_episode",
			"catalog_episode_title",
		).
		Order("catalog_episode ASC NULLS LAST, rel_path ASC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	} else if offset > 0 {
		query = query.Offset(offset)
	}

	var models []metadataModel
	err = query.Find(&models).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list catalog season episodes: %w", err)
	}

	rows := make([]metadata.CatalogEpisodeRow, 0, len(models))
	for _, model := range models {
		title := model.CatalogDisplayName
		if title == "" {
			title = model.RelPath
		}
		rows = append(rows, metadata.CatalogEpisodeRow{
			RelPath:      model.RelPath,
			Title:        title,
			Season:       model.CatalogSeason,
			Episode:      model.CatalogEpisode,
			EpisodeTitle: model.CatalogEpisodeTitle,
		})
	}

	return rows, int(total), nil
}
