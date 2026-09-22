// Package anidb implements the AniDB metadata, poster, and episode catalog provider (FI-1 E8).
//
// Titles are resolved via the weekly anime-titles dump (cached ~7 days). Anime details and
// episode lists come from the HTTP API. Optional UDP FILE (ed2k+size) identity uses AUTH/FILE
// when SUDOSTREAM_ANIDB_USERNAME/PASSWORD are set, via network.UDPDialer (SOCKS5 UDP ASSOCIATE).
// Client-side spacing defaults to 4s to reduce ban risk.
package anidb

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sudoStream/internal/network"
	"sudoStream/internal/provider"
	"sync"
	"time"
	"unicode"
)

// Key is the registry key for this adapter.
const Key = "anidb"

const (
	defaultBaseURL     = "http://api.anidb.net:9001/httpapi"
	defaultTitlesURL   = "https://anidb.net/api/anime-titles.xml.gz"
	defaultImageBase   = "https://cdn.anidb.net/images/main"
	defaultMinInterval = 4 * time.Second
	defaultClientVer   = "1"
	defaultProtoVer    = "1"
	maxPosterBytes     = 8 << 20
	titlesCacheTTL     = 7 * 24 * time.Hour
	errorBodyMaxLen    = 120
	yearPrefixLen      = 4
)

const (
	// EnvClient is the registered AniDB HTTP API client id (FI-1 L13).
	EnvClient = "SUDOSTREAM_ANIDB_CLIENT"
	// EnvUsername is the AniDB website username for UDP AUTH (file-hash path).
	EnvUsername = "SUDOSTREAM_ANIDB_USERNAME"
	// EnvPassword is the AniDB website password for UDP AUTH (file-hash path).
	EnvPassword = "SUDOSTREAM_ANIDB_PASSWORD" //nolint:gosec // G101: env var name, not a secret

	defaultUDPAddr    = "api.anidb.net:9000"
	defaultSessionTTL = 18 * time.Minute
)

var (
	// ErrMissingCredentials is returned when the client id is unset.
	ErrMissingCredentials = errors.New("anidb credentials missing")
	// ErrRequestFailed is returned on unexpected HTTP statuses or API errors.
	ErrRequestFailed = errors.New("anidb request failed")
	// ErrPosterEmpty is returned when a match has no picture.
	ErrPosterEmpty = errors.New("anidb poster empty")
	// ErrMissingAnimeID is returned when FetchEpisodes lacks an AniDB id.
	ErrMissingAnimeID = errors.New("anidb anime id missing")

	errNotFound = errors.New("anidb not found")
)

// Provider queries AniDB HTTP API + titles dump (+ optional UDP FILE by ed2k).
type Provider struct {
	baseURL   string
	titlesURL string
	imageBase string
	cacheDir  string
	client    string
	http      *provider.ThrottledClient

	titles *titlesIndex

	udpDialer    network.UDPDialer
	udpAddr      string
	username     string
	password     string
	sessionTTL   time.Duration
	forceSession string // tests: skip AUTH and reuse this session key

	udpMu      sync.Mutex
	sessionKey string
	sessionAt  time.Time
}

// Option customizes a Provider.
type Option func(*Provider)

// WithBaseURL overrides the HTTP API endpoint (tests).
func WithBaseURL(base string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimRight(base, "/") }
}

// WithTitlesURL overrides the anime-titles dump URL (tests).
func WithTitlesURL(raw string) Option {
	return func(p *Provider) { p.titlesURL = strings.TrimSpace(raw) }
}

// WithCacheDir overrides where the titles dump is cached (tests).
func WithCacheDir(dir string) Option {
	return func(p *Provider) { p.cacheDir = strings.TrimSpace(dir) }
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
		p.http = provider.NewThrottledClient(interval).WithProvider("anidb")
	}
}

// WithClient sets the registered AniDB client id (tests).
func WithClient(client string) Option {
	return func(p *Provider) { p.client = strings.ToLower(strings.TrimSpace(client)) }
}

// WithUDPDialer sets the UDP path used for FILE ed2k lookups (SOCKS-aware Live in production).
func WithUDPDialer(dialer network.UDPDialer) Option {
	return func(p *Provider) { p.udpDialer = dialer }
}

// WithUDPAddr overrides the AniDB UDP endpoint (default api.anidb.net:9000).
func WithUDPAddr(hostPort string) Option {
	return func(p *Provider) {
		if strings.TrimSpace(hostPort) != "" {
			p.udpAddr = strings.TrimSpace(hostPort)
		}
	}
}

