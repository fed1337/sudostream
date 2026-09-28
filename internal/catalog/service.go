package catalog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
)

var (
	// ErrUnsupportedType is returned when the library is not film or series.
	ErrUnsupportedType = errors.New("catalog unsupported for library type")
	// ErrShowNotFound is returned when showKey is unknown in the library.
	ErrShowNotFound = errors.New("show not found")
	// ErrLibraryNotFound is returned when slug does not match a readable library.
	ErrLibraryNotFound = errors.New("library not found")
)

// MetadataReader loads effective metadata for a media path.
type MetadataReader interface {
	Get(ctx context.Context, rawPath string) (metadata.MetadataResponse, error)
}

// Index lists film/series catalog pages from denorm SQL columns (E-33).
type Index interface {
	ListCatalogMoviesPage(
		ctx context.Context,
		libraryID string,
		limit, offset int,
	) ([]metadata.CatalogMovieRow, int, error)
	ListCatalogShowsPage(
		ctx context.Context,
		libraryID string,
		limit, offset int,
	) ([]metadata.CatalogShowRow, int, error)
	GetCatalogShowAgg(
		ctx context.Context,
		libraryID, showKey string,
	) (metadata.CatalogShowAgg, bool, error)
	CatalogShowExists(ctx context.Context, libraryID, showKey string) (bool, error)
	ListCatalogSeasonEpisodes(
		ctx context.Context,
		libraryID, showKey string,
		season, limit, offset int,
	) ([]metadata.CatalogEpisodeRow, int, error)
}

// AccessGateway resolves libraries and read ACL.
type AccessGateway interface {
	ListReadableLibraries(ctx context.Context, user auth.PublicUser) ([]access.Library, error)
	CanRead(ctx context.Context, user auth.PublicUser, rawPath string) (bool, error)
}

// PosterIndex reports which media paths of a library have a locally cached provider poster.
type PosterIndex interface {
	PosterPaths(ctx context.Context, libraryID string) (map[string]struct{}, error)
}

// Service builds film/series catalogs from denorm metadata columns (no library-wide scans).
type Service struct {
	index    Index
	access   AccessGateway
	metadata MetadataReader
	posters  PosterIndex
}

// NewService constructs a catalog service.
// index must implement denorm list/agg queries (typically metadata.Service).
func NewService(
	index Index,
	accessService AccessGateway,
	metadataService MetadataReader,
) *Service {
	return &Service{
		index:    index,
		access:   accessService,
		metadata: metadataService,
	}
}

// SetPosterIndex enables provider poster URLs on catalog cards. Safe to call with nil.
func (s *Service) SetPosterIndex(posters PosterIndex) {
	if s == nil {
		return
	}
	s.posters = posters
}

// GetLibraryCatalog returns one page of movies or shows for a library slug.
func (s *Service) GetLibraryCatalog(
	ctx context.Context,
	user auth.PublicUser,
	librarySlug string,
	opts mediafs.PageOpts,
) (LibraryCatalog, error) {
	library, err := s.libraryBySlug(ctx, user, librarySlug)
	if err != nil {
		return LibraryCatalog{}, err
	}

	opts = mediafs.NormalizePageOpts(opts)

	switch library.Type {
	case access.LibraryTypeFilm:
		movies, total, listErr := s.listMoviesPage(ctx, library, opts)
		if listErr != nil {
			return LibraryCatalog{}, listErr
		}

		return LibraryCatalog{
			Type:   library.Type,
			Movies: movies,
			Total:  total,
			Limit:  opts.Limit,
			Offset: opts.Offset,
		}, nil
	case access.LibraryTypeSeries:
		shows, total, listErr := s.listShowsPage(ctx, library, opts)
		if listErr != nil {
			return LibraryCatalog{}, listErr
		}

		return LibraryCatalog{
			Type:   library.Type,
			Shows:  shows,
			Total:  total,
			Limit:  opts.Limit,
			Offset: opts.Offset,
		}, nil
	case access.LibraryTypeMusic, access.LibraryTypePhotos, access.LibraryTypeOther:
		return LibraryCatalog{}, ErrUnsupportedType
	default:
		return LibraryCatalog{}, ErrUnsupportedType
	}
}

