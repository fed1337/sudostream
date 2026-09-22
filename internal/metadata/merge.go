package metadata

import (
	"path/filepath"
	"strconv"
	"strings"
	"sudoStream/internal/access"
)

const displayNamePartCapacity = 3

// UsesMetadataForDisplay reports whether a library type should use metadata for labels.
func UsesMetadataForDisplay(libraryType access.LibraryType) bool {
	switch libraryType {
	case access.LibraryTypeFilm, access.LibraryTypeSeries:
		return true
	case access.LibraryTypeMusic, access.LibraryTypePhotos, access.LibraryTypeOther:
		return false
	}

	return false
}

// EffectiveFields merges override over original, then fills empty identity
// fields from path heuristics for film/series (not persisted until Save).
func EffectiveFields(
	libraryType access.LibraryType,
	relPath string,
	original VideoFields,
	override StoredOverride,
) VideoFields {
	effective := cloneFields(original)
	applyOverride(&effective, override.VideoFields)

	if libraryType != access.LibraryTypeFilm && libraryType != access.LibraryTypeSeries {
		return effective
	}

	applyPathHeuristics(&effective, libraryType, relPath)

	return effective
}

// DisplayName computes the UI label for a file path.
func DisplayName( //nolint:cyclop // display precedence is explicit per library type
	libraryType access.LibraryType,
	relPath string,
	original VideoFields,
	override StoredOverride,
) string {
	basename := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))

	if !UsesMetadataForDisplay(libraryType) {
		return basename
	}

	effective := EffectiveFields(libraryType, relPath, original, override)

	switch libraryType {
	case access.LibraryTypeFilm:
		if effective.Title != nil && strings.TrimSpace(*effective.Title) != "" {
			return strings.TrimSpace(*effective.Title)
		}
		if effective.OriginalTitle != nil && strings.TrimSpace(*effective.OriginalTitle) != "" {
			return strings.TrimSpace(*effective.OriginalTitle)
		}

		return basename
	case access.LibraryTypeSeries:
		show := ""
		if effective.Show != nil {
			show = strings.TrimSpace(*effective.Show)
		}

		season := effective.Season
		episode := effective.Episode

		parts := make([]string, 0, displayNamePartCapacity)
		if show != "" {
			parts = append(parts, show)
		}

		if season != nil && episode != nil {
			parts = append(parts, formatSeasonEpisode(*season, *episode))
		}

		if effective.EpisodeTitle != nil {
			title := strings.TrimSpace(*effective.EpisodeTitle)
			if title != "" {
				parts = append(parts, title)
			}
		}

		if len(parts) > 0 {
			return strings.Join(parts, " — ")
		}

		return basename
	case access.LibraryTypeMusic, access.LibraryTypePhotos, access.LibraryTypeOther:
		return basename
	}

	return basename
}

func formatSeasonEpisode(season, episode int) string {
	return "S" + pad2(season) + "E" + pad2(episode)
}

func pad2(value int) string {
	const singleDigitSeasonEpisodeThreshold = 10
	if value < singleDigitSeasonEpisodeThreshold {
		return "0" + strconv.Itoa(value)
	}

	return strconv.Itoa(value)
}

func applyPathHeuristics( //nolint:cyclop // fill empty identity fields per library type
	fields *VideoFields,
	libraryType access.LibraryType,
	relPath string,
) {
	switch libraryType {
	case access.LibraryTypeFilm:
		identity := ParseFilmIdentity(relPath)
		if fields.Title == nil && identity.Title != "" {
			title := identity.Title
			fields.Title = &title
		}
		if fields.Year == nil && identity.Year != nil {
			fields.Year = identity.Year
		}
	case access.LibraryTypeSeries:
		// Path parse wins for S/E/episode title when confident. Embedded file tags (already in
		// fields from Original) are kept when the filename cannot be parsed.
		identity := ParseSeriesIdentity(relPath)
		if identity.Season != nil {
			fields.Season = identity.Season
		}
		if identity.Episode != nil {
			fields.Episode = identity.Episode
		}
		if identity.EpisodeTitle != "" {
			episodeTitle := identity.EpisodeTitle
			fields.EpisodeTitle = &episodeTitle
		}
		if fields.Show == nil && identity.Show != "" {
			show := identity.Show
			fields.Show = &show
		}
	case access.LibraryTypeMusic, access.LibraryTypePhotos, access.LibraryTypeOther:
		return
	}
}