// WithCredentials sets UDP AUTH username/password (tests).
func WithCredentials(user, pass string) Option {
	return func(p *Provider) {
		p.username = strings.TrimSpace(user)
		p.password = pass
	}
}

// WithSession injects a pre-authed UDP session key so tests can skip AUTH.
func WithSession(session string) Option {
	return func(p *Provider) { p.forceSession = strings.TrimSpace(session) }
}

// New constructs the AniDB adapter.
func New(opts ...Option) *Provider {
	instance := &Provider{
		baseURL:    defaultBaseURL,
		titlesURL:  defaultTitlesURL,
		imageBase:  defaultImageBase,
		cacheDir:   filepath.Join(provider.DefaultCacheRoot, "anidb"),
		client:     strings.ToLower(provider.Env(EnvClient)),
		http:       provider.NewThrottledClient(defaultMinInterval).WithProvider("anidb"),
		udpAddr:    defaultUDPAddr,
		username:   provider.Env(EnvUsername),
		password:   provider.Env(EnvPassword),
		sessionTTL: defaultSessionTTL,
	}
	for _, opt := range opts {
		opt(instance)
	}

	return instance
}

// Key returns the registry key.
func (p *Provider) Key() string { return Key }

// RequiredEnv lists the AniDB client env var (FI-1 L13).
func (p *Provider) RequiredEnv() []string {
	if strings.TrimSpace(p.client) != "" || provider.Env(EnvClient) != "" {
		return nil
	}

	return []string{EnvClient}
}

// MatchShow matches a series show once per show.
func (p *Provider) MatchShow(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.ShowFields, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.ShowFields{}, err
	}
	anime, status, err := p.resolve(ctx, hint.IDs.AnidbID, hint.Show)
	if err != nil || status != provider.MatchOK {
		return status, provider.ShowFields{}, err
	}

	return provider.MatchOK, anime.toShowFields(), nil
}

// MatchFilm matches a film once per file.
func (p *Provider) MatchFilm(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.FilmFields, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.FilmFields{}, err
	}
	anime, status, err := p.resolve(ctx, hint.IDs.AnidbID, hint.Title)
	if err != nil || status != provider.MatchOK {
		return status, provider.FilmFields{}, err
	}

	return provider.MatchOK, anime.toFilmFields(), nil
}

// FetchShowPoster downloads cover art for a series show.
func (p *Provider) FetchShowPoster(
	ctx context.Context,
	hint provider.ShowHint,
) (provider.MatchStatus, provider.Art, error) {
	return p.fetchPoster(ctx, hint.IDs.AnidbID, hint.Show)
}

// FetchFilmPoster downloads cover art for a film file.
func (p *Provider) FetchFilmPoster(
	ctx context.Context,
	hint provider.FilmHint,
) (provider.MatchStatus, provider.Art, error) {
	return p.fetchPoster(ctx, hint.IDs.AnidbID, hint.Title)
}

// FetchEpisodes maps regular (type=1) episode numbers to English-preferred titles.
func (p *Provider) FetchEpisodes(
	ctx context.Context,
	fields provider.ShowFields,
) (map[provider.EpisodeKey]provider.EpisodeInfo, error) {
	err := p.ensureCreds()
	if err != nil {
		return nil, err
	}
	aid := anidbIDFromFields(fields)
	if aid == "" {
		return nil, ErrMissingAnimeID
	}
	anime, err := p.getAnime(ctx, aid)
	if err != nil {
		return nil, err
	}

	return mapEpisodes(anime.Episodes.Items), nil
}

func (p *Provider) fetchPoster(
	ctx context.Context,
	storedID *string,
	query string,
) (provider.MatchStatus, provider.Art, error) {
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.Art{}, err
	}
	anime, status, err := p.resolve(ctx, storedID, query)
	if err != nil || status != provider.MatchOK {
		return status, provider.Art{}, err
	}
	imageURL := p.posterURL(anime.Picture)
	if imageURL == "" {
		return provider.MatchOK, provider.Art{}, ErrPosterEmpty
	}
	art, err := p.downloadArt(ctx, imageURL)
	if err != nil {
		return provider.MatchOK, provider.Art{}, err
	}
	aid := strconv.Itoa(anime.ID)
	art.ExternalID = &aid

	return provider.MatchOK, art, nil
}

