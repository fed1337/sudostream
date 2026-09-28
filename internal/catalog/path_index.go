package catalog

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/metadata"
)

// PathCatalogIndex implements Index in memory from paths + library type.
// Used by unit/handler tests without Postgres; production uses metadata denorm SQL.
type PathCatalogIndex struct {
	LibraryType access.LibraryType
	ByLibrary   map[string][]string
	Err         error
}

// ListCatalogMoviesPage implements Index.
func (p PathCatalogIndex) ListCatalogMoviesPage(
	_ context.Context,
	libraryID string,
	limit, offset int,
) ([]metadata.CatalogMovieRow, int, error) {
	if p.Err != nil {
		return nil, 0, p.Err
	}

	rows := make([]metadata.CatalogMovieRow, 0)
	for _, relPath := range p.ByLibrary[libraryID] {
		denorm := metadata.CatalogDenormFrom(
			p.LibraryType, relPath, metadata.VideoFields{}, metadata.StoredOverride{},
		)
		rows = append(rows, metadata.CatalogMovieRow{
			RelPath: relPath,
			Title:   denorm.SortTitle,
			Year:    denorm.Year,
		})
	}
	sort.Slice(rows, func(left, right int) bool {
		return strings.ToLower(rows[left].Title) < strings.ToLower(rows[right].Title)
	})

	paged, total := pageCatalogRows(rows, limit, offset)

	return paged, total, nil
}

// ListCatalogShowsPage implements Index.
func (p PathCatalogIndex) ListCatalogShowsPage(
	_ context.Context,
	libraryID string,
	limit, offset int,
) ([]metadata.CatalogShowRow, int, error) {
	if p.Err != nil {
		return nil, 0, p.Err
	}

	type agg struct {
		name         string
		seasons      map[int]struct{}
		episodeCount int
		posterPath   string
	}
	byKey := map[string]*agg{}
	for _, relPath := range p.ByLibrary[libraryID] {
		denorm := metadata.CatalogDenormFrom(
			p.LibraryType, relPath, metadata.VideoFields{}, metadata.StoredOverride{},
		)
		if denorm.ShowKey == "" {
			continue
		}
		entry, ok := byKey[denorm.ShowKey]
		if !ok {
			entry = &agg{
				name:       denorm.SortTitle,
				seasons:    map[int]struct{}{},
				posterPath: relPath,
			}
			byKey[denorm.ShowKey] = entry
		}
		entry.episodeCount++
		season := 0
		if denorm.Season != nil {
			season = *denorm.Season
		}
		entry.seasons[season] = struct{}{}
		if relPath < entry.posterPath {
			entry.posterPath = relPath
		}
	}

	rows := make([]metadata.CatalogShowRow, 0, len(byKey))
	for key, entry := range byKey {
		rows = append(rows, metadata.CatalogShowRow{
			ShowKey:      key,
			Name:         entry.name,
			SeasonCount:  len(entry.seasons),
			EpisodeCount: entry.episodeCount,
			PosterPath:   entry.posterPath,
		})
	}
	sort.Slice(rows, func(left, right int) bool {
		return strings.ToLower(rows[left].Name) < strings.ToLower(rows[right].Name)
	})

	paged, total := pageCatalogRows(rows, limit, offset)

	return paged, total, nil
}

