package provider

import "context"

// EpisodeKey identifies one episode within a show catalog (season + number).
type EpisodeKey struct {
	Season  int
	Episode int
}

// EpisodeInfo is one row from an episode-capable provider catalog.
type EpisodeInfo struct {
	Title string
}

// EpisodeCatalogProvider optionally supplies a per-show episode list so the enricher can write
// episode_title (and confirm S/E) after a show-level MatchShow (FI-1 further providers E4+).
type EpisodeCatalogProvider interface {
	FetchEpisodes(ctx context.Context, fields ShowFields) (map[EpisodeKey]EpisodeInfo, error)
}
