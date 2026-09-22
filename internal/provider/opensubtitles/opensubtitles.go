// Package opensubtitles implements the OpenSubtitles.com subtitle provider (FI-1 E3).
package opensubtitles

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
	"sudoStream/internal/mediahash"
	"sudoStream/internal/provider"
	"sync"
	"time"
)

// Key is the registry key for this adapter.
const Key = "opensubtitles"

const (
	defaultBaseURL     = "https://api.opensubtitles.com/api/v1"
	defaultUserAgent   = "sudoStream v1.0"
	defaultMinInterval = 250 * time.Millisecond
	defaultTimeout     = 30 * time.Second
	maxDownloadBytes   = 4 << 20
	truncateBodyLen    = 200
)

// Env keys (FI-1 L13 — own app key; do not embed Jellyfin's).
const (
	//nolint:gosec // G101: env var *names*, not secrets
	EnvAPIKey = "SUDOSTREAM_OPENSUBTITLES_API_KEY"

	EnvUsername = "SUDOSTREAM_OPENSUBTITLES_USERNAME"
	//nolint:gosec // G101: env var name
	EnvPassword = "SUDOSTREAM_OPENSUBTITLES_PASSWORD"
)

var (
	// ErrMissingCredentials is returned when required env vars are unset.
	ErrMissingCredentials = errors.New("opensubtitles credentials missing")
	// ErrRequestFailed is returned on non-success HTTP statuses.
	ErrRequestFailed = errors.New("opensubtitles request failed")
	// ErrQuotaExceeded is returned when the daily download quota is hit.
	ErrQuotaExceeded = errors.New("opensubtitles download quota exceeded")
	// ErrInvalidLanguage is returned when lang is not ISO 639-1.
	ErrInvalidLanguage = errors.New("invalid subtitle language")
	// ErrFileTooSmall is returned when the media file is under 64KiB.
	ErrFileTooSmall = mediahash.ErrFileTooSmall
	// ErrEmptyDownloadLink is returned when /download omits link.
	ErrEmptyDownloadLink = errors.New("opensubtitles empty download link")
	// ErrEmptyLoginToken is returned when /login omits token.
	ErrEmptyLoginToken = errors.New("opensubtitles empty login token")
)

// Provider fetches subtitles from OpenSubtitles.com.
type Provider struct {
	baseURL     string
	client      *http.Client
	userAgent   string
	minInterval time.Duration
	apiKey      string
	username    string
	password    string

	mu       sync.Mutex
	lastCall time.Time
	token    string
}

// Option customizes a Provider.
type Option func(*Provider)

// WithBaseURL overrides the API base URL (tests).
func WithBaseURL(base string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimRight(base, "/") }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) { p.client = client }
}

// WithMinInterval overrides the throttle between calls.
func WithMinInterval(interval time.Duration) Option {
	return func(p *Provider) { p.minInterval = interval }
}

// WithCredentials sets credentials explicitly (tests).
func WithCredentials(apiKey, username, password string) Option {
	return func(p *Provider) {
		p.apiKey = apiKey
		p.username = username
		p.password = password
	}
}

// New constructs the OpenSubtitles adapter. Credentials default from env.
func New(opts ...Option) *Provider {
	instance := &Provider{
		baseURL:     defaultBaseURL,
		client:      &http.Client{Timeout: defaultTimeout},
		userAgent:   defaultUserAgent,
		minInterval: defaultMinInterval,
		apiKey:      provider.Env(EnvAPIKey),
		username:    provider.Env(EnvUsername),
		password:    provider.Env(EnvPassword),
	}
	for _, opt := range opts {
		opt(instance)
	}

	return instance
}

// Key returns the registry key.
func (p *Provider) Key() string { return Key }

// RequiredEnv lists credential env vars (FI-1 L13).
func (p *Provider) RequiredEnv() []string {
	return []string{EnvAPIKey, EnvUsername, EnvPassword}
}

// FetchSubtitle matches and downloads one subtitle track for lang.
func (p *Provider) FetchSubtitle(
	ctx context.Context,
	hint provider.SubtitleHint,
	lang string,
) (provider.MatchStatus, provider.SubtitleFile, error) {
	err := p.ensureCredentials()
	if err != nil {
		return provider.MatchNone, provider.SubtitleFile{}, err
	}

	lang = strings.ToLower(strings.TrimSpace(lang))
	if !provider.IsSubtitleLanguage(lang) {
		return provider.MatchNone, provider.SubtitleFile{}, fmt.Errorf("%w: %s", ErrInvalidLanguage, lang)
	}

	fileID, status, err := p.search(ctx, hint, lang)
	if err != nil || status != provider.MatchOK {
		return status, provider.SubtitleFile{}, err
	}

	payload, err := p.download(ctx, fileID)
	if err != nil {
		return provider.MatchOK, provider.SubtitleFile{}, err
	}

	externalID := strconv.Itoa(fileID)

	return provider.MatchOK, provider.SubtitleFile{
		Bytes:      ensureVTT(payload),
		Lang:       lang,
		ExternalID: &externalID,
	}, nil
}