func (p *Provider) resolve(
	ctx context.Context,
	storedID *string,
	query string,
) (animeDTO, provider.MatchStatus, error) {
	if storedID != nil && strings.TrimSpace(*storedID) != "" {
		anime, err := p.getAnime(ctx, strings.TrimSpace(*storedID))
		if err != nil {
			if errors.Is(err, errNotFound) {
				return animeDTO{}, provider.MatchNone, nil
			}

			return animeDTO{}, provider.MatchNone, err
		}

		return anime, provider.MatchOK, nil
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return animeDTO{}, provider.MatchNone, nil
	}
	index, err := p.loadTitles(ctx)
	if err != nil {
		return animeDTO{}, provider.MatchNone, err
	}
	aid, status := index.search(query)
	if status != provider.MatchOK {
		return animeDTO{}, status, nil
	}
	anime, err := p.getAnime(ctx, aid)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return animeDTO{}, provider.MatchNone, nil
		}

		return animeDTO{}, provider.MatchNone, err
	}

	return anime, provider.MatchOK, nil
}

func (p *Provider) ensureCreds() error {
	if strings.TrimSpace(p.client) == "" {
		p.client = strings.ToLower(provider.Env(EnvClient))
	}
	if p.client == "" {
		return fmt.Errorf("%w: set %s", ErrMissingCredentials, EnvClient)
	}

	return nil
}

func (p *Provider) getAnime(ctx context.Context, aid string) (animeDTO, error) {
	params := url.Values{
		"request":   {"anime"},
		"client":    {p.client},
		"clientver": {defaultClientVer},
		"protover":  {defaultProtoVer},
		"aid":       {aid},
	}
	body, err := p.get(ctx, p.baseURL+"?"+params.Encode())
	if err != nil {
		return animeDTO{}, err
	}
	if looksBanned(body) {
		return animeDTO{}, fmt.Errorf("%w: %w: banned", provider.ErrProviderUnavailable, ErrRequestFailed)
	}
	if isErrorXML(body) {
		if looksNotFound(body) {
			return animeDTO{}, errNotFound
		}

		return animeDTO{}, fmt.Errorf("%w: %s", ErrRequestFailed, truncate(string(body)))
	}
	var anime animeDTO
	err = xml.Unmarshal(body, &anime)
	if err != nil {
		return animeDTO{}, fmt.Errorf("decode anidb anime: %w", err)
	}
	if anime.ID == 0 {
		return animeDTO{}, errNotFound
	}

	return anime, nil
}

