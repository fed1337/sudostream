package metadata

import (
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
)

// CatalogDenorm holds write-time identity columns for O(result) catalog SQL (E-33).
type CatalogDenorm struct {
	ShowKey      string
	Season       *int
	Episode      *int
	SortTitle    string
	Year         *int
	EpisodeTitle string
	DisplayName  string
}

// CatalogDenormFrom computes denorm columns using the same effective identity as Get/catalog.
func CatalogDenormFrom( //nolint:cyclop // film vs series identity branches
	libraryType access.LibraryType,
	relPath string,
	original VideoFields,
	override StoredOverride,
) CatalogDenorm {
	effective := EffectiveFields(libraryType, relPath, original, override)
	display := DisplayName(libraryType, relPath, original, override)
	basename := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))

	switch libraryType {
	case access.LibraryTypeFilm:
		title := strings.TrimSpace(derefString(effective.Title))
		if title == "" {
			title = strings.TrimSpace(derefString(effective.OriginalTitle))
		}
		if title == "" {
			identity := ParseFilmIdentity(relPath)
			title = identity.Title
		}
		if title == "" {
			title = basename
		}

		return CatalogDenorm{
			SortTitle:   title,
			Year:        effective.Year,
			DisplayName: display,
		}
	case access.LibraryTypeSeries:
		show := strings.TrimSpace(derefString(effective.Show))
		if show == "" {
			show = ParseSeriesIdentity(relPath).Show
		}
		showKey := NormalizeShowKey(show)
		sortTitle := show
		if sortTitle == "" {
			sortTitle = showKey
		}
		if sortTitle == "" {
			sortTitle = basename
		}
		episodeTitle := strings.TrimSpace(derefString(effective.EpisodeTitle))

		return CatalogDenorm{
			ShowKey:      showKey,
			Season:       effective.Season,
			Episode:      effective.Episode,
			SortTitle:    sortTitle,
			EpisodeTitle: episodeTitle,
			DisplayName:  display,
		}
	case access.LibraryTypeMusic, access.LibraryTypePhotos, access.LibraryTypeOther:
		return CatalogDenorm{
			SortTitle:   basename,
			DisplayName: display,
		}
	default:
		return CatalogDenorm{
			SortTitle:   basename,
			DisplayName: display,
		}
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
