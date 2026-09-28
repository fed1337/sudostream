package metadata

// CatalogMovieRow is one film card from denorm SQL (E-33).
type CatalogMovieRow struct {
	RelPath string
	Title   string
	Year    *int
}

// CatalogShowRow is one series card from denorm SQL (E-33).
type CatalogShowRow struct {
	ShowKey      string
	Name         string
	SeasonCount  int
	EpisodeCount int
	PosterPath   string
}

// CatalogSeasonCount is episode count for one season.
type CatalogSeasonCount struct {
	Season       int
	EpisodeCount int
}

// CatalogShowAgg is show detail aggregation for one show_key.
type CatalogShowAgg struct {
	ShowKey    string
	Name       string
	PosterPath string
	Seasons    []CatalogSeasonCount
}

// CatalogEpisodeRow is one season episode from denorm SQL (E-33).
type CatalogEpisodeRow struct {
	RelPath      string
	Title        string
	Season       *int
	Episode      *int
	EpisodeTitle string
}