func (p *Provider) ensureCredentials() error {
	if strings.TrimSpace(p.apiKey) == "" ||
		strings.TrimSpace(p.username) == "" ||
		strings.TrimSpace(p.password) == "" {
		return fmt.Errorf("%w: set %s, %s, %s", ErrMissingCredentials, EnvAPIKey, EnvUsername, EnvPassword)
	}

	return nil
}

func (p *Provider) search(
	ctx context.Context,
	hint provider.SubtitleHint,
	lang string,
) (int, provider.MatchStatus, error) {
	fileID, status, err := p.searchByStoredOrPathHash(ctx, hint, lang)
	if err != nil {
		return 0, provider.MatchNone, err
	}
	if status == provider.MatchOK || status == provider.MatchUncertain {
		return fileID, status, nil
	}

	return p.searchByQuery(ctx, hint, lang)
}

func (p *Provider) searchByStoredOrPathHash(
	ctx context.Context,
	hint provider.SubtitleHint,
	lang string,
) (int, provider.MatchStatus, error) {
	hash := strings.TrimSpace(hint.MovieHash)
	size := hint.MovieHashSize
	if hash == "" || size <= 0 {
		if hint.AbsPath == "" {
			return 0, provider.MatchNone, nil
		}
		var err error
		hash, size, err = MovieHash(hint.AbsPath)
		if err != nil {
			return 0, provider.MatchNone, nil //nolint:nilerr // fall through to query search
		}
	}

	params := url.Values{}
	params.Set("languages", lang)
	params.Set("moviehash", hash)
	params.Set("moviebytesize", strconv.FormatInt(size, 10))

	return p.searchOnce(ctx, params, true)
}

func (p *Provider) searchByQuery(
	ctx context.Context,
	hint provider.SubtitleHint,
	lang string,
) (int, provider.MatchStatus, error) {
	params := url.Values{}
	params.Set("languages", lang)
	putOptional(params, "imdb_id", stripTT(hint.ImdbID))
	putOptional(params, "tmdb_id", deref(hint.TmdbID))
	if hint.Season != nil {
		params.Set("season_number", strconv.Itoa(*hint.Season))
	}
	if hint.Episode != nil {
		params.Set("episode_number", strconv.Itoa(*hint.Episode))
	}
	query := strings.TrimSpace(hint.Title)
	if hint.Show != nil && strings.TrimSpace(*hint.Show) != "" {
		query = strings.TrimSpace(*hint.Show)
	}
	if query != "" {
		params.Set("query", query)
	}
	if len(params) <= 1 {
		return 0, provider.MatchNone, nil
	}

	return p.searchOnce(ctx, params, false)
}

func (p *Provider) searchOnce(
	ctx context.Context,
	params url.Values,
	hashSearch bool,
) (int, provider.MatchStatus, error) {
	err := p.ensureLogin(ctx)
	if err != nil {
		return 0, provider.MatchNone, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		p.baseURL+"/subtitles?"+params.Encode(),
		nil,
	)
	if err != nil {
		return 0, provider.MatchNone, fmt.Errorf("build opensubtitles search: %w", err)
	}
	p.setAuthHeaders(req)

	resp, err := p.do(req)
	if err != nil {
		return 0, provider.MatchNone, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes))
	if err != nil {
		return 0, provider.MatchNone, fmt.Errorf("read opensubtitles search: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, provider.MatchNone, fmt.Errorf("%w: search %d", ErrRequestFailed, resp.StatusCode)
	}

	candidates, err := parseSearchFileIDs(body)
	if err != nil {
		return 0, provider.MatchNone, err
	}

	return pickSearchMatch(candidates, hashSearch)
}

func parseSearchFileIDs(body []byte) ([]int, error) {
	var parsed searchResponse

	err := json.Unmarshal(body, &parsed)
	if err != nil {
		return nil, fmt.Errorf("decode opensubtitles search: %w", err)
	}

	candidates := make([]int, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		for _, file := range item.Attributes.Files {
			if file.FileID > 0 {
				candidates = append(candidates, file.FileID)
			}
		}
	}

	return candidates, nil
}