// FilenameHintForPath returns a filename hint for a media relative path, if any.
func FilenameHintForPath(relPath string) *FilenameHint {
	basename := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))

	return ParseFilenameHint(basename)
}

func cloneFields(fields VideoFields) VideoFields {
	cloned := fields
	cloned.Genres = cloneStringSlice(fields.Genres)
	cloned.Tags = cloneStringSlice(fields.Tags)
	cloned.Directors = cloneStringSlice(fields.Directors)
	cloned.Actors = cloneStringSlice(fields.Actors)
	cloned.Writers = cloneStringSlice(fields.Writers)
	cloned.Producers = cloneStringSlice(fields.Producers)
	cloned.AudioLanguages = cloneStringSlice(fields.AudioLanguages)

	return cloned
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]string, len(values))
	copy(cloned, values)

	return cloned
}

func applyOverride(target *VideoFields, override VideoFields) {
	setStringPtr(&target.Title, override.Title)
	setStringPtr(&target.SortTitle, override.SortTitle)
	setStringPtr(&target.OriginalTitle, override.OriginalTitle)
	setStringPtr(&target.EpisodeTitle, override.EpisodeTitle)
	setStringPtr(&target.Show, override.Show)
	setIntPtr(&target.Season, override.Season)
	setIntPtr(&target.Episode, override.Episode)
	setIntPtr(&target.Year, override.Year)
	setStringPtr(&target.ReleaseDate, override.ReleaseDate)
	setStringPtr(&target.Description, override.Description)
	setStringSlice(&target.Genres, override.Genres)
	setStringSlice(&target.Tags, override.Tags)
	setStringSlice(&target.Directors, override.Directors)
	setStringSlice(&target.Actors, override.Actors)
	setStringSlice(&target.Writers, override.Writers)
	setStringSlice(&target.Producers, override.Producers)
	setStringSlice(&target.AudioLanguages, override.AudioLanguages)
	setStringPtr(&target.Studio, override.Studio)
	setStringPtr(&target.Composer, override.Composer)
	setStringPtr(&target.Language, override.Language)
	setStringPtr(&target.Country, override.Country)
	setStringPtr(&target.ContentRating, override.ContentRating)
	setFloatPtr(&target.DurationSeconds, override.DurationSeconds)
	setStringPtr(&target.VideoCodec, override.VideoCodec)
	setStringPtr(&target.AudioCodec, override.AudioCodec)
	setIntPtr(&target.Width, override.Width)
	setIntPtr(&target.Height, override.Height)
	setStringPtr(&target.FrameRate, override.FrameRate)
	setInt64Ptr(&target.Bitrate, override.Bitrate)
	setIntPtr(&target.AudioChannels, override.AudioChannels)
	setStringPtr(&target.IMDBID, override.IMDBID)
	setStringPtr(&target.TMDBID, override.TMDBID)
	setStringPtr(&target.TVDBID, override.TVDBID)
	setStringPtr(&target.TvmazeID, override.TvmazeID)
}

func setStringPtr(target **string, value *string) {
	if value != nil {
		*target = value
	}
}

func setIntPtr(target **int, value *int) {
	if value != nil {
		*target = value
	}
}

func setInt64Ptr(target **int64, value *int64) {
	if value != nil {
		*target = value
	}
}

func setFloatPtr(target **float64, value *float64) {
	if value != nil {
		*target = value
	}
}

func setStringSlice(target *[]string, value []string) {
	if value != nil {
		*target = value
	}
}

