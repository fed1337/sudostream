// Package tvdb implements the TheTVDB v4 metadata and poster provider (FI-1 E6).
package tvdb

import (
	"bytes"
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
	"sync"
	"time"
)

// Key is the registry key for this adapter.
const Key = "tvdb"

const (
	defaultBaseURL     = "https://api4.thetvdb.com/v4"
	defaultMinInterval = 250 * time.Millisecond
	maxPosterBytes     = 8 << 20
	yearPrefixLen      = 4
)

// Env keys (FI-1 L13).
const (
	//nolint:gosec // G101: env var name
	EnvAPIKey = "SUDOSTREAM_TVDB_API_KEY"

	EnvPIN = "SUDOSTREAM_TVDB_PIN"
)

var (
	// ErrMissingCredentials is returned when the API key is unset.
	ErrMissingCredentials = errors.New("tvdb credentials missing")
	// ErrRequestFailed is returned on unexpected HTTP statuses.
	ErrRequestFailed = errors.New("tvdb request failed")
	// ErrPosterEmpty is returned when a match has no artwork URL.
	ErrPosterEmpty = errors.New("tvdb poster empty")
	// ErrMissingShowID is returned when FetchEpisodes lacks a TVDB id.
	ErrMissingShowID = errors.New("tvdb show id missing")
	// ErrEmptyLoginToken is returned when /login omits data.token.
	ErrEmptyLoginToken = errors.New("tvdb empty login token")
	// ErrUnsupportedIDJSON is returned when an id field is neither string nor number.
	ErrUnsupportedIDJSON = errors.New("tvdb id unsupported json")

	errNotFound = errors.New("tvdb not found")
)

// Provider queries api4.thetvdb.com.
type Provider struct {
	baseURL string
	apiKey  string
	pin     string
	http    *provider.ThrottledClient

	mu    sync.Mutex
	token string
}

// Option customizes a Provider.
type Option func(*Provider)

// WithBaseURL overrides the API base URL (tests).
func WithBaseURL(base string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimRight(base, "/") }
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
		p.http = provider.NewThrottledClient(interval).WithProvider("tvdb")
	}
}

// WithCredentials sets API key and optional PIN explicitly (tests).
func WithCredentials(apiKey, pin string) Option {
	return func(p *Provider) {
		p.apiKey = strings.TrimSpace(apiKey)
		p.pin = strings.TrimSpace(pin)
	}
}

// New constructs the TVDB adapter.
func New(opts ...Option) *Provider {
	instance := &Provider{
		baseURL: defaultBaseURL,
		apiKey:  provider.Env(EnvAPIKey),
		pin:     provider.Env(EnvPIN),
		http:    provider.NewThrottledClient(defaultMinInterval).WithProvider("tvdb"),
	}
	for _, opt := range opts {
		opt(instance)
	}

	return instance
}

// Key returns the registry key.
func (p *Provider) Key() string { return Key }

// RequiredEnv lists the required credential env var (API key only; PIN is optional).
func (p *Provider) RequiredEnv() []string {
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
	show, status, err := p.resolveSeries(ctx, hint)
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
	show, status, err := p.resolveSeries(ctx, hint)
	if err != nil || status != provider.MatchOK {
		return status, provider.Art{}, err
	}

	return p.artFromEntity(ctx, show.imageURL(), show.ID)
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

	return p.artFromEntity(ctx, movie.imageURL(), movie.ID)
}

// FetchEpisodes loads the default episode list once per show.
func (p *Provider) FetchEpisodes(
	ctx context.Context,
	fields provider.ShowFields,
) (map[provider.EpisodeKey]provider.EpisodeInfo, error) {
	err := p.ensureCreds()
	if err != nil {
		return nil, err
	}
	showID := tvdbIDFromFields(fields)
	if showID == "" {
		return nil, ErrMissingShowID
	}

	out := make(map[provider.EpisodeKey]provider.EpisodeInfo)
	page := 0
	for {
		more, pageErr := p.fetchEpisodePage(ctx, showID, page, out)
		if pageErr != nil {
			return nil, pageErr
		}
		if !more {
			break
		}
		page++
	}

	return out, nil
}