// GetCatalogShowAgg implements Index.
func (p PathCatalogIndex) GetCatalogShowAgg( //nolint:cyclop // season map aggregation for tests
	_ context.Context,
	libraryID, showKey string,
) (metadata.CatalogShowAgg, bool, error) {
	if p.Err != nil {
		return metadata.CatalogShowAgg{}, false, p.Err
	}

	counts := map[int]int{}
	name := ""
	posterPath := ""
	for _, relPath := range p.ByLibrary[libraryID] {
		denorm := metadata.CatalogDenormFrom(
			p.LibraryType, relPath, metadata.VideoFields{}, metadata.StoredOverride{},
		)
		if denorm.ShowKey != showKey {
			continue
		}
		if name == "" {
			name = denorm.SortTitle
		}
		if posterPath == "" || relPath < posterPath {
			posterPath = relPath
		}
		season := 0
		if denorm.Season != nil {
			season = *denorm.Season
		}
		counts[season]++
	}
	if len(counts) == 0 {
		return metadata.CatalogShowAgg{}, false, nil
	}
	if name == "" {
		name = showKey
	}

	seasons := make([]int, 0, len(counts))
	for season := range counts {
		seasons = append(seasons, season)
	}
	sort.Ints(seasons)
	out := metadata.CatalogShowAgg{
		ShowKey:    showKey,
		Name:       name,
		PosterPath: posterPath,
		Seasons:    make([]metadata.CatalogSeasonCount, 0, len(seasons)),
	}
	for _, season := range seasons {
		out.Seasons = append(out.Seasons, metadata.CatalogSeasonCount{
			Season:       season,
			EpisodeCount: counts[season],
		})
	}

	return out, true, nil
}

// CatalogShowExists implements Index.
func (p PathCatalogIndex) CatalogShowExists(
	_ context.Context,
	libraryID, showKey string,
) (bool, error) {
	if p.Err != nil {
		return false, p.Err
	}
	for _, relPath := range p.ByLibrary[libraryID] {
		denorm := metadata.CatalogDenormFrom(
			p.LibraryType, relPath, metadata.VideoFields{}, metadata.StoredOverride{},
		)
		if denorm.ShowKey == showKey {
			return true, nil
		}
	}

	return false, nil
}

// ListCatalogSeasonEpisodes implements Index.
func (p PathCatalogIndex) ListCatalogSeasonEpisodes( //nolint:cyclop // filter + sort for test index
	_ context.Context,
	libraryID, showKey string,
	season, limit, offset int,
) ([]metadata.CatalogEpisodeRow, int, error) {
	if p.Err != nil {
		return nil, 0, p.Err
	}

	rows := make([]metadata.CatalogEpisodeRow, 0)
	for _, relPath := range p.ByLibrary[libraryID] {
		denorm := metadata.CatalogDenormFrom(
			p.LibraryType, relPath, metadata.VideoFields{}, metadata.StoredOverride{},
		)
		if denorm.ShowKey != showKey {
			continue
		}
		seasonNum := 0
		if denorm.Season != nil {
			seasonNum = *denorm.Season
		}
		if seasonNum != season {
			continue
		}
		title := denorm.DisplayName
		if title == "" {
			title = filepath.Base(relPath)
		}
		rows = append(rows, metadata.CatalogEpisodeRow{
			RelPath:      relPath,
			Title:        title,
			Season:       denorm.Season,
			Episode:      denorm.Episode,
			EpisodeTitle: denorm.EpisodeTitle,
		})
	}
	sort.Slice(rows, func(left, right int) bool {
		ei, ej := rows[left].Episode, rows[right].Episode
		if ei != nil && ej != nil && *ei != *ej {
			return *ei < *ej
		}
		if ei != nil && ej == nil {
			return true
		}
		if ei == nil && ej != nil {
			return false
		}

		return rows[left].RelPath < rows[right].RelPath
	})

	paged, total := pageCatalogRows(rows, limit, offset)

	return paged, total, nil
}

func pageCatalogRows[T any](rows []T, limit, offset int) ([]T, int) {
	total := len(rows)
	if offset > total {
		return nil, total
	}
	end := total
	if limit > 0 {
		end = min(offset+limit, total)
	}

	return rows[offset:end], total
}

func indexedFilmPaths(slug string, names ...string) PathCatalogIndex {
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.ToSlash(filepath.Join(slug, name)))
	}

	return PathCatalogIndex{
		LibraryType: access.LibraryTypeFilm,
		ByLibrary:   map[string][]string{"1": paths},
	}
}

func indexedSeriesPaths(paths ...string) PathCatalogIndex {
	return PathCatalogIndex{
		LibraryType: access.LibraryTypeSeries,
		ByLibrary:   map[string][]string{"1": append([]string(nil), paths...)},
	}
}
