// Package anilist implements the AniList metadata and poster provider (FI-1 L19).
//
// AniList is a public GraphQL API and needs no credentials. Its catalog is anime/manga only:
// a film or series library of Western live-action content will legitimately match nothing,
// which the enricher logs and skips rather than writing bad data.
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sudoStream/internal/provider"
	"sync"
	"time"
)

// Key is the registry key for this adapter.
const Key = "anilist"

const (
	defaultEndpoint = "https://graphql.anilist.co"
	// defaultMinInterval keeps requests under AniList's degraded 30 requests/minute limit.
	defaultMinInterval = 2 * time.Second
	defaultTimeout     = 20 * time.Second
	maxRetryAfter      = 90 * time.Second
	maxResponseBytes   = 2 << 20
	maxPosterBytes     = 8 << 20
	searchCandidates   = 5
)

var (
	// ErrRequestFailed is returned when AniList answers with a non-200 status.
	ErrRequestFailed = errors.New("anilist request failed")
	// ErrGraphQLError is returned when AniList answers 200 with a GraphQL error payload.
	ErrGraphQLError = errors.New("anilist graphql error")
	// ErrRateLimited is returned when AniList throttles the client past its retry budget.
	ErrRateLimited = errors.New("anilist rate limited")
	// ErrPosterEmpty is returned when a matched title has no usable cover image.
	ErrPosterEmpty = errors.New("anilist poster empty")
)

// Provider queries AniList for show/film metadata and cover art.
type Provider struct {
	endpoint    string
	client      *http.Client
	minInterval time.Duration

	mu       sync.Mutex
	lastCall time.Time
}

// Option customizes a Provider (tests point it at a local server with no throttle).
type Option func(*Provider)

// WithEndpoint overrides the GraphQL endpoint.
func WithEndpoint(endpoint string) Option {
	return func(p *Provider) { p.endpoint = endpoint }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) { p.client = client }
}

// WithMinInterval overrides the client-side throttle between API calls.
func WithMinInterval(interval time.Duration) Option {
	return func(p *Provider) { p.minInterval = interval }
}