func (p *Provider) fetchEpisodePage(
	ctx context.Context,
	showID string,
	page int,
	out map[provider.EpisodeKey]provider.EpisodeInfo,
) (bool, error) {
	body, err := p.get(ctx, "/series/"+url.PathEscape(showID)+"/episodes/default?page="+strconv.Itoa(page))
	if err != nil {
		return false, err
	}
	var payload episodesPageDTO
	err = json.Unmarshal(body, &payload)
	if err != nil {
		return false, fmt.Errorf("decode tvdb episodes: %w", err)
	}
	for _, ep := range payload.Data.Episodes {
		title := strings.TrimSpace(ep.Name)
		if ep.SeasonNumber < 0 || ep.Number <= 0 || title == "" {
			continue
		}
		out[provider.EpisodeKey{Season: ep.SeasonNumber, Episode: ep.Number}] = provider.EpisodeInfo{
			Title: title,
		}
	}
	if payload.Links.Next == nil || strings.TrimSpace(*payload.Links.Next) == "" {
		return false, nil
	}

	return true, nil
}

func (p *Provider) ensureCreds() error {
	if strings.TrimSpace(p.apiKey) == "" {
		p.apiKey = provider.Env(EnvAPIKey)
	}
	if strings.TrimSpace(p.pin) == "" {
		p.pin = provider.Env(EnvPIN)
	}
	if p.apiKey == "" {
		return fmt.Errorf("%w: set %s", ErrMissingCredentials, EnvAPIKey)
	}

	return nil
}

func tvdbIDFromFields(fields provider.ShowFields) string {
	if fields.IDs.TvdbID != nil {
		return strings.TrimSpace(*fields.IDs.TvdbID)
	}
	if fields.ExternalID != nil {
		return strings.TrimSpace(*fields.ExternalID)
	}

	return ""
}

func (p *Provider) resolveSeries(
	ctx context.Context,
	hint provider.ShowHint,
) (entityDTO, provider.MatchStatus, error) {
	if hint.IDs.TvdbID != nil && strings.TrimSpace(*hint.IDs.TvdbID) != "" {
		return p.getEntity(ctx, "series", strings.TrimSpace(*hint.IDs.TvdbID))
	}

	query := strings.TrimSpace(hint.Show)
	if query == "" {
		return entityDTO{}, provider.MatchNone, nil
	}

	return p.searchEntity(ctx, query, "series", hint.Year)
}

func (p *Provider) resolveMovie(
	ctx context.Context,
	hint provider.FilmHint,
) (entityDTO, provider.MatchStatus, error) {
	if hint.IDs.TvdbID != nil && strings.TrimSpace(*hint.IDs.TvdbID) != "" {
		return p.getEntity(ctx, "movies", strings.TrimSpace(*hint.IDs.TvdbID))
	}

	query := strings.TrimSpace(hint.Title)
	if query == "" {
		return entityDTO{}, provider.MatchNone, nil
	}

	return p.searchEntity(ctx, query, "movie", hint.Year)
}

func (p *Provider) getEntity(
	ctx context.Context,
	kind, entityID string,
) (entityDTO, provider.MatchStatus, error) {
	body, err := p.get(ctx, "/"+kind+"/"+url.PathEscape(entityID)+"/extended")
	if errors.Is(err, errNotFound) {
		body, err = p.get(ctx, "/"+kind+"/"+url.PathEscape(entityID))
	}
	if err != nil {
		if errors.Is(err, errNotFound) {
			return entityDTO{}, provider.MatchNone, nil
		}

		return entityDTO{}, provider.MatchNone, err
	}
	var wrap entityWrapDTO
	err = json.Unmarshal(body, &wrap)
	if err != nil {
		return entityDTO{}, provider.MatchNone, fmt.Errorf("decode tvdb %s: %w", kind, err)
	}
	if wrap.Data.ID == 0 {
		return entityDTO{}, provider.MatchNone, nil
	}

	return wrap.Data, provider.MatchOK, nil
}

func (p *Provider) searchEntity(
	ctx context.Context,
	query, entityType string,
	year *int,
) (entityDTO, provider.MatchStatus, error) {
	values := url.Values{
		"query": {query},
		"type":  {entityType},
	}
	body, err := p.get(ctx, "/search?"+values.Encode())
	if err != nil {
		return entityDTO{}, provider.MatchNone, err
	}
	var payload searchDTO
	err = json.Unmarshal(body, &payload)
	if err != nil {
		return entityDTO{}, provider.MatchNone, fmt.Errorf("decode tvdb search: %w", err)
	}

	status, entityID := pickSearchHit(payload.Data, year)
	if status != provider.MatchOK || entityID == "" {
		return entityDTO{}, status, nil
	}
	kind := "series"
	if entityType == "movie" {
		kind = "movies"
	}

	return p.getEntity(ctx, kind, entityID)
}

