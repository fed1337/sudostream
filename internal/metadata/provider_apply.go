package metadata

import (
	"context"
	"strings"
)

// ProviderFields is the metadata subset an FI-1 provider may write. EpisodeTitle (and optional
// Season/Episode confirmation) are filled by episode-capable adapters after a show-level match
// (see plans/FI-1-further-providers.md). AniList stays show-level only.
type ProviderFields struct {
	Title        *string
	Show         *string
	Description  *string
	Genres       []string
	Studio       *string
	Year         *int
	EpisodeTitle *string
	Season       *int
	Episode      *int
	IMDBID       *string
	TMDBID       *string
	TVDBID       *string
	TvmazeID     *string
}

// ApplyProviderFields writes provider-supplied fields for a media path and stamps
// overridden_by = provider:<key>. Nil/blank fields are left untouched, so callers decide
// fill-missing vs full-rewrite semantics before calling.
func (s *Service) ApplyProviderFields(
	ctx context.Context,
	rawPath string,
	target PatchTarget,
	fields ProviderFields,
	providerKey string,
) (MetadataResponse, error) {
	patch := providerPatchFields(fields)
	if len(patch) == 0 {
		return s.Get(ctx, rawPath)
	}

	return s.patch(
		ctx,
		rawPath,
		PatchRequest{Target: target, Fields: patch},
		FormatProviderOverriddenBy(providerKey),
	)
}

func providerPatchFields(fields ProviderFields) map[string]*jsonValue {
	patch := map[string]*jsonValue{}

	putProviderString(patch, fieldTitle, fields.Title)
	putProviderString(patch, fieldShow, fields.Show)
	putProviderString(patch, fieldDescription, fields.Description)
	putProviderString(patch, fieldStudio, fields.Studio)
	putProviderString(patch, fieldEpisodeTitle, fields.EpisodeTitle)
	putProviderString(patch, fieldIMDBID, fields.IMDBID)
	putProviderString(patch, fieldTMDBID, fields.TMDBID)
	putProviderString(patch, fieldTVDBID, fields.TVDBID)
	putProviderString(patch, fieldTvmazeID, fields.TvmazeID)

	if fields.Year != nil {
		patch[fieldYear] = &jsonValue{raw: float64(*fields.Year)}
	}
	if fields.Season != nil {
		patch[fieldSeason] = &jsonValue{raw: float64(*fields.Season)}
	}
	if fields.Episode != nil {
		patch[fieldEpisode] = &jsonValue{raw: float64(*fields.Episode)}
	}

	genres := make([]any, 0, len(fields.Genres))
	for _, genre := range fields.Genres {
		trimmed := strings.TrimSpace(genre)
		if trimmed != "" {
			genres = append(genres, trimmed)
		}
	}
	if len(genres) > 0 {
		patch[fieldGenres] = &jsonValue{raw: genres}
	}

	return patch
}

func putProviderString(patch map[string]*jsonValue, key string, value *string) {
	if value == nil {
		return
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return
	}

	patch[key] = &jsonValue{raw: trimmed}
}
