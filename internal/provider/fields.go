package provider

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sudoStream/internal/metadata"
)

// showGroup is one series show: every episode file plus the representative file that owns the
// show's poster (catalog's PosterPath convention).
type showGroup struct {
	key        string
	name       string
	year       *int
	posterPath string
	paths      []string
}

// groupShows collapses episode files into shows so metadata and poster tasks call a provider
// once per show instead of once per episode (FI-1: match granularity).
func (e *Enricher) groupShows(ctx context.Context, paths []string) []showGroup {
	byKey := map[string]*showGroup{}

	for _, relPath := range paths {
		name, year := e.showIdentity(ctx, relPath)
		if strings.TrimSpace(name) == "" {
			continue
		}

		key := metadata.NormalizeShowKey(name)
		group, ok := byKey[key]
		if !ok {
			group = &showGroup{key: key, name: name, year: year, posterPath: relPath}
			byKey[key] = group
		}
		if year != nil && group.year == nil {
			group.year = year
		}
		if relPath < group.posterPath {
			group.posterPath = relPath
		}
		group.paths = append(group.paths, relPath)
	}

	groups := make([]showGroup, 0, len(byKey))
	for _, group := range byKey {
		sort.Strings(group.paths)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(left, right int) bool {
		return groups[left].key < groups[right].key
	})

	return groups
}

func (e *Enricher) showIdentity(ctx context.Context, relPath string) (string, *int) {
	if e.deps.Metadata == nil {
		return metadata.ParseSeriesIdentity(relPath).Show, nil
	}

	current, err := e.deps.Metadata.Get(ctx, relPath)
	if err != nil {
		return metadata.ParseSeriesIdentity(relPath).Show, nil
	}

	name := ""
	if current.Effective.Show != nil {
		name = strings.TrimSpace(*current.Effective.Show)
	}
	if name != "" {
		return name, current.Effective.Year
	}

	return metadata.ParseSeriesIdentity(relPath).Show, nil
}

func effectiveTitle(current metadata.MetadataResponse, relPath string) string {
	if current.Effective.Title != nil && strings.TrimSpace(*current.Effective.Title) != "" {
		return strings.TrimSpace(*current.Effective.Title)
	}
	if current.DisplayName != "" {
		return current.DisplayName
	}

	return metadata.ParseFilmIdentity(relPath).Title
}

// fillMissingOnly drops provider values for fields that already resolve to something in the
// effective merge, which is the default apply mode (FI-1 L10 fill_missing).
func fillMissingOnly(
	fields metadata.ProviderFields,
	effective metadata.VideoFields,
) metadata.ProviderFields {
	fields = clearFilledStrings(fields, effective)
	fields = clearFilledNumbers(fields, effective)

	return fields
}

func clearFilledStrings(
	fields metadata.ProviderFields,
	effective metadata.VideoFields,
) metadata.ProviderFields {
	if !blankString(effective.Title) {
		fields.Title = nil
	}
	if !blankString(effective.Show) {
		fields.Show = nil
	}
	if !blankString(effective.Description) {
		fields.Description = nil
	}
	if !blankString(effective.Studio) {
		fields.Studio = nil
	}
	if !blankString(effective.EpisodeTitle) {
		fields.EpisodeTitle = nil
	}
	fields = clearFilledIDs(fields, effective)
	if len(effective.Genres) > 0 {
		fields.Genres = nil
	}

	return fields
}

func clearFilledIDs(
	fields metadata.ProviderFields,
	effective metadata.VideoFields,
) metadata.ProviderFields {
	if !blankString(effective.IMDBID) {
		fields.IMDBID = nil
	}
	if !blankString(effective.TMDBID) {
		fields.TMDBID = nil
	}
	if !blankString(effective.TVDBID) {
		fields.TVDBID = nil
	}
	if !blankString(effective.TvmazeID) {
		fields.TvmazeID = nil
	}

	return fields
}

func clearFilledNumbers(
	fields metadata.ProviderFields,
	effective metadata.VideoFields,
) metadata.ProviderFields {
	if effective.Year != nil {
		fields.Year = nil
	}
	if effective.Season != nil {
		fields.Season = nil
	}
	if effective.Episode != nil {
		fields.Episode = nil
	}

	return fields
}

func providerFieldsEmpty(fields metadata.ProviderFields) bool { //nolint:cyclop // field-empty checklist
	if !blankString(fields.Title) ||
		!blankString(fields.Show) ||
		!blankString(fields.Description) ||
		!blankString(fields.Studio) ||
		!blankString(fields.EpisodeTitle) ||
		!blankString(fields.IMDBID) ||
		!blankString(fields.TMDBID) ||
		!blankString(fields.TVDBID) ||
		!blankString(fields.TvmazeID) {
		return false
	}

	return fields.Year == nil &&
		fields.Season == nil &&
		fields.Episode == nil &&
		len(fields.Genres) == 0
}

func blankString(value *string) bool {
	return value == nil || strings.TrimSpace(*value) == ""
}

func fileExists(absPath string) bool {
	info, err := os.Stat(filepath.Clean(absPath))

	return err == nil && info.Size() > 0
}