// ApplyPatchFields merges partial patch values into override fields.
// JSON null (or a nil *jsonValue from encoding/json) removes the key so
// effective falls back to the file/original value.
func ApplyPatchFields(current StoredOverride, patch map[string]*jsonValue) (StoredOverride, error) {
	updated := StoredOverride{VideoFields: cloneFields(current.VideoFields)}

	for key, value := range patch {
		if !IsEditableField(key) {
			return StoredOverride{}, ErrInvalidPatchValue
		}

		// encoding/json sets map values to nil for JSON null (*T).
		if value == nil || value.IsNull() {
			clearField(&updated.VideoFields, key)

			continue
		}

		err := setField(&updated.VideoFields, key, value.Value())
		if err != nil {
			return StoredOverride{}, err
		}
	}

	return updated, nil
}

// ClearOverrideKeys drops override keys that were written to the file with a
// non-null value (now living in original). Null/empty file writes clear the
// container tag but leave the override value so effective can fall back to it.
func ClearOverrideKeys(override StoredOverride, keys map[string]*jsonValue) StoredOverride {
	updated := StoredOverride{VideoFields: cloneFields(override.VideoFields)}
	for key, value := range keys {
		if !IsEditableField(key) {
			continue
		}
		if value == nil || value.IsNull() {
			continue
		}

		clearField(&updated.VideoFields, key)
	}

	return updated
}

func clearField( //nolint:cyclop,gocyclo,funlen // explicit field clears keep patch semantics obvious
	fields *VideoFields,
	key string,
) {
	switch key {
	case fieldTitle:
		fields.Title = nil
	case fieldSortTitle:
		fields.SortTitle = nil
	case fieldOriginalTitle:
		fields.OriginalTitle = nil
	case fieldEpisodeTitle:
		fields.EpisodeTitle = nil
	case fieldShow:
		fields.Show = nil
	case fieldSeason:
		fields.Season = nil
	case fieldEpisode:
		fields.Episode = nil
	case fieldYear:
		fields.Year = nil
	case fieldReleaseDate:
		fields.ReleaseDate = nil
	case fieldDescription:
		fields.Description = nil
	case fieldGenres:
		fields.Genres = nil
	case fieldTags:
		fields.Tags = nil
	case fieldDirectors:
		fields.Directors = nil
	case fieldActors:
		fields.Actors = nil
	case fieldWriters:
		fields.Writers = nil
	case fieldProducers:
		fields.Producers = nil
	case fieldAudioLanguages:
		fields.AudioLanguages = nil
	case fieldStudio:
		fields.Studio = nil
	case fieldComposer:
		fields.Composer = nil
	case fieldLanguage:
		fields.Language = nil
	case fieldCountry:
		fields.Country = nil
	case fieldContentRating:
		fields.ContentRating = nil
	case fieldDurationSecs:
		fields.DurationSeconds = nil
	case fieldVideoCodec:
		fields.VideoCodec = nil
	case fieldAudioCodec:
		fields.AudioCodec = nil
	case fieldWidth:
		fields.Width = nil
	case fieldHeight:
		fields.Height = nil
	case fieldFrameRate:
		fields.FrameRate = nil
	case fieldBitrate:
		fields.Bitrate = nil
	case fieldAudioChannels:
		fields.AudioChannels = nil
	case fieldIMDBID:
		fields.IMDBID = nil
	case fieldTMDBID:
		fields.TMDBID = nil
	case fieldTVDBID:
		fields.TVDBID = nil
	case fieldTvmazeID:
		fields.TvmazeID = nil
	}
}

func setField( //nolint:cyclop // field-type routing is explicit
	fields *VideoFields,
	key string,
	raw any,
) error {
	switch key {
	case fieldTitle,
		fieldSortTitle,
		fieldOriginalTitle,
		fieldEpisodeTitle,
		fieldShow,
		fieldReleaseDate,
		fieldDescription,
		fieldStudio,
		fieldComposer,
		fieldLanguage,
		fieldCountry,
		fieldContentRating,
		fieldVideoCodec,
		fieldAudioCodec,
		fieldFrameRate,
		fieldIMDBID,
		fieldTMDBID,
		fieldTVDBID,
		fieldTvmazeID:
		value, err := asString(raw)
		if err != nil {
			return err
		}

		assignString(fields, key, value)

		return nil
	case fieldSeason, fieldEpisode, fieldYear, fieldWidth, fieldHeight, fieldAudioChannels:
		value, err := asInt(raw)
		if err != nil {
			return err
		}

		assignInt(fields, key, value)

		return nil
	case fieldBitrate:
		value, err := asInt64(raw)
		if err != nil {
			return err
		}

		fields.Bitrate = &value

		return nil
	case fieldDurationSecs:
		value, err := asFloat(raw)
		if err != nil {
			return err
		}

		fields.DurationSeconds = &value

		return nil
	case fieldGenres,
		fieldTags,
		fieldDirectors,
		fieldActors,
		fieldWriters,
		fieldProducers,
		fieldAudioLanguages:
		values, err := asStringSlice(raw)
		if err != nil {
			return err
		}

		assignStringSlice(fields, key, values)

		return nil
	default:
		return nil
	}
}

