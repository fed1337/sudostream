package catalog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
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

// IndexedPathLister lists indexed video paths for a library (DB only; no FS walk).
type IndexedPathLister interface {
	ListIndexedPaths(ctx context.Context, libraryID string) ([]string, error)
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

// Service builds film/series catalogs from the metadata index (no request-time FS walks).
type Service struct {
	paths    IndexedPathLister
	access   AccessGateway
	metadata MetadataReader
	posters  PosterIndex
}

// NewService constructs a catalog service.
// paths must list indexed media rows (typically metadata.Service.ListIndexedPaths).
func NewService(
	paths IndexedPathLister,
	accessService AccessGateway,
	metadataService MetadataReader,
) *Service {
	return &Service{
		paths:    paths,
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

	videos, err := s.listVideoPaths(ctx, library.ID)
	if err != nil {
		return ShowDetail{}, err
	}

	detail, ok := s.aggregateShow(ctx, library, showKey, videos)
	if !ok {
		return ShowDetail{}, ErrShowNotFound
	}

	return detail, nil
}

// ListSeasonEpisodes returns episodes for a show season.
// When limit is unset (≤0), the full season is returned (accordion load).
func (s *Service) ListSeasonEpisodes(
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

	videos, err := s.listVideoPaths(ctx, library.ID)
	if err != nil {
		return SeasonEpisodes{}, err
	}

	matchedShow := false
	episodes := make([]Episode, 0)
	posters := s.posterPaths(ctx, library)
	for _, relPath := range videos {
		identity := s.resolveSeries(ctx, library.Type, relPath)
		if identity.showKey != showKey {
			continue
		}
		matchedShow = true
		seasonNum := 0
		if identity.season != nil {
			seasonNum = *identity.season
		}
		if seasonNum != season {
			continue
		}
		episodes = append(episodes, Episode{
			Path:         relPath,
			Title:        identity.display,
			Season:       identity.season,
			Episode:      identity.episode,
			EpisodeTitle: identity.episodeTitle,
			PosterURL:    providerPosterURL(posters, relPath),
			Actions:      videoActions(relPath),
		})
	}
	if !matchedShow {
		return SeasonEpisodes{}, ErrShowNotFound
	}

	sortEpisodes(episodes)

	return pageSeasonEpisodes(season, episodes, opts), nil
}

// SeriesEpisodeIdentity is show/season/episode for a path in a series library.
type SeriesEpisodeIdentity struct {
	ShowKey  string
	ShowName string
	Season   int
	Episode  int
}

// ResolveSeriesEpisode returns series position when the path resolves to S/E.
func (s *Service) ResolveSeriesEpisode(
	ctx context.Context,
	libraryType access.LibraryType,
	relPath string,
) (SeriesEpisodeIdentity, bool) {
	if libraryType != access.LibraryTypeSeries {
		return SeriesEpisodeIdentity{}, false
	}

	resolved := s.resolveSeries(ctx, libraryType, relPath)
	if resolved.showKey == "" || resolved.season == nil || resolved.episode == nil {
		return SeriesEpisodeIdentity{}, false
	}

	return SeriesEpisodeIdentity{
		ShowKey:  resolved.showKey,
		ShowName: resolved.show,
		Season:   *resolved.season,
		Episode:  *resolved.episode,
	}, true
}

func (s *Service) aggregateShow( //nolint:cyclop // season map + poster pick for one show
	ctx context.Context,
	library access.Library,
	showKey string,
	videos []string,
) (ShowDetail, bool) {
	detail := ShowDetail{ShowKey: showKey}
	seasonCounts := map[int]int{}
	posterPath := ""
	for _, relPath := range videos {
		identity := s.resolveSeries(ctx, library.Type, relPath)
		if identity.showKey != showKey {
			continue
		}
		if detail.Name == "" {
			detail.Name = identity.show
		}
		if posterPath == "" || relPath < posterPath {
			posterPath = relPath
		}
		seasonNum := 0
		if identity.season != nil {
			seasonNum = *identity.season
		}
		seasonCounts[seasonNum]++
	}
	if detail.Name == "" && len(seasonCounts) == 0 {
		return ShowDetail{}, false
	}
	if detail.Name == "" {
		detail.Name = showKey
	}
	fillShowSeasons(&detail, seasonCounts)
	detail.PosterPath = posterPath
	if posterPath != "" {
		detail.Actions = videoActions(posterPath)
		detail.PosterURL = providerPosterURL(s.posterPaths(ctx, library), posterPath)
	}

	return detail, true
}

func fillShowSeasons(detail *ShowDetail, seasonCounts map[int]int) {
	seasonNums := make([]int, 0, len(seasonCounts))
	episodeTotal := 0
	for seasonNum, count := range seasonCounts {
		seasonNums = append(seasonNums, seasonNum)
		episodeTotal += count
	}
	sort.Ints(seasonNums)
	for _, seasonNum := range seasonNums {
		detail.Seasons = append(detail.Seasons, SeasonSummary{
			Season:       seasonNum,
			EpisodeCount: seasonCounts[seasonNum],
		})
	}
	detail.SeasonCount = len(seasonNums)
	detail.EpisodeCount = episodeTotal
}

func pageSeasonEpisodes(season int, episodes []Episode, opts mediafs.PageOpts) SeasonEpisodes {
	if opts.Limit <= 0 {
		if opts.Offset < 0 {
			opts.Offset = 0
		}

		return SeasonEpisodes{
			Season:   season,
			Episodes: episodes,
			Total:    len(episodes),
			Limit:    len(episodes),
			Offset:   opts.Offset,
		}
	}

	opts = mediafs.NormalizePageOpts(opts)
	page, meta := mediafs.SlicePage(episodes, opts)

	return SeasonEpisodes{
		Season:   season,
		Episodes: page,
		Total:    meta.Total,
		Limit:    meta.Limit,
		Offset:   meta.Offset,
	}
}

func sortEpisodes(episodes []Episode) {
	sort.Slice(episodes, func(left, right int) bool {
		ei, ej := episodes[left].Episode, episodes[right].Episode
		if ei != nil && ej != nil && *ei != *ej {
			return *ei < *ej
		}
		if ei != nil && ej == nil {
			return true
		}
		if ei == nil && ej != nil {
			return false
		}

		return episodes[left].Path < episodes[right].Path
	})
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

type slimMovie struct {
	path  string
	title string
	year  *int
}

func (s *Service) listMoviesPage(
	ctx context.Context,
	library access.Library,
	opts mediafs.PageOpts,
) ([]Movie, int, error) {
	paths, err := s.listVideoPaths(ctx, library.ID)
	if err != nil {
		return nil, 0, err
	}

	slim := make([]slimMovie, 0, len(paths))
	for _, relPath := range paths {
		title, year := s.resolveFilm(ctx, library.Type, relPath)
		slim = append(slim, slimMovie{path: relPath, title: title, year: year})
	}
	sort.Slice(slim, func(i, j int) bool {
		return strings.ToLower(slim[i].title) < strings.ToLower(slim[j].title)
	})

	pageSlim, meta := mediafs.SlicePage(slim, opts)
	posters := s.posterPaths(ctx, library)
	movies := make([]Movie, 0, len(pageSlim))
	for _, entry := range pageSlim {
		movies = append(movies, Movie{
			Path:      entry.path,
			Title:     entry.title,
			Year:      entry.year,
			PosterURL: providerPosterURL(posters, entry.path),
			Actions:   videoActions(entry.path),
		})
	}

	return movies, meta.Total, nil
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

type slimShow struct {
	key          string
	name         string
	seasonCount  int
	episodeCount int
	posterPath   string
}

func (s *Service) listShowsPage(
	ctx context.Context,
	library access.Library,
	opts mediafs.PageOpts,
) ([]ShowSummary, int, error) {
	paths, err := s.listVideoPaths(ctx, library.ID)
	if err != nil {
		return nil, 0, err
	}

	type agg struct {
		name         string
		seasons      map[int]struct{}
		episodeCount int
		posterPath   string
	}
	byKey := map[string]*agg{}
	for _, relPath := range paths {
		identity := s.resolveSeries(ctx, library.Type, relPath)
		if identity.showKey == "" {
			continue
		}
		entry, ok := byKey[identity.showKey]
		if !ok {
			entry = &agg{
				name:       identity.show,
				seasons:    map[int]struct{}{},
				posterPath: relPath,
			}
			byKey[identity.showKey] = entry
		}
		entry.episodeCount++
		if identity.season != nil {
			entry.seasons[*identity.season] = struct{}{}
		} else {
			entry.seasons[0] = struct{}{}
		}
		if relPath < entry.posterPath {
			entry.posterPath = relPath
		}
	}

	slim := make([]slimShow, 0, len(byKey))
	for key, entry := range byKey {
		slim = append(slim, slimShow{
			key:          key,
			name:         entry.name,
			seasonCount:  len(entry.seasons),
			episodeCount: entry.episodeCount,
			posterPath:   entry.posterPath,
		})
	}
	sort.Slice(slim, func(i, j int) bool {
		return strings.ToLower(slim[i].name) < strings.ToLower(slim[j].name)
	})

	pageSlim, meta := mediafs.SlicePage(slim, opts)
	posters := s.posterPaths(ctx, library)
	shows := make([]ShowSummary, 0, len(pageSlim))
	for _, entry := range pageSlim {
		shows = append(shows, ShowSummary{
			ShowKey:      entry.key,
			Name:         entry.name,
			SeasonCount:  entry.seasonCount,
			EpisodeCount: entry.episodeCount,
			PosterPath:   entry.posterPath,
			PosterURL:    providerPosterURL(posters, entry.posterPath),
			Actions:      videoActions(entry.posterPath),
		})
	}

	return shows, meta.Total, nil
}

func (s *Service) listVideoPaths(ctx context.Context, libraryID string) ([]string, error) {
	if s == nil || s.paths == nil || libraryID == "" {
		return nil, nil
	}

	paths, err := s.paths.ListIndexedPaths(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list indexed library videos: %w", err)
	}

	return paths, nil
}

func (s *Service) resolveFilm(
	ctx context.Context,
	libraryType access.LibraryType,
	relPath string,
) (string, *int) {
	if s.metadata != nil {
		response, err := s.metadata.Get(ctx, relPath)
		if err == nil {
			title := response.DisplayName
			if response.Effective.Title != nil &&
				strings.TrimSpace(*response.Effective.Title) != "" {
				title = strings.TrimSpace(*response.Effective.Title)
			}

			return title, response.Effective.Year
		}
	}

	identity := metadata.ParseFilmIdentity(relPath)
	_ = libraryType

	return identity.Title, identity.Year
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
	// Effective already merges file tags + path heuristics; fall back to path then Original
	// so embedded tags still surface when the filename cannot be parsed.
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
