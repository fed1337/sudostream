// Package tmdb implements the TMDB metadata and poster provider (FI-1 E5).
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sudoStream/internal/provider"
	"time"
)

// Key is the registry key for this adapter.
const Key = "tmdb"

const (
	defaultBaseURL     = "https://api.themoviedb.org/3"
	defaultImageBase   = "https://image.tmdb.org/t/p"
	defaultPosterSize  = "w780"
	defaultMinInterval = 250 * time.Millisecond
	maxPosterBytes     = 8 << 20
)

// Env keys (FI-1 L13). Either API key or read-access token is enough.
const (
	//nolint:gosec // G101: env var name
	EnvAPIKey = "SUDOSTREAM_TMDB_API_KEY"
	//nolint:gosec // G101: env var name
	EnvAccessToken = "SUDOSTREAM_TMDB_ACCESS_TOKEN"
)

var (
	// ErrMissingCredentials is returned when neither API key nor access token is set.
	ErrMissingCredentials = errors.New("tmdb credentials missing")
	// ErrRequestFailed is returned on unexpected HTTP statuses.
	ErrRequestFailed = errors.New("tmdb request failed")
	// ErrPosterEmpty is returned when a match has no poster path.
	ErrPosterEmpty = errors.New("tmdb poster empty")
	// ErrMissingShowID is returned when FetchEpisodes lacks a TMDB id.
	ErrMissingShowID = errors.New("tmdb show id missing")

	errNotFound = errors.New("tmdb not found")
)

// Provider queries api.themoviedb.org.
type Provider struct {
	baseURL      string
	imageBaseURL string
	posterSize   string
	apiKey       string
	accessToken  string
	http         *provider.ThrottledClient
}

// Option customizes a Provider.
type Option func(*Provider)

// WithBaseURL overrides the API base URL (tests).
func WithBaseURL(base string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimRight(base, "/") }
}

// WithImageBaseURL overrides the image CDN base (tests).
func WithImageBaseURL(base string) Option {
	return func(p *Provider) { p.imageBaseURL = strings.TrimRight(base, "/") }
}

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.http = p.http.WithHTTPClient(client)
	}
}

// WithMinInterval overrides client-side spacing (tests use 0).
func WithMinInterval(interval time.Duration) Option {
	return func(p *Provider) {
		p.http = provider.NewThrottledClient(interval).WithProvider("tmdb")
	}
}

// WithCredentials sets API key and/or bearer token explicitly (tests).
func WithCredentials(apiKey, accessToken string) Option {
	return func(p *Provider) {
		p.apiKey = strings.TrimSpace(apiKey)
		p.accessToken = strings.TrimSpace(accessToken)
	}
}

// New constructs the TMDB adapter.
func New(opts ...Option) *Provider {
	instance := &Provider{
		baseURL:      defaultBaseURL,
		imageBaseURL: defaultImageBase,
		posterSize:   defaultPosterSize,
		apiKey:       provider.Env(EnvAPIKey),
		accessToken:  provider.Env(EnvAccessToken),
		http:         provider.NewThrottledClient(defaultMinInterval).WithProvider("tmdb"),
	}
	for _, opt := range opts {
		opt(instance)
	}

	return instance
}

// Key returns the registry key.
func (p *Provider) Key() string { return Key }

// RequiredEnv lists credential env vars. Either key or token satisfies MissingEnvFor.
func (p *Provider) RequiredEnv() []string {
	if strings.TrimSpace(p.apiKey) != "" || strings.TrimSpace(p.accessToken) != "" {
		return nil
	}
	if provider.Env(EnvAPIKey) != "" || provider.Env(EnvAccessToken) != "" {
		return nil
	}

	return []string{EnvAPIKey}
}

// MatchShow matches a TV series once per show.
func (p *Provider) MatchShow(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.ShowFields, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.ShowFields{}, err
	}
	show, status, err := p.resolveTV(ctx, hint)
	if err != nil || status != provider.MatchOK {
		return status, provider.ShowFields{}, err
	}

	return provider.MatchOK, show.toShowFields(), nil
}