// New constructs the AniList adapter.
func New(opts ...Option) *Provider {
	instance := &Provider{
		endpoint:    defaultEndpoint,
		client:      &http.Client{Timeout: defaultTimeout},
		minInterval: defaultMinInterval,
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
	status, media, err := p.match(ctx, hint.Show, hint.Year)
	if err != nil || status != provider.MatchOK {
		return status, provider.ShowFields{}, err
	}

	fields := media.fields()

	return provider.MatchOK, provider.ShowFields{
		Title:       fields.title,
		Description: fields.description,
		Genres:      fields.genres,
		Studio:      fields.studio,
		Year:        fields.year,
		ExternalID:  fields.externalID,
		IDs:         provider.ExternalIDs{AnilistID: fields.externalID},
	}, nil
}

// MatchFilm matches a film once per file.
func (p *Provider) MatchFilm(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.FilmFields, error) {
	status, media, err := p.match(ctx, hint.Title, hint.Year)
	if err != nil || status != provider.MatchOK {
		return status, provider.FilmFields{}, err
	}

	fields := media.fields()

	return provider.MatchOK, provider.FilmFields{
		Title:       fields.title,
		Description: fields.description,
		Genres:      fields.genres,
		Studio:      fields.studio,
		Year:        fields.year,
		ExternalID:  fields.externalID,
		IDs:         provider.ExternalIDs{AnilistID: fields.externalID},
	}, nil
}

// FetchShowPoster fetches cover art for a series show.
func (p *Provider) FetchShowPoster(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.Art, error) {
	return p.fetchPoster(ctx, hint.Show, hint.Year)
}

// FetchFilmPoster fetches cover art for a film file.
func (p *Provider) FetchFilmPoster(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.Art, error) {
	return p.fetchPoster(ctx, hint.Title, hint.Year)
}

func (p *Provider) fetchPoster(
	ctx context.Context,
	title string,
	year *int,
) (provider.MatchStatus, provider.Art, error) {
	status, media, err := p.match(ctx, title, year)
	if err != nil || status != provider.MatchOK {
		return status, provider.Art{}, err
	}

	imageURL := media.CoverImage.best()
	if imageURL == "" {
		return provider.MatchOK, provider.Art{}, ErrPosterEmpty
	}

	art, err := p.downloadArt(ctx, imageURL)
	if err != nil {
		return provider.MatchOK, provider.Art{}, err
	}

	externalID := strconv.Itoa(media.ID)
	art.ExternalID = &externalID

	return provider.MatchOK, art, nil
}

func (p *Provider) downloadArt(ctx context.Context, imageURL string) (provider.Art, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return provider.Art{}, fmt.Errorf("build poster request: %w", err)
	}
	request.Header.Set("User-Agent", provider.DefaultHTTPUserAgent)
	request.Header.Set("Accept", "image/*,*/*")

	response, err := p.client.Do(request)
	if err != nil {
		return provider.Art{}, fmt.Errorf("fetch poster: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode == http.StatusForbidden {
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
		return provider.Art{}, fmt.Errorf("read poster: %w", err)
	}
	if len(body) == 0 {
		return provider.Art{}, ErrPosterEmpty
	}

	return provider.Art{
		Bytes:       body,
		ContentType: response.Header.Get("Content-Type"),
	}, nil
}

// post sends one throttled GraphQL request, retrying once when AniList reports a 429.
func (p *Provider) post(ctx context.Context, body []byte) ([]byte, error) {
	for attempt := range 2 {
		p.throttle(ctx)

		payload, retryAfter, err := p.postOnce(ctx, body)
		if err == nil {
			return payload, nil
		}
		if retryAfter <= 0 || attempt == 1 {
			if errors.Is(err, ErrRateLimited) {
				return nil, fmt.Errorf("%w: %w", provider.ErrProviderUnavailable, err)
			}

			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("anilist wait canceled: %w", ctx.Err())
		case <-time.After(retryAfter):
		}
	}

	return nil, fmt.Errorf("%w: %w", provider.ErrProviderUnavailable, ErrRateLimited)
}

func (p *Provider) postOnce(
	ctx context.Context,
	body []byte,
) ([]byte, time.Duration, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("build anilist request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", provider.DefaultHTTPUserAgent)

	response, err := p.client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("call anilist: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("read anilist response: %w", err)
	}

	switch response.StatusCode {
	case http.StatusOK:
		return payload, 0, nil
	case http.StatusTooManyRequests:
		return nil, retryAfterDuration(response.Header.Get("Retry-After")), fmt.Errorf(
			"%w: status %d",
			ErrRateLimited,
			response.StatusCode,
		)
	case http.StatusForbidden:
		return nil, 0, fmt.Errorf(
			"%w: %w: status %d",
			provider.ErrProviderUnavailable,
			ErrRequestFailed,
			response.StatusCode,
		)
	default:
		return nil, 0, fmt.Errorf("%w: status %d", ErrRequestFailed, response.StatusCode)
	}
}

// throttle spaces API calls out client-side; AniList's limit is per minute and shared by the
// whole process, so one mutex-guarded timestamp is enough.
func (p *Provider) throttle(ctx context.Context) {
	if p.minInterval <= 0 {
		return
	}

	p.mu.Lock()
	wait := p.minInterval - time.Since(p.lastCall)
	if p.lastCall.IsZero() {
		wait = 0
	}
	p.lastCall = time.Now().Add(max(wait, 0))
	p.mu.Unlock()

	if wait <= 0 {
		return
	}

	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

func retryAfterDuration(header string) time.Duration {
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds <= 0 {
		return time.Second
	}

	return min(time.Duration(seconds)*time.Second, maxRetryAfter)
}

func decodeGraphQL(payload []byte) (searchResponse, error) {
	var decoded searchResponse

	err := json.Unmarshal(payload, &decoded)
	if err != nil {
		return searchResponse{}, fmt.Errorf("decode anilist response: %w", err)
	}
	if len(decoded.Errors) > 0 {
		return searchResponse{}, fmt.Errorf("%w: %s", ErrGraphQLError, decoded.Errors[0].Message)
	}

	return decoded, nil
}
