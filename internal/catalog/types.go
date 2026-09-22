// Package catalog builds film/series library views from the metadata index and path heuristics.
// Request paths do not walk the media filesystem; discovery uses indexed rows only.
package catalog

import (
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
)

// Movie is one film catalog card. PosterURL is set only when a provider poster is cached
// locally; clients fall back to Actions.Thumbnail otherwise (FI-1 display rule).
type Movie struct {
	Path      string         `json:"path"`
	Title     string         `json:"title"`
	Year      *int           `json:"year,omitempty"`
	Watched   *bool          `json:"watched,omitempty"`
	Favorited *bool          `json:"favorited,omitempty"`
	PosterURL string         `json:"posterUrl,omitempty"`
	Actions   mediafs.Action `json:"actions"`
}

// ShowSummary is one series catalog card.
type ShowSummary struct {
	ShowKey      string         `json:"showKey"`
	Name         string         `json:"name"`
	SeasonCount  int            `json:"seasonCount"`
	EpisodeCount int            `json:"episodeCount"`
	PosterPath   string         `json:"posterPath"`
	PosterURL    string         `json:"posterUrl,omitempty"`
	Actions      mediafs.Action `json:"actions"`
}

// Episode is one playable episode row.
type Episode struct {
	Path         string         `json:"path"`
	Title        string         `json:"title"`
	Season       *int           `json:"season,omitempty"`
	Episode      *int           `json:"episode,omitempty"`
	EpisodeTitle string         `json:"episodeTitle,omitempty"`
	Watched      *bool          `json:"watched,omitempty"`
	Favorited    *bool          `json:"favorited,omitempty"`
	PosterURL    string         `json:"posterUrl,omitempty"`
	Actions      mediafs.Action `json:"actions"`
}

// SeasonSummary is one season header without embedded episodes.
type SeasonSummary struct {
	Season       int `json:"season"`
	EpisodeCount int `json:"episodeCount"`
}

// ShowDetail is the series detail payload (seasons only).
// PosterURL is set when a provider poster is cached for PosterPath (representative episode).
type ShowDetail struct {
	ShowKey      string          `json:"showKey"`
	Name         string          `json:"name"`
	SeasonCount  int             `json:"seasonCount"`
	EpisodeCount int             `json:"episodeCount"`
	PosterPath   string          `json:"posterPath,omitempty"`
	PosterURL    string          `json:"posterUrl,omitempty"`
	Actions      mediafs.Action  `json:"actions"`
	Seasons      []SeasonSummary `json:"seasons"`
}

// SeasonEpisodes is one season's episode page.
type SeasonEpisodes struct {
	Season   int       `json:"season"`
	Episodes []Episode `json:"episodes"`
	Total    int       `json:"total"`
	Limit    int       `json:"limit"`
	Offset   int       `json:"offset"`
}

// LibraryCatalog is the typed catalog response for a library.
type LibraryCatalog struct {
	Type   access.LibraryType `json:"type"`
	Movies []Movie            `json:"movies,omitempty"`
	Shows  []ShowSummary      `json:"shows,omitempty"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}
