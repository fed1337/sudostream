package metadata

import (
	"slices"
	"strings"
)

const (
	fieldTitle          = "title"
	fieldSortTitle      = "sort_title"
	fieldOriginalTitle  = "original_title"
	fieldEpisodeTitle   = "episode_title"
	fieldShow           = "show"
	fieldSeason         = "season"
	fieldEpisode        = "episode"
	fieldYear           = "year"
	fieldReleaseDate    = "release_date"
	fieldDescription    = "description"
	fieldGenres         = "genres"
	fieldTags           = "tags"
	fieldDirectors      = "directors"
	fieldActors         = "actors"
	fieldWriters        = "writers"
	fieldProducers      = "producers"
	fieldAudioLanguages = "audio_languages"
	fieldStudio         = "studio"
	fieldComposer       = "composer"
	fieldLanguage       = "language"
	fieldCountry        = "country"
	fieldContentRating  = "content_rating"
	fieldDurationSecs   = "duration_seconds"
	fieldVideoCodec     = "video_codec"
	fieldAudioCodec     = "audio_codec"
	fieldWidth          = "width"
	fieldHeight         = "height"
	fieldFrameRate      = "frame_rate"
	fieldBitrate        = "bitrate"
	fieldAudioChannels  = "audio_channels"
	fieldIMDBID         = "imdb_id"
	fieldTMDBID         = "tmdb_id"
	fieldTVDBID         = "tvdb_id"
	fieldTvmazeID       = "tvmaze_id"

	overriddenByUserPrefix     = "user:"
	overriddenByProviderPrefix = "provider:"
)

// EditableFieldKeys are the only keys accepted on PATCH (DB or file).
var EditableFieldKeys = []string{
	fieldTitle,
	fieldSortTitle,
	fieldOriginalTitle,
	fieldEpisodeTitle,
	fieldShow,
	fieldSeason,
	fieldEpisode,
	fieldYear,
	fieldReleaseDate,
	fieldDescription,
	fieldGenres,
	fieldDirectors,
	fieldActors,
	fieldWriters,
	fieldProducers,
	fieldStudio,
	fieldComposer,
	fieldLanguage,
	fieldCountry,
	fieldContentRating,
	fieldIMDBID,
	fieldTMDBID,
	fieldTVDBID,
	fieldTvmazeID,
}

// IsEditableField reports whether key may be written via PATCH.
func IsEditableField(key string) bool {
	return slices.Contains(EditableFieldKeys, key)
}

// FormatUserOverriddenBy builds overridden_by for a user actor.
func FormatUserOverriddenBy(userID string) string {
	return overriddenByUserPrefix + userID
}

// FormatProviderOverriddenBy builds overridden_by for a metadata provider actor (FI-1 L6).
func FormatProviderOverriddenBy(providerKey string) string {
	return overriddenByProviderPrefix + providerKey
}

// IsUserOverridden reports whether overridden_by marks a human edit. Provider enrichment
// skips these rows unless the library allows overwriting user-edited metadata (FI-1 L5).
func IsUserOverridden(overriddenBy *string) bool {
	return overriddenBy != nil && strings.HasPrefix(*overriddenBy, overriddenByUserPrefix)
}