// GetShow returns season summaries (no episodes) for one show in a series library.
func (s *Service) GetShow(
	ctx context.Context,
	user auth.PublicUser,
	librarySlug, showKey string,
) (ShowDetail, error) {
	library, err := s.libraryBySlug(ctx, user, librarySlug)
	if err != nil {
		return ShowDetail{}, err
	}
	if library.Type != access.LibraryTypeSeries {
		return ShowDetail{}, ErrUnsupportedType
	}
	if s.index == nil {
		return ShowDetail{}, ErrShowNotFound
	}

	agg, ok, aggErr := s.index.GetCatalogShowAgg(ctx, library.ID, showKey)
	if aggErr != nil {
		return ShowDetail{}, fmt.Errorf("catalog show: %w", aggErr)
	}
	if !ok {
		return ShowDetail{}, ErrShowNotFound
	}

	detail := ShowDetail{
		ShowKey:      agg.ShowKey,
		Name:         agg.Name,
		PosterPath:   agg.PosterPath,
		SeasonCount:  len(agg.Seasons),
		EpisodeCount: 0,
		Seasons:      make([]SeasonSummary, 0, len(agg.Seasons)),
	}
	if detail.Name == "" {
		detail.Name = showKey
	}
	for _, season := range agg.Seasons {
		detail.EpisodeCount += season.EpisodeCount
		detail.Seasons = append(detail.Seasons, SeasonSummary{
			Season:       season.Season,
			EpisodeCount: season.EpisodeCount,
		})
	}
	if agg.PosterPath != "" {
		detail.Actions = videoActions(agg.PosterPath)
		detail.PosterURL = providerPosterURL(s.posterPaths(ctx, library), agg.PosterPath)
	}

	return detail, nil
}

// ListSeasonEpisodes returns episodes for a show season.
// When limit is unset (≤0), the full season is returned (accordion load).
func (s *Service) ListSeasonEpisodes( //nolint:cyclop // ACL + existence + page opts + row map
	ctx context.Context,
	user auth.PublicUser,
	librarySlug, showKey string,
	season int,
	opts mediafs.PageOpts,
) (SeasonEpisodes, error) {
	library, err := s.libraryBySlug(ctx, user, librarySlug)
	if err != nil {
		return SeasonEpisodes{}, err
	}
	if library.Type != access.LibraryTypeSeries {
		return SeasonEpisodes{}, ErrUnsupportedType
	}
	if s.index == nil {
		return SeasonEpisodes{}, ErrShowNotFound
	}

	exists, existsErr := s.index.CatalogShowExists(ctx, library.ID, showKey)
	if existsErr != nil {
		return SeasonEpisodes{}, fmt.Errorf("catalog show exists: %w", existsErr)
	}
	if !exists {
		return SeasonEpisodes{}, ErrShowNotFound
	}

	limit, offset := 0, opts.Offset
	if opts.Limit > 0 {
		opts = mediafs.NormalizePageOpts(opts)
		limit, offset = opts.Limit, opts.Offset
	} else if offset < 0 {
		offset = 0
	}

	rows, total, listErr := s.index.ListCatalogSeasonEpisodes(
		ctx, library.ID, showKey, season, limit, offset,
	)
	if listErr != nil {
		return SeasonEpisodes{}, fmt.Errorf("catalog season episodes: %w", listErr)
	}

	posters := s.posterPaths(ctx, library)
	episodes := make([]Episode, 0, len(rows))
	for _, row := range rows {
		title := row.Title
		if title == "" {
			title = filepath.Base(row.RelPath)
		}
		episodes = append(episodes, Episode{
			Path:         row.RelPath,
			Title:        title,
			Season:       row.Season,
			Episode:      row.Episode,
			EpisodeTitle: row.EpisodeTitle,
			PosterURL:    providerPosterURL(posters, row.RelPath),
			Actions:      videoActions(row.RelPath),
		})
	}

	outLimit := limit
	if outLimit <= 0 {
		outLimit = total
	}

	return SeasonEpisodes{
		Season:   season,
		Episodes: episodes,
		Total:    total,
		Limit:    outLimit,
		Offset:   offset,
	}, nil
}

// SeriesEpisodeIdentity is show/season/episode for a path in a series library.
type SeriesEpisodeIdentity struct {
	ShowKey  string
	ShowName string
	Season   int
	Episode  int
}

// ResolveSeriesEpisode returns series position for a path in a series library.
// When season/episode cannot be parsed (specials / extras), they default to 0 so
// player chrome still mounts and can browse other seasons of the same show.
func (s *Service) ResolveSeriesEpisode(
	ctx context.Context,
	libraryType access.LibraryType,
	relPath string,
) (SeriesEpisodeIdentity, bool) {
	if libraryType != access.LibraryTypeSeries {
		return SeriesEpisodeIdentity{}, false
	}

	resolved := s.resolveSeries(ctx, libraryType, relPath)
	if resolved.showKey == "" {
		return SeriesEpisodeIdentity{}, false
	}

	season, episode := 0, 0
	if resolved.season != nil {
		season = *resolved.season
	}
	if resolved.episode != nil {
		episode = *resolved.episode
	}

	return SeriesEpisodeIdentity{
		ShowKey:  resolved.showKey,
		ShowName: resolved.show,
		Season:   season,
		Episode:  episode,
	}, true
}

func (s *Service) libraryBySlug(
	ctx context.Context,
	user auth.PublicUser,
	slug string,
) (access.Library, error) {
	if s == nil || s.access == nil {
		return access.Library{}, ErrLibraryNotFound
	}

	libraries, err := s.access.ListReadableLibraries(ctx, user)
	if err != nil {
		return access.Library{}, fmt.Errorf("list libraries: %w", err)
	}
	for _, library := range libraries {
		if library.Slug == slug || library.RelPath == slug {
			return library, nil
		}
		if slices.Contains(library.RootPaths(), slug) {
			return library, nil
		}
	}

	return access.Library{}, ErrLibraryNotFound
}

