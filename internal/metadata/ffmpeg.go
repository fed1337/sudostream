package metadata

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// fileTagUpdates maps patched field keys to ffmpeg -metadata tag pairs.
// Only user-editable tag fields are writable; technical stream fields
// (codecs, dimensions, duration, …) come from ffprobe and are skipped.
// A cleared field maps to an empty value, which removes the container tag.
func fileTagUpdates( //nolint:cyclop,funlen // explicit field-to-tag mapping mirrors ffprobe read keys
	patch map[string]*jsonValue,
	fields VideoFields,
) map[string]string {
	tags := map[string]string{}

	for key := range patch {
		switch key {
		case fieldTitle:
			tags["title"] = stringTagValue(fields.Title)
		case fieldSortTitle:
			tags["sort_name"] = stringTagValue(fields.SortTitle)
		case fieldOriginalTitle:
			tags["original_title"] = stringTagValue(fields.OriginalTitle)
		case fieldEpisodeTitle:
			tags["episode_title"] = stringTagValue(fields.EpisodeTitle)
		case fieldShow:
			tags["show"] = stringTagValue(fields.Show)
		case fieldSeason:
			tags["season_number"] = intTagValue(fields.Season)
		case fieldEpisode:
			tags["episode_id"] = intTagValue(fields.Episode)
		case fieldYear:
			tags["year"] = intTagValue(fields.Year)
		case fieldReleaseDate:
			tags["release_date"] = stringTagValue(fields.ReleaseDate)
		case fieldDescription:
			tags["description"] = stringTagValue(fields.Description)
		case fieldGenres:
			tags["genre"] = listTagValue(fields.Genres)
		case fieldDirectors:
			tags["director"] = listTagValue(fields.Directors)
		case fieldActors:
			tags["actor"] = listTagValue(fields.Actors)
		case fieldWriters:
			tags["writer"] = listTagValue(fields.Writers)
		case fieldProducers:
			tags["producer"] = listTagValue(fields.Producers)
		case fieldStudio:
			tags["studio"] = stringTagValue(fields.Studio)
		case fieldComposer:
			tags["composer"] = stringTagValue(fields.Composer)
		case fieldLanguage:
			tags["language"] = stringTagValue(fields.Language)
		case fieldCountry:
			tags["country"] = stringTagValue(fields.Country)
		case fieldContentRating:
			tags["content_rating"] = stringTagValue(fields.ContentRating)
		case fieldIMDBID:
			tags["imdb_id"] = stringTagValue(fields.IMDBID)
		case fieldTMDBID:
			tags["tmdb_id"] = stringTagValue(fields.TMDBID)
		}
	}

	return tags
}

func stringTagValue(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func intTagValue(value *int) string {
	if value == nil {
		return ""
	}

	return strconv.Itoa(*value)
}

func listTagValue(values []string) string {
	return strings.Join(values, ", ")
}

// WriteFileTags remuxes absPath with updated container tags using
// ffmpeg -c copy, writing to a temp file beside the source and renaming
// atomically on success. An empty tag value removes the container tag.
func WriteFileTags(ctx context.Context, absPath string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}

	tempPath, err := tempOutputPath(absPath)
	if err != nil {
		return err
	}

	args := []string{"-y", "-loglevel", "error", "-i", absPath, "-map", "0", "-c", "copy"}
	for _, key := range slices.Sorted(maps.Keys(tags)) {
		args = append(args, "-metadata", key+"="+tags[key])
	}
	args = append(args, tempPath)

	output, err := exec.CommandContext(
		ctx,
		"ffmpeg",
		args...,
	).CombinedOutput()
	if err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf(
			"%w: ffmpeg %s: %s",
			ErrFileWriteFailed,
			filepath.Base(absPath),
			strings.TrimSpace(string(output)),
		)
	}

	err = os.Rename(tempPath, absPath)
	if err != nil {
		_ = os.Remove(tempPath)

		return fmt.Errorf("%w: replace %s: %w", ErrFileWriteFailed, filepath.Base(absPath), err)
	}

	return nil
}

// tempOutputPath reserves a unique temp file beside absPath keeping the
// container extension so ffmpeg selects the same muxer.
func tempOutputPath(absPath string) (string, error) {
	dir := filepath.Dir(absPath)
	ext := filepath.Ext(absPath)
	base := strings.TrimSuffix(filepath.Base(absPath), ext)

	temp, err := os.CreateTemp(dir, "."+base+".tagwrite-*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp output: %w", err)
	}

	tempPath := temp.Name()
	_ = temp.Close()
	_ = os.Remove(tempPath)

	return tempPath, nil
}