// MatchFilm matches a movie once per file.
func (p *Provider) MatchFilm(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.FilmFields, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.FilmFields{}, err
	}
	movie, status, err := p.resolveMovie(ctx, hint)
	if err != nil || status != provider.MatchOK {
		return status, provider.FilmFields{}, err
	}

	return provider.MatchOK, movie.toFilmFields(), nil
}

// FetchShowPoster downloads the series poster.
func (p *Provider) FetchShowPoster(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.Art, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.Art{}, err
	}
	show, status, err := p.resolveTV(ctx, hint)
	if err != nil || status != provider.MatchOK {
		return status, provider.Art{}, err
	}

	return p.artFromPath(ctx, show.PosterPath, show.ID)
}

// FetchFilmPoster downloads the movie poster.
func (p *Provider) FetchFilmPoster(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.Art, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.Art{}, err
	}
	movie, status, err := p.resolveMovie(ctx, hint)
	if err != nil || status != provider.MatchOK {
		return status, provider.Art{}, err
	}

	return p.artFromPath(ctx, movie.PosterPath, movie.ID)
}

// FetchEpisodes loads season episode lists once per show.
func (p *Provider) FetchEpisodes(
	ctx context.Context,
	fields provider.ShowFields,
) (map[provider.EpisodeKey]provider.EpisodeInfo, error) {
	err := p.ensureCreds()
	if err != nil {
		return nil, err
	}
	showID := tmdbIDFromFields(fields)
	if showID == "" {
		return nil, ErrMissingShowID
	}

	body, err := p.get(ctx, "/tv/"+url.PathEscape(showID))
	if err != nil {
		return nil, err
	}
	var detail tvDetailDTO
	err = json.Unmarshal(body, &detail)
	if err != nil {
		return nil, fmt.Errorf("decode tmdb tv detail: %w", err)
	}

	return p.mapShowEpisodes(ctx, showID, detail.Seasons)
}

func (p *Provider) mapShowEpisodes(
	ctx context.Context,
	showID string,
	seasons []seasonSummaryDTO,
) (map[provider.EpisodeKey]provider.EpisodeInfo, error) {
	out := make(map[provider.EpisodeKey]provider.EpisodeInfo)
	for _, season := range seasons {
		if season.SeasonNumber < 0 {
			continue
		}
		err := p.appendSeasonEpisodes(ctx, showID, season.SeasonNumber, out)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

func (p *Provider) appendSeasonEpisodes(
	ctx context.Context,
	showID string,
	seasonNumber int,
	out map[provider.EpisodeKey]provider.EpisodeInfo,
) error {
	seasonBody, err := p.get(
		ctx,
		"/tv/"+url.PathEscape(showID)+"/season/"+strconv.Itoa(seasonNumber),
	)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}

		return err
	}
	var seasonDTO seasonDetailDTO
	err = json.Unmarshal(seasonBody, &seasonDTO)
	if err != nil {
		return fmt.Errorf("decode tmdb season: %w", err)
	}
	for _, ep := range seasonDTO.Episodes {
		title := strings.TrimSpace(ep.Name)
		if ep.EpisodeNumber <= 0 || title == "" {
			continue
		}
		out[provider.EpisodeKey{Season: seasonNumber, Episode: ep.EpisodeNumber}] = provider.EpisodeInfo{
			Title: title,
		}
	}

	return nil
}

func (p *Provider) ensureCreds() error {
	if strings.TrimSpace(p.apiKey) == "" {
		p.apiKey = provider.Env(EnvAPIKey)
	}
	if strings.TrimSpace(p.accessToken) == "" {
		p.accessToken = provider.Env(EnvAccessToken)
	}
	if p.apiKey == "" && p.accessToken == "" {
		return fmt.Errorf("%w: set %s or %s", ErrMissingCredentials, EnvAPIKey, EnvAccessToken)
	}

	return nil
}

func tmdbIDFromFields(fields provider.ShowFields) string {
	if fields.IDs.TmdbID != nil {
		return strings.TrimSpace(*fields.IDs.TmdbID)
	}
	if fields.ExternalID != nil {
		return strings.TrimSpace(*fields.ExternalID)
	}

	return ""
}