func (p *Provider) get(ctx context.Context, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build anidb request: %w", err)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return nil, fmt.Errorf("anidb http: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
		data, readErr := provider.ReadLimited(response.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read anidb body: %w", readErr)
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

func (p *Provider) downloadArt(ctx context.Context, imageURL string) (provider.Art, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return provider.Art{}, fmt.Errorf("build anidb poster request: %w", err)
	}
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return provider.Art{}, fmt.Errorf("fetch anidb poster: %w", err)
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
		return provider.Art{}, fmt.Errorf("read anidb poster: %w", err)
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

func (p *Provider) posterURL(picture string) string {
	picture = strings.TrimSpace(picture)
	if picture == "" {
		return ""
	}
	if strings.HasPrefix(picture, "http://") || strings.HasPrefix(picture, "https://") {
		return picture
	}

	return strings.TrimRight(p.imageBase, "/") + "/" + strings.TrimPrefix(picture, "/")
}

func anidbIDFromFields(fields provider.ShowFields) string {
	if fields.IDs.AnidbID != nil {
		return strings.TrimSpace(*fields.IDs.AnidbID)
	}
	if fields.ExternalID != nil {
		return strings.TrimSpace(*fields.ExternalID)
	}

	return ""
}

func mapEpisodes(rows []episodeDTO) map[provider.EpisodeKey]provider.EpisodeInfo {
	out := make(map[provider.EpisodeKey]provider.EpisodeInfo)
	for _, row := range rows {
		if strings.TrimSpace(row.EpNo.Type) != "1" {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSpace(row.EpNo.Value))
		if err != nil || number <= 0 {
			continue
		}
		title := pickEpisodeTitle(row.Titles)
		if title == "" {
			continue
		}
		// AniDB regular episodes are seasonless; map as S1 so filename SxxExx matches.
		out[provider.EpisodeKey{Season: 1, Episode: number}] = provider.EpisodeInfo{Title: title}
	}

	return out
}

func pickEpisodeTitle(titles []titleDTO) string {
	var english, anyTitle string
	for _, title := range titles {
		value := strings.TrimSpace(title.Value)
		if value == "" {
			continue
		}
		if anyTitle == "" {
			anyTitle = value
		}
		if strings.EqualFold(title.Lang, "en") {
			english = value
		}
	}
	if english != "" {
		return english
	}

	return anyTitle
}

func (a animeDTO) toShowFields() provider.ShowFields {
	aid := strconv.Itoa(a.ID)
	fields := provider.ShowFields{
		Title:       stringPtr(a.bestTitle()),
		Description: stringPtr(strings.TrimSpace(a.Description)),
		Genres:      a.genres(),
		Year:        a.year(),
		ExternalID:  &aid,
		IDs:         provider.ExternalIDs{AnidbID: &aid},
	}

	return fields
}

func (a animeDTO) toFilmFields() provider.FilmFields {
	show := a.toShowFields()

	return provider.FilmFields{
		Title:       show.Title,
		Description: show.Description,
		Genres:      show.Genres,
		Year:        show.Year,
		ExternalID:  show.ExternalID,
		IDs:         show.IDs,
	}
}

func (a animeDTO) bestTitle() string {
	mainTitle := firstTitleOfType(a.Titles.Items, "main")
	if mainTitle != "" {
		return mainTitle
	}
	official := firstTitleOfTypeLang(a.Titles.Items, "official", "en")
	if official != "" {
		return official
	}

	return firstNonEmptyTitle(a.Titles.Items)
}

func firstTitleOfType(titles []titleDTO, wantType string) string {
	var fallback string
	for _, title := range titles {
		value := strings.TrimSpace(title.Value)
		if value == "" || !strings.EqualFold(title.Type, wantType) {
			continue
		}
		if strings.EqualFold(title.Lang, "en") {
			return value
		}
		if fallback == "" {
			fallback = value
		}
	}

	return fallback
}

func firstTitleOfTypeLang(titles []titleDTO, wantType, wantLang string) string {
	for _, title := range titles {
		value := strings.TrimSpace(title.Value)
		if value == "" {
			continue
		}
		if strings.EqualFold(title.Type, wantType) && strings.EqualFold(title.Lang, wantLang) {
			return value
		}
	}

	return ""
}

func firstNonEmptyTitle(titles []titleDTO) string {
	for _, title := range titles {
		value := strings.TrimSpace(title.Value)
		if value != "" {
			return value
		}
	}

	return ""
}

func (a animeDTO) genres() []string {
	out := make([]string, 0, len(a.Categories.Items))
	seen := map[string]struct{}{}
	for _, category := range a.Categories.Items {
		name := strings.TrimSpace(category.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}

	return out
}

func (a animeDTO) year() *int {
	raw := strings.TrimSpace(a.StartDate)
	if len(raw) < yearPrefixLen {
		return nil
	}
	year, err := strconv.Atoi(raw[:yearPrefixLen])
	if err != nil || year <= 0 {
		return nil
	}

	return &year
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	return &value
}

func looksBanned(body []byte) bool {
	lower := strings.ToLower(string(body))

	return strings.Contains(lower, "banned") || strings.Contains(lower, "client banned")
}

func isErrorXML(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))

	return strings.HasPrefix(strings.ToLower(trimmed), "<error")
}

func looksNotFound(body []byte) bool {
	lower := strings.ToLower(string(body))

	return strings.Contains(lower, "not found") || strings.Contains(lower, "unknown anime")
}

func truncate(value string) string {
	const limit = errorBodyMaxLen
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}

	return value[:limit] + "…"
}

func normalizeTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	builder.Grow(len(value))
	prevSpace := false
	for _, runeValue := range value {
		if unicode.IsSpace(runeValue) || runeValue == '_' || runeValue == '-' {
			if !prevSpace && builder.Len() > 0 {
				builder.WriteByte(' ')
				prevSpace = true
			}

			continue
		}
		prevSpace = false
		builder.WriteRune(runeValue)
	}

	return strings.TrimSpace(builder.String())
}

type animeDTO struct {
	ID          int          `xml:"id,attr"`
	Titles      titlesWrap   `xml:"titles"`
	Description string       `xml:"description"`
	StartDate   string       `xml:"startdate"`
	Picture     string       `xml:"picture"`
	Categories  categoryWrap `xml:"categories"`
	Episodes    episodeWrap  `xml:"episodes"`
}

type titlesWrap struct {
	Items []titleDTO `xml:"title"`
}

type titleDTO struct {
	Lang  string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type categoryWrap struct {
	Items []categoryDTO `xml:"category"`
}

type categoryDTO struct {
	Name string `xml:"name"`
}

type episodeWrap struct {
	Items []episodeDTO `xml:"episode"`
}

type episodeDTO struct {
	ID     int        `xml:"id,attr"`
	EpNo   epNoDTO    `xml:"epno"`
	Titles []titleDTO `xml:"title"`
}

type epNoDTO struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}