func assignString( //nolint:cyclop // one case per metadata field
	fields *VideoFields,
	key, value string,
) {
	switch key {
	case fieldTitle:
		fields.Title = &value
	case fieldSortTitle:
		fields.SortTitle = &value
	case fieldOriginalTitle:
		fields.OriginalTitle = &value
	case fieldEpisodeTitle:
		fields.EpisodeTitle = &value
	case fieldShow:
		fields.Show = &value
	case fieldReleaseDate:
		fields.ReleaseDate = &value
	case fieldDescription:
		fields.Description = &value
	case fieldStudio:
		fields.Studio = &value
	case fieldComposer:
		fields.Composer = &value
	case fieldLanguage:
		fields.Language = &value
	case fieldCountry:
		fields.Country = &value
	case fieldContentRating:
		fields.ContentRating = &value
	case fieldVideoCodec:
		fields.VideoCodec = &value
	case fieldAudioCodec:
		fields.AudioCodec = &value
	case fieldFrameRate:
		fields.FrameRate = &value
	case fieldIMDBID:
		fields.IMDBID = &value
	case fieldTMDBID:
		fields.TMDBID = &value
	case fieldTVDBID:
		fields.TVDBID = &value
	case fieldTvmazeID:
		fields.TvmazeID = &value
	}
}

func assignInt(fields *VideoFields, key string, value int) {
	switch key {
	case fieldSeason:
		fields.Season = &value
	case fieldEpisode:
		fields.Episode = &value
	case fieldYear:
		fields.Year = &value
	case fieldWidth:
		fields.Width = &value
	case fieldHeight:
		fields.Height = &value
	case fieldAudioChannels:
		fields.AudioChannels = &value
	}
}

func assignStringSlice(fields *VideoFields, key string, values []string) {
	switch key {
	case fieldGenres:
		fields.Genres = values
	case fieldTags:
		fields.Tags = values
	case fieldDirectors:
		fields.Directors = values
	case fieldActors:
		fields.Actors = values
	case fieldWriters:
		fields.Writers = values
	case fieldProducers:
		fields.Producers = values
	case fieldAudioLanguages:
		fields.AudioLanguages = values
	}
}

func asString(raw any) (string, error) {
	switch value := raw.(type) {
	case string:
		return value, nil
	case float64:
		return strconv.FormatInt(int64(value), 10), nil
	default:
		return "", ErrInvalidPatchValue
	}
}

func asInt(raw any) (int, error) {
	switch value := raw.(type) {
	case float64:
		return int(value), nil
	case string:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, ErrInvalidPatchValue
		}

		return parsed, nil
	default:
		return 0, ErrInvalidPatchValue
	}
}

func asInt64(raw any) (int64, error) {
	switch value := raw.(type) {
	case float64:
		return int64(value), nil
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, ErrInvalidPatchValue
		}

		return parsed, nil
	default:
		return 0, ErrInvalidPatchValue
	}
}

func asFloat(raw any) (float64, error) {
	switch value := raw.(type) {
	case float64:
		return value, nil
	case string:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, ErrInvalidPatchValue
		}

		return parsed, nil
	default:
		return 0, ErrInvalidPatchValue
	}
}

func asStringSlice(raw any) ([]string, error) {
	switch value := raw.(type) {
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			str, err := asString(item)
			if err != nil {
				return nil, err
			}

			out = append(out, str)
		}

		return out, nil
	case string:
		return splitList(value), nil
	default:
		return nil, ErrInvalidPatchValue
	}
}