func pickSearchHit(hits []searchHitDTO, year *int) (provider.MatchStatus, string) {
	if len(hits) == 0 {
		return provider.MatchNone, ""
	}
	candidates := filterSearchHits(hits, year)
	switch len(candidates) {
	case 0:
		if len(hits) == 1 {
			if entityID := hits[0].id(); entityID != "" {
				return provider.MatchUncertain, entityID
			}
		}

		return provider.MatchNone, ""
	case 1:
		return provider.MatchOK, candidates[0].id()
	default:
		return provider.MatchUncertain, ""
	}
}

func filterSearchHits(hits []searchHitDTO, year *int) []searchHitDTO {
	candidates := make([]searchHitDTO, 0, len(hits))
	for _, hit := range hits {
		if hit.id() == "" {
			continue
		}
		if year != nil {
			got := hit.yearInt()
			if got != nil && *got != *year {
				continue
			}
		}
		candidates = append(candidates, hit)
	}

	return candidates
}

func (p *Provider) artFromEntity(
	ctx context.Context,
	imageURL string,
	entityID int,
) (provider.MatchStatus, provider.Art, error) {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return provider.MatchOK, provider.Art{}, ErrPosterEmpty
	}
	art, err := p.downloadArt(ctx, imageURL)
	if err != nil {
		return provider.MatchOK, provider.Art{}, err
	}
	externalID := strconv.Itoa(entityID)
	art.ExternalID = &externalID

	return provider.MatchOK, art, nil
}