func (p *Provider) resolveTV(
	ctx context.Context,
	hint provider.ShowHint,
) (tvDetailDTO, provider.MatchStatus, error) {
	if hint.IDs.TmdbID != nil && strings.TrimSpace(*hint.IDs.TmdbID) != "" {
		return p.getTV(ctx, strings.TrimSpace(*hint.IDs.TmdbID))
	}
	found, status, err := p.findTVByExternal(ctx, hint.IDs)
	if status != provider.MatchNone || err != nil {
		return found, status, err
	}

	query := strings.TrimSpace(hint.Show)
	if query == "" {
		return tvDetailDTO{}, provider.MatchNone, nil
	}

	return p.searchTV(ctx, query, hint.Year)
}

func (p *Provider) resolveMovie(
	ctx context.Context,
	hint provider.FilmHint,
) (movieDetailDTO, provider.MatchStatus, error) {
	if hint.IDs.TmdbID != nil && strings.TrimSpace(*hint.IDs.TmdbID) != "" {
		return p.getMovie(ctx, strings.TrimSpace(*hint.IDs.TmdbID))
	}
	found, status, err := p.findMovieByExternal(ctx, hint.IDs)
	if status != provider.MatchNone || err != nil {
		return found, status, err
	}

	query := strings.TrimSpace(hint.Title)
	if query == "" {
		return movieDetailDTO{}, provider.MatchNone, nil
	}

	return p.searchMovie(ctx, query, hint.Year)
}

func (p *Provider) findTVByExternal(
	ctx context.Context,
	ids provider.ExternalIDs,
) (tvDetailDTO, provider.MatchStatus, error) {
	tmdbID, status, err := p.findExternalID(ctx, ids, true)
	if err != nil || status != provider.MatchOK || tmdbID == 0 {
		return tvDetailDTO{}, status, err
	}

	return p.getTV(ctx, strconv.Itoa(tmdbID))
}

func (p *Provider) findMovieByExternal(
	ctx context.Context,
	ids provider.ExternalIDs,
) (movieDetailDTO, provider.MatchStatus, error) {
	tmdbID, status, err := p.findExternalID(ctx, ids, false)
	if err != nil || status != provider.MatchOK || tmdbID == 0 {
		return movieDetailDTO{}, status, err
	}

	return p.getMovie(ctx, strconv.Itoa(tmdbID))
}

func (p *Provider) findExternalID(
	ctx context.Context,
	ids provider.ExternalIDs,
	preferTV bool,
) (int, provider.MatchStatus, error) {
	if ids.ImdbID != nil && strings.TrimSpace(*ids.ImdbID) != "" {
		return p.findBySource(ctx, strings.TrimSpace(*ids.ImdbID), "imdb_id", preferTV)
	}
	if preferTV && ids.TvdbID != nil && strings.TrimSpace(*ids.TvdbID) != "" {
		return p.findBySource(ctx, strings.TrimSpace(*ids.TvdbID), "tvdb_id", true)
	}

	return 0, provider.MatchNone, nil
}

func (p *Provider) findBySource(
	ctx context.Context,
	externalID, source string,
	preferTV bool,
) (int, provider.MatchStatus, error) {
	body, err := p.get(ctx, "/find/"+url.PathEscape(externalID)+"?"+url.Values{
		"external_source": {source},
	}.Encode())
	if err != nil {
		if errors.Is(err, errNotFound) {
			return 0, provider.MatchNone, nil
		}

		return 0, provider.MatchNone, err
	}
	var payload findDTO
	err = json.Unmarshal(body, &payload)
	if err != nil {
		return 0, provider.MatchNone, fmt.Errorf("decode tmdb find: %w", err)
	}
	results := payload.MovieResults
	if preferTV {
		results = payload.TVResults
	}
	if len(results) == 0 || results[0].ID == 0 {
		return 0, provider.MatchNone, nil
	}

	return results[0].ID, provider.MatchOK, nil
}

