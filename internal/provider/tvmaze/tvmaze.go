// Package tvmaze implements the TVmaze metadata and poster provider (FI-1 E4).
//
// Public API, no credentials. Admin may assign this adapter to any film/series library;
// titles are resolved via TVmaze show search/lookup (no library-type gate in the adapter).
package tvmaze

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sudoStream/internal/provider"
	"time"
)

// Key is the registry key for this adapter.
const Key = "tvmaze"

const (
	defaultBaseURL     = "https://api.tvmaze.com"
	defaultMinInterval = 500 * time.Millisecond // stay under ~20 req / 10s
	maxPosterBytes     = 8 << 20
)

var (
	// ErrRequestFailed is returned when TVmaze answers with an unexpected status.
	ErrRequestFailed = errors.New("tvmaze request failed")
	// ErrPosterEmpty is returned when a matched show has no image.
	ErrPosterEmpty = errors.New("tvmaze poster empty")
	// ErrMissingShowID is returned when FetchEpisodes is called without a TVmaze id.
	ErrMissingShowID = errors.New("tvmaze show id missing")

	htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)
)

// Provider queries api.tvmaze.com.
type Provider struct {
	baseURL string
	http    *provider.ThrottledClient
}

// Option customizes a Provider.
type Option func(*Provider)

// WithBaseURL overrides the API base URL (tests).
func WithBaseURL(base string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimRight(base, "/") }
}

// WithHTTPClient overrides the underlying HTTP client (tests).
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.http = p.http.WithHTTPClient(client)
	}
}

// WithMinInterval overrides client-side spacing (tests use 0).
func WithMinInterval(interval time.Duration) Option {
	return func(p *Provider) {
		p.http = provider.NewThrottledClient(interval).WithProvider("tvmaze")
	}
}

// New constructs the TVmaze adapter.
func New(opts ...Option) *Provider {
	instance := &Provider{
		baseURL: defaultBaseURL,
		http:    provider.NewThrottledClient(defaultMinInterval).WithProvider("tvmaze"),
	}
	for _, opt := range opts {
		opt(instance)
	}

	return instance
}

// Key returns the registry key.
func (p *Provider) Key() string { return Key }

// MatchShow matches a series show once per show.
func (p *Provider) MatchShow(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.ShowFields, error) {
	show, status, err := p.resolveShow(ctx, hint)
	if err != nil || status != provider.MatchOK {
		return status, provider.ShowFields{}, err
	}

	return provider.MatchOK, show.toFields(), nil
}

// MatchFilm resolves a film title the same way as a show (TVmaze catalog is show-oriented).
func (p *Provider) MatchFilm(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.FilmFields, error) {
	show, status, err := p.resolveShow(ctx, provider.ShowHint{
		Show: hint.Title,
		Year: hint.Year,
	})
	if err != nil || status != provider.MatchOK {
		return status, provider.FilmFields{}, err
	}

	return provider.MatchOK, show.toFilmFields(), nil
}

// FetchShowPoster downloads the show image.
func (p *Provider) FetchShowPoster(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.Art, error) {
	return p.fetchPoster(ctx, provider.ShowHint{
		Show: hint.Show,
		Year: hint.Year,
		IDs:  hint.IDs,
	})
}

// FetchFilmPoster downloads cover art using the same TVmaze resolve path as shows.
func (p *Provider) FetchFilmPoster(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.Art, error) {
	return p.fetchPoster(ctx, provider.ShowHint{
		Show: hint.Title,
		Year: hint.Year,
	})
}

// FetchEpisodes loads /shows/{id}/episodes once per show.
func (p *Provider) FetchEpisodes(
	ctx context.Context,
	fields provider.ShowFields,
) (map[provider.EpisodeKey]provider.EpisodeInfo, error) {
	showID := tvmazeIDFromFields(fields)
	if showID == "" {
		return nil, ErrMissingShowID
	}

	body, err := p.get(ctx, "/shows/"+url.PathEscape(showID)+"/episodes")
	if err != nil {
		return nil, err
	}

	var rows []episodeDTO
	err = json.Unmarshal(body, &rows)
	if err != nil {
		return nil, fmt.Errorf("decode tvmaze episodes: %w", err)
	}

	return mapEpisodeRows(rows), nil
}

func (p *Provider) fetchPoster(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.Art, error) {
	show, status, err := p.resolveShow(ctx, hint)
	if err != nil || status != provider.MatchOK {
		return status, provider.Art{}, err
	}
	imageURL := show.imageURL()
	if imageURL == "" {
		return provider.MatchOK, provider.Art{}, ErrPosterEmpty
	}

	art, err := p.downloadArt(ctx, imageURL)
	if err != nil {
		return provider.MatchOK, provider.Art{}, err
	}
	externalID := strconv.Itoa(show.ID)
	art.ExternalID = &externalID

	return provider.MatchOK, art, nil
}