func pickSearchMatch(candidates []int, hashSearch bool) (int, provider.MatchStatus, error) {
	switch len(candidates) {
	case 0:
		return 0, provider.MatchNone, nil
	case 1:
		return candidates[0], provider.MatchOK, nil
	default:
		if hashSearch {
			return candidates[0], provider.MatchOK, nil
		}

		return 0, provider.MatchUncertain, nil
	}
}

func (p *Provider) download(ctx context.Context, fileID int) ([]byte, error) {
	err := p.ensureLogin(ctx)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(map[string]any{"file_id": fileID})
	if err != nil {
		return nil, fmt.Errorf("encode opensubtitles download: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/download",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("build opensubtitles download: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	p.setAuthHeaders(req)

	resp, err := p.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	return p.readDownload(ctx, resp)
}

func (p *Provider) readDownload(ctx context.Context, resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes))
	if err != nil {
		return nil, fmt.Errorf("read opensubtitles download: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusPaymentRequired {
		return nil, ErrQuotaExceeded
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: download %d: %s", ErrRequestFailed, resp.StatusCode, truncate(body))
	}

	var parsed downloadResponse

	err = json.Unmarshal(body, &parsed)
	if err != nil {
		return nil, fmt.Errorf("decode opensubtitles download: %w", err)
	}
	if strings.TrimSpace(parsed.Link) == "" {
		return nil, ErrEmptyDownloadLink
	}

	return p.fetchLink(ctx, parsed.Link)
}

func (p *Provider) fetchLink(ctx context.Context, link string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, fmt.Errorf("build opensubtitles link fetch: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch opensubtitles link: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: fetch link %d", ErrRequestFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes))
	if err != nil {
		return nil, fmt.Errorf("read opensubtitles link: %w", err)
	}

	return body, nil
}

func (p *Provider) ensureLogin(ctx context.Context) error {
	p.mu.Lock()
	token := p.token
	p.mu.Unlock()
	if token != "" {
		return nil
	}

	payload, err := json.Marshal(map[string]string{
		"username": p.username,
		"password": p.password,
	})
	if err != nil {
		return fmt.Errorf("encode opensubtitles login: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.baseURL+"/login",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("build opensubtitles login: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", p.apiKey)
	req.Header.Set("User-Agent", p.userAgent)

	resp, err := p.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes))
	if err != nil {
		return fmt.Errorf("read opensubtitles login: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: login %d", ErrRequestFailed, resp.StatusCode)
	}

	var parsed loginResponse

	err = json.Unmarshal(body, &parsed)
	if err != nil {
		return fmt.Errorf("decode opensubtitles login: %w", err)
	}
	if strings.TrimSpace(parsed.Token) == "" {
		return ErrEmptyLoginToken
	}

	p.mu.Lock()
	p.token = parsed.Token
	p.mu.Unlock()

	return nil
}

func (p *Provider) setAuthHeaders(req *http.Request) {
	req.Header.Set("Api-Key", p.apiKey)
	req.Header.Set("User-Agent", p.userAgent)
	p.mu.Lock()
	token := p.token
	p.mu.Unlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (p *Provider) do(req *http.Request) (*http.Response, error) {
	p.wait(req.Context())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles http: %w", err)
	}

	return resp, nil
}

func (p *Provider) wait(ctx context.Context) {
	if p.minInterval <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	wait := p.minInterval - time.Since(p.lastCall)
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
	p.lastCall = time.Now()
}

// MovieHash computes the OpenSubtitles moviehash for path and returns hash + filesize.
func MovieHash(path string) (string, int64, error) {
	hash, size, err := mediahash.MovieHash(path)
	if err != nil {
		return "", size, fmt.Errorf("opensubtitles moviehash: %w", err)
	}

	return hash, size, nil
}

func ensureVTT(data []byte) []byte {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("WEBVTT")) {
		return data
	}

	return srtToVTT(data)
}

func srtToVTT(data []byte) []byte {
	text := string(data)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, ",", ".")
	var out strings.Builder
	out.WriteString("WEBVTT\n\n")
	out.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		out.WriteByte('\n')
	}

	return []byte(out.String())
}

func truncate(body []byte) string {
	if len(body) <= truncateBodyLen {
		return string(body)
	}

	return string(body[:truncateBodyLen])
}

func putOptional(params url.Values, key, value string) {
	if value != "" {
		params.Set(key, value)
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

func stripTT(value *string) string {
	return strings.TrimPrefix(deref(value), "tt")
}

type loginResponse struct {
	Token string `json:"token"`
}

type searchResponse struct {
	Data []struct {
		Attributes struct {
			Files []struct {
				FileID int `json:"file_id"` //nolint:tagliatelle // OpenSubtitles API wire format
			} `json:"files"`
		} `json:"attributes"`
	} `json:"data"`
}

type downloadResponse struct {
	Link string `json:"link"`
}