func (p *Provider) getTV(ctx context.Context, id string) (tvDetailDTO, provider.MatchStatus, error) {
	body, err := p.get(ctx, "/tv/"+url.PathEscape(id)+"?append_to_response=external_ids")
	if err != nil {
		if errors.Is(err, errNotFound) {
			return tvDetailDTO{}, provider.MatchNone, nil
		}

		return tvDetailDTO{}, provider.MatchNone, err
	}
	var show tvDetailDTO
	err = json.Unmarshal(body, &show)
	if err != nil {
		return tvDetailDTO{}, provider.MatchNone, fmt.Errorf("decode tmdb tv: %w", err)
	}
	if show.ID == 0 {
		return tvDetailDTO{}, provider.MatchNone, nil
	}

	return show, provider.MatchOK, nil
}

func (p *Provider) getMovie(ctx context.Context, id string) (movieDetailDTO, provider.MatchStatus, error) {
	body, err := p.get(ctx, "/movie/"+url.PathEscape(id)+"?append_to_response=external_ids")
	if err != nil {
		if errors.Is(err, errNotFound) {
			return movieDetailDTO{}, provider.MatchNone, nil
		}

		return movieDetailDTO{}, provider.MatchNone, err
	}
	var movie movieDetailDTO
	err = json.Unmarshal(body, &movie)
	if err != nil {
		return movieDetailDTO{}, provider.MatchNone, fmt.Errorf("decode tmdb movie: %w", err)
	}
	if movie.ID == 0 {
		return movieDetailDTO{}, provider.MatchNone, nil
	}

	return movie, provider.MatchOK, nil
}

func (p *Provider) searchTV(
	ctx context.Context,
	query string,
	year *int,
) (tvDetailDTO, provider.MatchStatus, error) {
	status, tmdbID, err := p.searchID(ctx, "tv", "first_air_date_year", query, year, tvYear)
	if err != nil || status != provider.MatchOK || tmdbID == 0 {
		return tvDetailDTO{}, status, err
	}

	return p.getTV(ctx, strconv.Itoa(tmdbID))
}

func (p *Provider) searchMovie(
	ctx context.Context,
	query string,
	year *int,
) (movieDetailDTO, provider.MatchStatus, error) {
	status, tmdbID, err := p.searchID(ctx, "movie", "year", query, year, movieYear)
	if err != nil || status != provider.MatchOK || tmdbID == 0 {
		return movieDetailDTO{}, status, err
	}

	return p.getMovie(ctx, strconv.Itoa(tmdbID))
}

func (p *Provider) searchID(
	ctx context.Context,
	kind, yearParam, query string,
	year *int,
	yearOf func(searchResultDTO) *int,
) (provider.MatchStatus, int, error) {
	values := url.Values{"query": {query}}
	if year != nil {
		values.Set(yearParam, strconv.Itoa(*year))
	}
	body, err := p.get(ctx, "/search/"+kind+"?"+values.Encode())
	if err != nil {
		return provider.MatchNone, 0, err
	}
	var payload searchDTO
	err = json.Unmarshal(body, &payload)
	if err != nil {
		return provider.MatchNone, 0, fmt.Errorf("decode tmdb %s search: %w", kind, err)
	}
	status, tmdbID := pickSearchResult(payload.Results, year, yearOf)

	return status, tmdbID, nil
}

func pickSearchResult(
	results []searchResultDTO,
	year *int,
	yearOf func(searchResultDTO) *int,
) (provider.MatchStatus, int) {
	if len(results) == 0 {
		return provider.MatchNone, 0
	}
	candidates := filterSearchCandidates(results, year, yearOf)
	switch len(candidates) {
	case 0:
		if len(results) == 1 && results[0].ID != 0 {
			return provider.MatchUncertain, results[0].ID
		}

		return provider.MatchNone, 0
	case 1:
		return provider.MatchOK, candidates[0].ID
	default:
		return provider.MatchUncertain, 0
	}
}

func filterSearchCandidates(
	results []searchResultDTO,
	year *int,
	yearOf func(searchResultDTO) *int,
) []searchResultDTO {
	candidates := make([]searchResultDTO, 0, len(results))
	for _, row := range results {
		if row.ID == 0 {
			continue
		}
		if year != nil {
			got := yearOf(row)
			if got != nil && *got != *year {
				continue
			}
		}
		candidates = append(candidates, row)
	}

	return candidates
}