func tvmazeIDFromFields(fields provider.ShowFields) string {
	if fields.IDs.TvmazeID != nil {
		return strings.TrimSpace(*fields.IDs.TvmazeID)
	}
	if fields.ExternalID != nil {
		return strings.TrimSpace(*fields.ExternalID)
	}

	return ""
}

func mapEpisodeRows(rows []episodeDTO) map[provider.EpisodeKey]provider.EpisodeInfo {
	out := make(map[provider.EpisodeKey]provider.EpisodeInfo, len(rows))
	for _, row := range rows {
		if row.Season <= 0 || row.Number == nil || *row.Number <= 0 {
			continue
		}
		title := strings.TrimSpace(row.Name)
		if title == "" {
			continue
		}
		out[provider.EpisodeKey{Season: row.Season, Episode: *row.Number}] = provider.EpisodeInfo{
			Title: title,
		}
	}

	return out
}

func (p *Provider) resolveShow(
	ctx context.Context,
	hint provider.ShowHint,
) (showDTO, provider.MatchStatus, error) {
	show, status, err := p.resolveByStoredIDs(ctx, hint.IDs)
	if status != provider.MatchNone || err != nil {
		return show, status, err
	}

	query := strings.TrimSpace(hint.Show)
	if query == "" {
		return showDTO{}, provider.MatchNone, nil
	}

	return p.searchShow(ctx, query, hint.Year)
}

func (p *Provider) resolveByStoredIDs(
	ctx context.Context,
	ids provider.ExternalIDs,
) (showDTO, provider.MatchStatus, error) {
	if ids.TvmazeID != nil && strings.TrimSpace(*ids.TvmazeID) != "" {
		return p.getShow(ctx, strings.TrimSpace(*ids.TvmazeID))
	}
	if ids.TvdbID != nil && strings.TrimSpace(*ids.TvdbID) != "" {
		return p.lookupStatus(ctx, "thetvdb", strings.TrimSpace(*ids.TvdbID))
	}
	if ids.ImdbID != nil && strings.TrimSpace(*ids.ImdbID) != "" {
		return p.lookupStatus(ctx, "imdb", strings.TrimSpace(*ids.ImdbID))
	}

	return showDTO{}, provider.MatchNone, nil
}

func (p *Provider) lookupStatus(
	ctx context.Context,
	key, value string,
) (showDTO, provider.MatchStatus, error) {
	show, err := p.lookup(ctx, key, value)
	if err == nil {
		return show, provider.MatchOK, nil
	}
	if errors.Is(err, errNotFound) {
		return showDTO{}, provider.MatchNone, nil
	}

	return showDTO{}, provider.MatchNone, err
}

var errNotFound = errors.New("tvmaze not found")

func (p *Provider) getShow(ctx context.Context, id string) (showDTO, provider.MatchStatus, error) {
	body, err := p.get(ctx, "/shows/"+url.PathEscape(id))
	if err != nil {
		if errors.Is(err, errNotFound) {
			return showDTO{}, provider.MatchNone, nil
		}

		return showDTO{}, provider.MatchNone, err
	}
	var show showDTO
	err = json.Unmarshal(body, &show)
	if err != nil {
		return showDTO{}, provider.MatchNone, fmt.Errorf("decode tvmaze show: %w", err)
	}
	if show.ID == 0 {
		return showDTO{}, provider.MatchNone, nil
	}

	return show, provider.MatchOK, nil
}

func (p *Provider) lookup(ctx context.Context, key, value string) (showDTO, error) {
	body, err := p.get(ctx, "/lookup/shows?"+url.Values{key: {value}}.Encode())
	if err != nil {
		return showDTO{}, err
	}
	var show showDTO
	err = json.Unmarshal(body, &show)
	if err != nil {
		return showDTO{}, fmt.Errorf("decode tvmaze lookup: %w", err)
	}
	if show.ID == 0 {
		return showDTO{}, errNotFound
	}

	return show, nil
}

func (p *Provider) searchShow(
	ctx context.Context,
	query string,
	year *int,
) (showDTO, provider.MatchStatus, error) {
	body, err := p.get(ctx, "/search/shows?"+url.Values{"q": {query}}.Encode())
	if err != nil {
		return showDTO{}, provider.MatchNone, err
	}
	var hits []searchHit
	err = json.Unmarshal(body, &hits)
	if err != nil {
		return showDTO{}, provider.MatchNone, fmt.Errorf("decode tvmaze search: %w", err)
	}

	return pickSearchHit(hits, year)
}