func (p *Provider) downloadArt(ctx context.Context, imageURL string) (provider.Art, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return provider.Art{}, fmt.Errorf("build tvdb poster request: %w", err)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return provider.Art{}, fmt.Errorf("fetch tvdb poster: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return provider.Art{}, fmt.Errorf("%w: poster status %d", ErrRequestFailed, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPosterBytes))
	if err != nil {
		return provider.Art{}, fmt.Errorf("read tvdb poster: %w", err)
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

func (p *Provider) ensureLogin(ctx context.Context) error {
	p.mu.Lock()
	token := p.token
	p.mu.Unlock()
	if token != "" {
		return nil
	}

	return p.login(ctx)
}

func (p *Provider) login(ctx context.Context) error {
	body := map[string]string{"apikey": p.apiKey}
	if p.pin != "" {
		body["pin"] = p.pin
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode tvdb login: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/login", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build tvdb login: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return fmt.Errorf("tvdb login http: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	return p.handleLoginResponse(response)
}

func (p *Provider) handleLoginResponse(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusOK:
		data, readErr := provider.ReadLimited(response.Body)
		if readErr != nil {
			return fmt.Errorf("read tvdb login: %w", readErr)
		}
		var parsed loginDTO
		err := json.Unmarshal(data, &parsed)
		if err != nil {
			return fmt.Errorf("decode tvdb login: %w", err)
		}
		token := strings.TrimSpace(parsed.Data.Token)
		if token == "" {
			return ErrEmptyLoginToken
		}
		p.mu.Lock()
		p.token = token
		p.mu.Unlock()

		return nil
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		_, _ = io.Copy(io.Discard, response.Body)

		return fmt.Errorf(
			"%w: %w: login status %d",
			provider.ErrProviderUnavailable,
			ErrRequestFailed,
			response.StatusCode,
		)
	default:
		_, _ = io.Copy(io.Discard, response.Body)

		return fmt.Errorf("%w: login status %d", ErrRequestFailed, response.StatusCode)
	}
}

func (p *Provider) get(ctx context.Context, pathQuery string) ([]byte, error) {
	err := p.ensureLogin(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	token := p.token
	p.mu.Unlock()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+pathQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("build tvdb request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return nil, fmt.Errorf("tvdb http: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	return readAPIResponse(response, func() {
		p.mu.Lock()
		p.token = ""
		p.mu.Unlock()
	})
}

func readAPIResponse(response *http.Response, clearToken func()) ([]byte, error) {
	switch response.StatusCode {
	case http.StatusOK:
		data, readErr := provider.ReadLimited(response.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read tvdb body: %w", readErr)
		}

		return data, nil
	case http.StatusNotFound:
		_, _ = io.Copy(io.Discard, response.Body)

		return nil, errNotFound
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		_, _ = io.Copy(io.Discard, response.Body)
		if response.StatusCode == http.StatusUnauthorized {
			clearToken()
		}

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

type loginDTO struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

type searchDTO struct {
	Data []searchHitDTO `json:"data"`
}

type searchHitDTO struct {
	TVDBID   flexID `json:"tvdb_id"` //nolint:tagliatelle // TVDB API wire format
	ObjectID flexID `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Year     string `json:"year"`
	ImageURL string `json:"image_url"` //nolint:tagliatelle // TVDB API wire format
}

func (h searchHitDTO) id() string {
	if entityID := strings.TrimSpace(string(h.TVDBID)); entityID != "" {
		return entityID
	}

	return strings.TrimSpace(string(h.ObjectID))
}

func (h searchHitDTO) yearInt() *int {
	return parseYear(h.Year)
}

type entityWrapDTO struct {
	Data entityDTO `json:"data"`
}

type entityDTO struct {
	ID          int           `json:"id"`
	Name        string        `json:"name"`
	Overview    string        `json:"overview"`
	Year        string        `json:"year"`
	Image       string        `json:"image"`
	RemoteIDs   []remoteIDDTO `json:"remoteIds"`
	Artworks    []artworkDTO  `json:"artworks"`
	Genres      []genreDTO    `json:"genres"`
	Companies   []companyDTO  `json:"companies"`
	OriginalNet *networkDTO   `json:"originalNetwork"`
}

type remoteIDDTO struct {
	ID         string `json:"id"`
	Type       int    `json:"type"`
	SourceName string `json:"sourceName"`
}

type artworkDTO struct {
	Image string `json:"image"`
	Type  int    `json:"type"`
}

type genreDTO struct {
	Name string `json:"name"`
}

type companyDTO struct {
	Name string `json:"name"`
}

type networkDTO struct {
	Name string `json:"name"`
}

type episodesPageDTO struct {
	Data struct {
		Episodes []episodeDTO `json:"episodes"`
	} `json:"data"`
	Links struct {
		Next *string `json:"next"`
	} `json:"links"`
}

type episodeDTO struct {
	SeasonNumber int    `json:"seasonNumber"`
	Number       int    `json:"number"`
	Name         string `json:"name"`
}

// flexID unmarshals JSON string or number identifiers.
type flexID string

func (f *flexID) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*f = ""

		return nil
	}
	var asString string
	err := json.Unmarshal(data, &asString)
	if err == nil {
		*f = flexID(asString)

		return nil
	}
	var asNumber json.Number
	err = json.Unmarshal(data, &asNumber)
	if err == nil {
		*f = flexID(asNumber.String())

		return nil
	}

	return fmt.Errorf("%w: %s", ErrUnsupportedIDJSON, string(data))
}

func (e entityDTO) imageURL() string {
	if img := strings.TrimSpace(e.Image); img != "" {
		return img
	}
	for _, art := range e.Artworks {
		if img := strings.TrimSpace(art.Image); img != "" {
			return img
		}
	}

	return ""
}

func (e entityDTO) toShowFields() provider.ShowFields {
	tvdbID := strconv.Itoa(e.ID)
	ids := provider.ExternalIDs{TvdbID: &tvdbID}
	applyRemoteIDs(&ids, e.RemoteIDs)

	fields := provider.ShowFields{
		ExternalID: &tvdbID,
		IDs:        ids,
	}
	if title := strings.TrimSpace(e.Name); title != "" {
		fields.Title = &title
	}
	if description := strings.TrimSpace(e.Overview); description != "" {
		fields.Description = &description
	}
	if year := parseYear(e.Year); year != nil {
		fields.Year = year
	}
	if studio := e.studioName(); studio != "" {
		fields.Studio = &studio
	}
	fields.Genres = genreNames(e.Genres)

	return fields
}

func (e entityDTO) toFilmFields() provider.FilmFields {
	show := e.toShowFields()

	return provider.FilmFields(show)
}

func (e entityDTO) studioName() string {
	if e.OriginalNet != nil {
		if name := strings.TrimSpace(e.OriginalNet.Name); name != "" {
			return name
		}
	}
	for _, company := range e.Companies {
		if name := strings.TrimSpace(company.Name); name != "" {
			return name
		}
	}

	return ""
}

func applyRemoteIDs(ids *provider.ExternalIDs, remotes []remoteIDDTO) {
	for _, remote := range remotes {
		value := strings.TrimSpace(remote.ID)
		if value == "" {
			continue
		}
		source := strings.ToLower(strings.TrimSpace(remote.SourceName))
		switch {
		case source == "imdb" || strings.Contains(source, "imdb"):
			if ids.ImdbID == nil {
				ids.ImdbID = &value
			}
		case source == "tmdb" || source == "themoviedb" || strings.Contains(source, "themoviedb"):
			if ids.TmdbID == nil {
				ids.TmdbID = &value
			}
		}
	}
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

func parseYear(raw string) *int {
	raw = strings.TrimSpace(raw)
	if len(raw) < yearPrefixLen {
		return nil
	}
	year, err := strconv.Atoi(raw[:yearPrefixLen])
	if err != nil {
		return nil
	}

	return &year
}