func tvYear(row searchResultDTO) *int {
	return yearFromDate(row.FirstAirDate)
}

func movieYear(row searchResultDTO) *int {
	return yearFromDate(row.ReleaseDate)
}

func yearFromDate(raw string) *int {
	const yearPrefixLen = 4
	if len(raw) < yearPrefixLen {
		return nil
	}
	year, err := strconv.Atoi(raw[:yearPrefixLen])
	if err != nil {
		return nil
	}

	return &year
}

func (p *Provider) artFromPath(
	ctx context.Context,
	posterPath string,
	tmdbID int,
) (provider.MatchStatus, provider.Art, error) {
	posterPath = strings.TrimSpace(posterPath)
	if posterPath == "" {
		return provider.MatchOK, provider.Art{}, ErrPosterEmpty
	}
	imageURL := p.imageBaseURL + "/" + p.posterSize + posterPath
	art, err := p.downloadArt(ctx, imageURL)
	if err != nil {
		return provider.MatchOK, provider.Art{}, err
	}
	externalID := strconv.Itoa(tmdbID)
	art.ExternalID = &externalID

	return provider.MatchOK, art, nil
}

func (p *Provider) downloadArt(ctx context.Context, imageURL string) (provider.Art, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return provider.Art{}, fmt.Errorf("build tmdb poster request: %w", err)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return provider.Art{}, fmt.Errorf("fetch tmdb poster: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return provider.Art{}, fmt.Errorf("%w: poster status %d", ErrRequestFailed, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPosterBytes))
	if err != nil {
		return provider.Art{}, fmt.Errorf("read tmdb poster: %w", err)
	}
	if len(body) == 0 {
		return provider.Art{}, ErrPosterEmpty
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	return provider.Art{Bytes: body, ContentType: contentType}, nil
}

func (p *Provider) get(ctx context.Context, pathQuery string) ([]byte, error) {
	endpoint := p.authenticatedURL(pathQuery)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build tmdb request: %w", err)
	}
	if p.accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+p.accessToken)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return nil, fmt.Errorf("tmdb http: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	return readTMDBResponse(response)
}

func (p *Provider) authenticatedURL(pathQuery string) string {
	endpoint := p.baseURL + pathQuery
	if p.apiKey == "" {
		return endpoint
	}
	sep := "?"
	if strings.Contains(pathQuery, "?") {
		sep = "&"
	}

	return endpoint + sep + "api_key=" + url.QueryEscape(p.apiKey)
}

func readTMDBResponse(response *http.Response) ([]byte, error) {
	switch response.StatusCode {
	case http.StatusOK:
		data, readErr := provider.ReadLimited(response.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read tmdb body: %w", readErr)
		}

		return data, nil
	case http.StatusNotFound:
		_, _ = io.Copy(io.Discard, response.Body)

		return nil, errNotFound
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		_, _ = io.Copy(io.Discard, response.Body)

		return nil, fmt.Errorf(
			"%w: %w: status %d",
			provider.ErrProviderUnavailable,
			ErrRequestFailed,
			response.StatusCode,
		)
	default:
		_, _ = io.Copy(io.Discard, response.Body)

		return nil, fmt.Errorf("%w: status %d", ErrRequestFailed, response.StatusCode)
	}
}

type searchDTO struct {
	Results []searchResultDTO `json:"results"`
}

type searchResultDTO struct {
	ID           int    `json:"id"`
	FirstAirDate string `json:"first_air_date"` //nolint:tagliatelle // TMDB API wire format
	ReleaseDate  string `json:"release_date"`   //nolint:tagliatelle // TMDB API wire format
}

type findDTO struct {
	MovieResults []searchResultDTO `json:"movie_results"` //nolint:tagliatelle // TMDB API wire format
	TVResults    []searchResultDTO `json:"tv_results"`    //nolint:tagliatelle // TMDB API wire format
}

type externalIDsDTO struct {
	IMDbID string `json:"imdb_id"` //nolint:tagliatelle // TMDB API wire format
	TVDBID int    `json:"tvdb_id"` //nolint:tagliatelle // TMDB API wire format
}

type genreDTO struct {
	Name string `json:"name"`
}

type seasonSummaryDTO struct {
	SeasonNumber int `json:"season_number"` //nolint:tagliatelle // TMDB API wire format
}

type tvDetailDTO struct {
	ID           int        `json:"id"`
	Name         string     `json:"name"`
	Overview     string     `json:"overview"`
	FirstAirDate string     `json:"first_air_date"` //nolint:tagliatelle // TMDB API wire format
	PosterPath   string     `json:"poster_path"`    //nolint:tagliatelle // TMDB API wire format
	Genres       []genreDTO `json:"genres"`
	Networks     []struct {
		Name string `json:"name"`
	} `json:"networks"`
	Seasons     []seasonSummaryDTO `json:"seasons"`
	ExternalIDs externalIDsDTO     `json:"external_ids"` //nolint:tagliatelle // TMDB API wire format
}

type movieDetailDTO struct {
	ID            int        `json:"id"`
	Title         string     `json:"title"`
	Overview      string     `json:"overview"`
	ReleaseDate   string     `json:"release_date"` //nolint:tagliatelle // TMDB API wire format
	PosterPath    string     `json:"poster_path"`  //nolint:tagliatelle // TMDB API wire format
	Genres        []genreDTO `json:"genres"`
	ProductionCos []struct {
		Name string `json:"name"`
	} `json:"production_companies"` //nolint:tagliatelle // TMDB API wire format
	ExternalIDs externalIDsDTO `json:"external_ids"` //nolint:tagliatelle // TMDB API wire format
}

type seasonDetailDTO struct {
	Episodes []struct {
		EpisodeNumber int    `json:"episode_number"` //nolint:tagliatelle // TMDB API wire format
		Name          string `json:"name"`
	} `json:"episodes"`
}

func (s tvDetailDTO) toShowFields() provider.ShowFields {
	title := strings.TrimSpace(s.Name)
	description := strings.TrimSpace(s.Overview)
	tmdbID := strconv.Itoa(s.ID)
	ids := provider.ExternalIDs{TmdbID: &tmdbID}
	if imdb := strings.TrimSpace(s.ExternalIDs.IMDbID); imdb != "" {
		ids.ImdbID = &imdb
	}
	if s.ExternalIDs.TVDBID > 0 {
		tvdb := strconv.Itoa(s.ExternalIDs.TVDBID)
		ids.TvdbID = &tvdb
	}
	fields := provider.ShowFields{
		ExternalID: &tmdbID,
		IDs:        ids,
	}
	if title != "" {
		fields.Title = &title
	}
	if description != "" {
		fields.Description = &description
	}
	if year := yearFromDate(s.FirstAirDate); year != nil {
		fields.Year = year
	}
	if len(s.Networks) > 0 {
		studio := strings.TrimSpace(s.Networks[0].Name)
		if studio != "" {
			fields.Studio = &studio
		}
	}
	fields.Genres = genreNames(s.Genres)

	return fields
}

func (m movieDetailDTO) toFilmFields() provider.FilmFields {
	title := strings.TrimSpace(m.Title)
	description := strings.TrimSpace(m.Overview)
	tmdbID := strconv.Itoa(m.ID)
	ids := provider.ExternalIDs{TmdbID: &tmdbID}
	if imdb := strings.TrimSpace(m.ExternalIDs.IMDbID); imdb != "" {
		ids.ImdbID = &imdb
	}
	fields := provider.FilmFields{
		ExternalID: &tmdbID,
		IDs:        ids,
	}
	if title != "" {
		fields.Title = &title
	}
	if description != "" {
		fields.Description = &description
	}
	if year := yearFromDate(m.ReleaseDate); year != nil {
		fields.Year = year
	}
	if len(m.ProductionCos) > 0 {
		studio := strings.TrimSpace(m.ProductionCos[0].Name)
		if studio != "" {
			fields.Studio = &studio
		}
	}
	fields.Genres = genreNames(m.Genres)

	return fields
}

func genreNames(genres []genreDTO) []string {
	out := make([]string, 0, len(genres))
	for _, genre := range genres {
		name := strings.TrimSpace(genre.Name)
		if name != "" {
			out = append(out, name)
		}
	}

	return out
}