func pickSearchHit(hits []searchHit, year *int) (showDTO, provider.MatchStatus, error) {
	if len(hits) == 0 {
		return showDTO{}, provider.MatchNone, nil
	}

	candidates := filterSearchHits(hits, year)
	switch len(candidates) {
	case 0:
		if len(hits) == 1 && hits[0].Show.ID != 0 {
			return hits[0].Show, provider.MatchUncertain, nil
		}

		return showDTO{}, provider.MatchNone, nil
	case 1:
		return candidates[0], provider.MatchOK, nil
	default:
		return showDTO{}, provider.MatchUncertain, nil
	}
}

func filterSearchHits(hits []searchHit, year *int) []showDTO {
	candidates := make([]showDTO, 0, len(hits))
	for _, hit := range hits {
		if hit.Show.ID == 0 {
			continue
		}
		if year != nil {
			premiered := hit.Show.premieredYear()
			if premiered != nil && *premiered != *year {
				continue
			}
		}
		candidates = append(candidates, hit.Show)
	}

	return candidates
}

func (p *Provider) downloadArt(ctx context.Context, imageURL string) (provider.Art, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return provider.Art{}, fmt.Errorf("build tvmaze poster request: %w", err)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return provider.Art{}, fmt.Errorf("fetch tvmaze poster: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return provider.Art{}, fmt.Errorf("%w: poster status %d", ErrRequestFailed, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPosterBytes))
	if err != nil {
		return provider.Art{}, fmt.Errorf("read tvmaze poster: %w", err)
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+pathQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("build tvmaze request: %w", err)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return nil, fmt.Errorf("tvmaze http: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
		data, readErr := provider.ReadLimited(response.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read tvmaze body: %w", readErr)
		}

		return data, nil
	case http.StatusNotFound:
		_, _ = io.Copy(io.Discard, response.Body)

		return nil, errNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
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

type searchHit struct {
	Show showDTO `json:"show"`
}

type showDTO struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Summary   string   `json:"summary"`
	Genres    []string `json:"genres"`
	Premiered string   `json:"premiered"`
	Network   *struct {
		Name string `json:"name"`
	} `json:"network"`
	WebChannel *struct {
		Name string `json:"name"`
	} `json:"webChannel"`
	Externals struct {
		TVDB *int    `json:"thetvdb"`
		IMDb *string `json:"imdb"`
	} `json:"externals"`
	Image *struct {
		Medium   string `json:"medium"`
		Original string `json:"original"`
	} `json:"image"`
}

type episodeDTO struct {
	Season int    `json:"season"`
	Number *int   `json:"number"`
	Name   string `json:"name"`
}

func (s showDTO) premieredYear() *int {
	const yearPrefixLen = 4
	if len(s.Premiered) < yearPrefixLen {
		return nil
	}
	year, err := strconv.Atoi(s.Premiered[:yearPrefixLen])
	if err != nil {
		return nil
	}

	return &year
}

func (s showDTO) imageURL() string {
	if s.Image == nil {
		return ""
	}
	if s.Image.Original != "" {
		return s.Image.Original
	}

	return s.Image.Medium
}

func (s showDTO) toFields() provider.ShowFields {
	title := strings.TrimSpace(s.Name)
	description := stripHTML(s.Summary)
	studio := s.studioName()
	year := s.premieredYear()
	tvmazeID := strconv.Itoa(s.ID)
	ids := provider.ExternalIDs{TvmazeID: &tvmazeID}
	if s.Externals.TVDB != nil {
		tvdb := strconv.Itoa(*s.Externals.TVDB)
		ids.TvdbID = &tvdb
	}
	if s.Externals.IMDb != nil && strings.TrimSpace(*s.Externals.IMDb) != "" {
		imdb := strings.TrimSpace(*s.Externals.IMDb)
		ids.ImdbID = &imdb
	}

	fields := provider.ShowFields{
		ExternalID: &tvmazeID,
		IDs:        ids,
	}
	if title != "" {
		fields.Title = &title
	}
	if description != "" {
		fields.Description = &description
	}
	if studio != "" {
		fields.Studio = &studio
	}
	if year != nil {
		fields.Year = year
	}
	if len(s.Genres) > 0 {
		fields.Genres = append([]string{}, s.Genres...)
	}

	return fields
}

func (s showDTO) toFilmFields() provider.FilmFields {
	show := s.toFields()

	return provider.FilmFields(show)
}

func (s showDTO) studioName() string {
	if s.Network != nil && strings.TrimSpace(s.Network.Name) != "" {
		return strings.TrimSpace(s.Network.Name)
	}
	if s.WebChannel != nil {
		return strings.TrimSpace(s.WebChannel.Name)
	}

	return ""
}

func stripHTML(raw string) string {
	text := htmlTagPattern.ReplaceAllString(raw, " ")
	text = html.UnescapeString(text)
	text = strings.Join(strings.Fields(text), " ")

	return strings.TrimSpace(text)
}
