package provider

import "context"

// ShowHint identifies a series' show for once-per-show matching (FI-1: match granularity —
// show vs episode). Season/episode numbers stay filename-derived unless an episode-capable
// adapter fills episode_title after a show-level match (see FI-1-further-providers).
type ShowHint struct {
	Show string
	Year *int
	IDs  ExternalIDs
}

// FilmHint identifies a single film file for one-match-per-file matching.
type FilmHint struct {
	Title string
	Year  *int
	IDs   ExternalIDs
}

// ExternalIDs holds cross-provider identifiers. ImdbID is a foreign key only — there is no
// imdb registry provider.
type ExternalIDs struct {
	AnilistID *string
	TvdbID    *string
	TmdbID    *string
	TvmazeID  *string
	AnidbID   *string
	ImdbID    *string
}

// ShowFields is the show-level field set a metadata provider can supply, broadcast to every
// episode file in the show (FI-1 apply rules: metadata task, series branch).
type ShowFields struct {
	Title       *string
	Description *string
	Genres      []string
	Studio      *string
	Year        *int
	IDs         ExternalIDs
	// ExternalID is the adapter's primary id (legacy; prefer IDs.*). Still set by AniList.
	ExternalID *string
}

// FilmFields is the per-file field set a metadata provider can supply for film libraries.
type FilmFields struct {
	Title       *string
	Description *string
	Genres      []string
	Studio      *string
	Year        *int
	IDs         ExternalIDs
	ExternalID  *string
}

// MetadataProvider matches and maps metadata fields for one adapter (e.g. AniList, E2).
type MetadataProvider interface {
	// Key returns the registry key, e.g. "anilist".
	Key() string
	// MatchShow matches once per show for series libraries.
	MatchShow(ctx context.Context, hint ShowHint) (MatchStatus, ShowFields, error)
	// MatchFilm matches once per file for film libraries.
	MatchFilm(ctx context.Context, hint FilmHint) (MatchStatus, FilmFields, error)
}
