package provider

import "context"

// Art is fetched poster image bytes ready to write under
// /var/lib/sudostream/cache/providers/posters/ (FI-1 L7).
type Art struct {
	Bytes       []byte
	ContentType string
	ExternalID  *string
}

// PosterProvider fetches poster art for one adapter (e.g. AniList, E2).
type PosterProvider interface {
	// Key returns the registry key, e.g. "anilist".
	Key() string
	// FetchShowPoster fetches once per show, keyed by catalog's representative episode
	// (PosterPath convention) for series libraries.
	FetchShowPoster(ctx context.Context, hint ShowHint) (MatchStatus, Art, error)
	// FetchFilmPoster fetches once per file for film libraries.
	FetchFilmPoster(ctx context.Context, hint FilmHint) (MatchStatus, Art, error)
}