func (s *Service) listMoviesPage(
	ctx context.Context,
	library access.Library,
	opts mediafs.PageOpts,
) ([]Movie, int, error) {
	if s.index == nil {
		return nil, 0, nil
	}

	rows, total, err := s.index.ListCatalogMoviesPage(ctx, library.ID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list catalog movies: %w", err)
	}

	posters := s.posterPaths(ctx, library)
	movies := make([]Movie, 0, len(rows))
	for _, row := range rows {
		title := row.Title
		if title == "" {
			title = filepath.Base(row.RelPath)
		}
		movies = append(movies, Movie{
			Path:      row.RelPath,
			Title:     title,
			Year:      row.Year,
			PosterURL: providerPosterURL(posters, row.RelPath),
			Actions:   videoActions(row.RelPath),
		})
	}

	return movies, total, nil
}

// posterPaths loads the library's cached provider posters in one query so cards can prefer
// them over generated thumbnails. Failures degrade to thumbnails only.
func (s *Service) posterPaths(ctx context.Context, library access.Library) map[string]struct{} {
	if s.posters == nil {
		return nil
	}

	paths, err := s.posters.PosterPaths(ctx, library.ID)
	if err != nil {
		return nil
	}

	return paths
}

func providerPosterURL(posters map[string]struct{}, relPath string) string {
	if _, ok := posters[relPath]; !ok {
		return ""
	}

	return "/api/provider-poster/" + mediafs.EscapePathSegments(relPath)
}

func (s *Service) listShowsPage(
	ctx context.Context,
	library access.Library,
	opts mediafs.PageOpts,
) ([]ShowSummary, int, error) {
	if s.index == nil {
		return nil, 0, nil
	}

	rows, total, err := s.index.ListCatalogShowsPage(ctx, library.ID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list catalog shows: %w", err)
	}

	posters := s.posterPaths(ctx, library)
	shows := make([]ShowSummary, 0, len(rows))
	for _, row := range rows {
		shows = append(shows, ShowSummary{
			ShowKey:      row.ShowKey,
			Name:         row.Name,
			SeasonCount:  row.SeasonCount,
			EpisodeCount: row.EpisodeCount,
			PosterPath:   row.PosterPath,
			PosterURL:    providerPosterURL(posters, row.PosterPath),
			Actions:      videoActions(row.PosterPath),
		})
	}

	return shows, total, nil
}

type seriesResolved struct {
	show, showKey, display, episodeTitle string
	season, episode                      *int
}

func (s *Service) resolveSeries(
	ctx context.Context,
	libraryType access.LibraryType,
	relPath string,
) seriesResolved {
	_ = libraryType
	if s.metadata == nil {
		return seriesFromPath(relPath)
	}

	response, err := s.metadata.Get(ctx, relPath)
	if err != nil {
		return seriesFromPath(relPath)
	}

	parsed := metadata.ParseSeriesIdentity(relPath)
	show := firstNonEmptyString(
		trimPtr(response.Effective.Show),
		parsed.Show,
		trimPtr(response.Source.Original.Show),
	)
	episodeTitle := firstNonEmptyString(
		trimPtr(response.Effective.EpisodeTitle),
		parsed.EpisodeTitle,
		trimPtr(response.Source.Original.EpisodeTitle),
	)
	showKey := metadata.NormalizeShowKey(show)
	display := response.DisplayName
	if display == "" {
		display = filepath.Base(relPath)
	}

	season := firstNonNilInt(
		response.Effective.Season,
		parsed.Season,
		response.Source.Original.Season,
	)
	episode := firstNonNilInt(
		response.Effective.Episode,
		parsed.Episode,
		response.Source.Original.Episode,
	)

	return seriesResolved{
		show:         show,
		showKey:      showKey,
		display:      display,
		episodeTitle: episodeTitle,
		season:       season,
		episode:      episode,
	}
}

func trimPtr(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func firstNonNilInt(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}

	return nil
}

func seriesFromPath(relPath string) seriesResolved {
	parsed := metadata.ParseSeriesIdentity(relPath)
	display := parsed.Show
	if parsed.Season != nil && parsed.Episode != nil {
		display = fmt.Sprintf(
			"%s — S%02dE%02d",
			parsed.Show,
			*parsed.Season,
			*parsed.Episode,
		)
	}

	return seriesResolved{
		show:    parsed.Show,
		showKey: parsed.ShowKey,
		display: display,
		season:  parsed.Season,
		episode: parsed.Episode,
	}
}

func videoActions(relPath string) mediafs.Action {
	escaped := mediafs.EscapePathSegments(relPath)

	return mediafs.Action{
		Play:      "/api/play/" + escaped + "/master.m3u8",
		Thumbnail: "/api/thumbnail/" + escaped,
		Download:  "/api/download/" + escaped,
	}
}
