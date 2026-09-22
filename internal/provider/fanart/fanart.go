// Package fanart implements the Fanart.tv poster-only provider (FI-1 E7).
//
// No search: film posters require a stored TMDB id; series posters require a TVDB id
// (typically written by a prior metadata run such as TMDB/TVDB/TVmaze).
package fanart

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
const Key = "fanart"

const (
	defaultBaseURL     = "https://webservice.fanart.tv/v3"
	defaultMinInterval = 250 * time.Millisecond
	maxPosterBytes     = 8 << 20
)

// Env keys (FI-1 L13).
const (
	//nolint:gosec // G101: env var name
	EnvAPIKey = "SUDOSTREAM_FANART_API_KEY"

	EnvClientKey = "SUDOSTREAM_FANART_CLIENT_KEY"
)

var (
	// ErrMissingCredentials is returned when the API key is unset.
	ErrMissingCredentials = errors.New("fanart credentials missing")
	// ErrRequestFailed is returned on unexpected HTTP statuses.
	ErrRequestFailed = errors.New("fanart request failed")
	// ErrPosterEmpty is returned when a response has no usable poster URL.
	ErrPosterEmpty = errors.New("fanart poster empty")

	errNotFound = errors.New("fanart not found")
)

// Provider queries webservice.fanart.tv for poster art.
type Provider struct {
	baseURL   string
	apiKey    string
	clientKey string
	http      *provider.ThrottledClient
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
		p.http = provider.NewThrottledClient(interval).WithProvider("fanart")
	}
}

// WithCredentials sets API key and optional personal client-key (tests).
func WithCredentials(apiKey, clientKey string) Option {
	return func(p *Provider) {
		p.apiKey = strings.TrimSpace(apiKey)
		p.clientKey = strings.TrimSpace(clientKey)
	}
}

// New constructs the Fanart.tv adapter.
func New(opts ...Option) *Provider {
	instance := &Provider{
		baseURL:   defaultBaseURL,
		apiKey:    provider.Env(EnvAPIKey),
		clientKey: provider.Env(EnvClientKey),
		http:      provider.NewThrottledClient(defaultMinInterval).WithProvider("fanart"),
	}
	for _, opt := range opts {
		opt(instance)
	}

	return instance
}

// Key returns the registry key.
func (p *Provider) Key() string { return Key }

// RequiredEnv lists the project API key env var (FI-1 L13).
func (p *Provider) RequiredEnv() []string {
	if strings.TrimSpace(p.apiKey) != "" || provider.Env(EnvAPIKey) != "" {
		return nil
	}

	return []string{EnvAPIKey}
}

// FetchShowPoster downloads series poster art for a TVDB id.
func (p *Provider) FetchShowPoster(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.Art, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.Art{}, err
	}
	tvdbID := derefID(hint.IDs.TvdbID)
	if tvdbID == "" {
		return provider.MatchNone, provider.Art{}, nil
	}

	body, err := p.get(ctx, "/tv/"+url.PathEscape(tvdbID))
	if err != nil {
		if errors.Is(err, errNotFound) {
			return provider.MatchNone, provider.Art{}, nil
		}

		return provider.MatchNone, provider.Art{}, err
	}
	var payload tvArtDTO
	err = json.Unmarshal(body, &payload)
	if err != nil {
		return provider.MatchNone, provider.Art{}, fmt.Errorf("decode fanart tv: %w", err)
	}
	imageURL := pickArtURL(payload.TVPoster, payload.HDTVPoster)
	if imageURL == "" {
		return provider.MatchOK, provider.Art{}, ErrPosterEmpty
	}

	return p.downloadPoster(ctx, imageURL, tvdbID)
}

// FetchFilmPoster downloads movie poster art for a TMDB id.
func (p *Provider) FetchFilmPoster(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.Art, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.Art{}, err
	}
	tmdbID := derefID(hint.IDs.TmdbID)
	if tmdbID == "" {
		return provider.MatchNone, provider.Art{}, nil
	}

	body, err := p.get(ctx, "/movies/"+url.PathEscape(tmdbID))
	if err != nil {
		if errors.Is(err, errNotFound) {
			return provider.MatchNone, provider.Art{}, nil
		}

		return provider.MatchNone, provider.Art{}, err
	}
	var payload movieArtDTO
	err = json.Unmarshal(body, &payload)
	if err != nil {
		return provider.MatchNone, provider.Art{}, fmt.Errorf("decode fanart movie: %w", err)
	}
	imageURL := pickArtURL(payload.MoviePoster)
	if imageURL == "" {
		return provider.MatchOK, provider.Art{}, ErrPosterEmpty
	}

	return p.downloadPoster(ctx, imageURL, tmdbID)
}

func (p *Provider) downloadPoster(
	ctx context.Context,
	imageURL, externalID string,
) (provider.MatchStatus, provider.Art, error) {
	art, err := p.downloadArt(ctx, imageURL)
	if err != nil {
		return provider.MatchOK, provider.Art{}, err
	}
	id := externalID
	art.ExternalID = &id

	return provider.MatchOK, art, nil
}

func (p *Provider) ensureCreds() error {
	if strings.TrimSpace(p.apiKey) == "" {
		p.apiKey = provider.Env(EnvAPIKey)
	}
	if strings.TrimSpace(p.clientKey) == "" {
		p.clientKey = provider.Env(EnvClientKey)
	}
	if p.apiKey == "" {
		return fmt.Errorf("%w: set %s", ErrMissingCredentials, EnvAPIKey)
	}

	return nil
}

func (p *Provider) downloadArt(ctx context.Context, imageURL string) (provider.Art, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return provider.Art{}, fmt.Errorf("build fanart poster request: %w", err)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return provider.Art{}, fmt.Errorf("fetch fanart poster: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusForbidden ||
		response.StatusCode == http.StatusTooManyRequests {
		return provider.Art{}, fmt.Errorf(
			"%w: %w: poster status %d",
			provider.ErrProviderUnavailable,
			ErrRequestFailed,
			response.StatusCode,
		)
	}
	if response.StatusCode != http.StatusOK {
		return provider.Art{}, fmt.Errorf("%w: poster status %d", ErrRequestFailed, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPosterBytes))
	if err != nil {
		return provider.Art{}, fmt.Errorf("read fanart poster: %w", err)
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

func (p *Provider) get(ctx context.Context, path string) ([]byte, error) {
	endpoint := p.baseURL + path + "?api_key=" + url.QueryEscape(p.apiKey)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build fanart request: %w", err)
	}
	if p.clientKey != "" {
		request.Header.Set("Client-Key", p.clientKey)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return nil, fmt.Errorf("fanart http: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
		data, readErr := provider.ReadLimited(response.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read fanart body: %w", readErr)
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

func pickArtURL(lists ...[]artItemDTO) string {
	var bestURL string
	bestLikes := -1
	for _, list := range lists {
		for _, item := range list {
			imageURL := strings.TrimSpace(item.URL)
			if imageURL == "" {
				continue
			}
			likes := parseLikes(item.Likes)
			if bestURL == "" || likes > bestLikes {
				bestURL = imageURL
				bestLikes = likes
			}
		}
		if bestURL != "" {
			return bestURL
		}
	}

	return bestURL
}

func parseLikes(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}

	return n
}

func derefID(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

type artItemDTO struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Likes string `json:"likes"`
}

type movieArtDTO struct {
	MoviePoster []artItemDTO `json:"movieposter"`
}

type tvArtDTO struct {
	TVPoster   []artItemDTO `json:"tvposter"`
	HDTVPoster []artItemDTO `json:"hdtvposter"`
}
