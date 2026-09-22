package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/metadata"
	"sudoStream/internal/observability"
	"time"
)

var (
	// ErrProviderNotRegistered is returned when stored settings name an adapter that this build
	// does not contain (e.g. after downgrading a release).
	ErrProviderNotRegistered = errors.New("configured provider is not registered")
	// ErrUnsupportedLibraryType is returned for non film/series libraries (FI-1 L1).
	ErrUnsupportedLibraryType = errors.New("provider tasks support film and series libraries only")
	// ErrEnricherUnavailable is returned when the enricher is missing a required dependency.
	ErrEnricherUnavailable = errors.New("provider enricher unavailable")
)

// IsProviderAbort reports whether err should stop the rest of a library provider run
// (HTTP 403, exhausted rate limits, etc.).
func IsProviderAbort(err error) bool {
	return errors.Is(err, ErrProviderUnavailable)
}

// LibraryLoader loads the library a task runs against.
type LibraryLoader interface {
	GetLibrary(ctx context.Context, libraryID string) (access.Library, error)
}

// SettingsLoader loads a library's provider configuration.
type SettingsLoader interface {
	GetSettings(ctx context.Context, libraryID string) (Settings, error)
}

// MetadataGateway reads effective metadata and writes provider-provenance fields.
type MetadataGateway interface {
	Get(ctx context.Context, rawPath string) (metadata.MetadataResponse, error)
	ApplyProviderFields(
		ctx context.Context,
		rawPath string,
		target metadata.PatchTarget,
		fields metadata.ProviderFields,
		providerKey string,
	) (metadata.MetadataResponse, error)
}

// EnricherDeps wires the provider tasks into domain services.
type EnricherDeps struct {
	Libraries  LibraryLoader
	Settings   SettingsLoader
	Registry   *Registry
	Metadata   MetadataGateway
	Artifacts  ArtifactStore
	Cache      *Cache
	ListVideos func(roots []string) ([]string, error)
	// ResolvePath maps a media-relative path to an absolute filesystem path (mediafs.FilePath).
	ResolvePath func(relPath string) (string, error)
	// LocalSubtitleLanguages returns ISO 639-1 codes already present via sidecar or embedded
	// tracks. Provider search skips only those languages; other configured langs still fetch.
	// When nil, local presence is not checked.
	LocalSubtitleLanguages func(ctx context.Context, absPath string) (map[string]struct{}, error)
}

// Enricher runs the three provider maintenance tasks over a whole library (FI-1 L20).
type Enricher struct {
	deps EnricherDeps
}

// NewEnricher constructs the provider task runner.
func NewEnricher(deps EnricherDeps) *Enricher {
	return &Enricher{deps: deps}
}

// Enrich runs one provider task for a library. Unconfigured slots are a successful no-op.
func (e *Enricher) Enrich(
	ctx context.Context,
	libraryID string,
	kind TaskKind,
) (RunSummary, error) {
	library, settings, err := e.load(ctx, libraryID)
	if err != nil {
		return RunSummary{}, err
	}

	switch kind {
	case TaskMetadata:
		summary, err := e.runMetadata(ctx, library, settings)
		recordProviderTaskMetrics(kind, summary)

		return summary, err
	case TaskPoster:
		summary, err := e.runPosters(ctx, library, settings)
		recordProviderTaskMetrics(kind, summary)

		return summary, err
	case TaskSubtitle:
		summary, err := e.runSubtitles(ctx, library, settings)
		recordProviderTaskMetrics(kind, summary)

		return summary, err
	default:
		return RunSummary{}, ErrProviderNotRegistered
	}
}

func recordProviderTaskMetrics(kind TaskKind, summary RunSummary) {
	if !summary.ProviderConfigured {
		return
	}

	observability.RecordProviderTaskItems(summary.ProviderKey, string(kind), map[string]int{
		"applied":           summary.Applied,
		"skipped_user":      summary.SkippedUser,
		"unmatched":         summary.Unmatched,
		"skipped_uncertain": summary.SkippedUncertain,
		"pruned":            summary.Pruned,
		"errors":            summary.Errors,
	})
}

func (e *Enricher) load(
	ctx context.Context,
	libraryID string,
) (access.Library, Settings, error) {
	if e == nil || e.deps.Libraries == nil || e.deps.Settings == nil || e.deps.Registry == nil {
		return access.Library{}, Settings{}, ErrEnricherUnavailable
	}

	library, err := e.deps.Libraries.GetLibrary(ctx, libraryID)
	if err != nil {
		return access.Library{}, Settings{}, fmt.Errorf("get library: %w", err)
	}
	if library.Type != access.LibraryTypeFilm && library.Type != access.LibraryTypeSeries {
		return access.Library{}, Settings{}, ErrUnsupportedLibraryType
	}

	settings, err := e.deps.Settings.GetSettings(ctx, libraryID)
	if err != nil {
		return access.Library{}, Settings{}, fmt.Errorf("get provider settings: %w", err)
	}

	return library, settings, nil
}

func (e *Enricher) runMetadata( //nolint:cyclop // film vs series + abort branches
	ctx context.Context,
	library access.Library,
	settings Settings,
) (RunSummary, error) {
	var summary RunSummary

	if settings.MetadataProvider == nil {
		return summary, nil
	}

	adapter, ok := e.deps.Registry.MetadataAdapter(*settings.MetadataProvider)
	if !ok {
		return summary, fmt.Errorf("%w: %s", ErrProviderNotRegistered, *settings.MetadataProvider)
	}
	if e.deps.Metadata == nil || e.deps.ListVideos == nil {
		return summary, ErrEnricherUnavailable
	}
	summary.ProviderConfigured = true
	summary.ProviderKey = adapter.Key()

	paths, err := e.deps.ListVideos(library.RootPaths())
	if err != nil {
		return summary, fmt.Errorf("list library videos: %w", err)
	}

	if library.Type == access.LibraryTypeFilm {
		for _, relPath := range paths {
			err = e.applyFilmMetadata(ctx, adapter, library, settings, relPath, &summary)
			if IsProviderAbort(err) {
				e.logAbort("providers.metadata", adapter.Key(), library, err)

				return summary, err
			}
		}

		return summary, nil
	}

	for _, show := range e.groupShows(ctx, paths) {
		err = e.applyShowMetadata(ctx, adapter, library, settings, show, &summary)
		if IsProviderAbort(err) {
			e.logAbort("providers.metadata", adapter.Key(), library, err)

			return summary, err
		}
	}

	return summary, nil
}

func (e *Enricher) applyFilmMetadata(
	ctx context.Context,
	adapter MetadataProvider,
	library access.Library,
	settings Settings,
	relPath string,
	summary *RunSummary,
) error {
	current, err := e.deps.Metadata.Get(ctx, relPath)
	if err != nil {
		e.logUnit("providers.metadata", adapter.Key(), library, relPath, "read metadata failed", err)
		summary.Errors++

		return nil
	}
	if e.skipUserEdited(current, settings, library, relPath, summary) {
		return nil
	}

	ok, abortErr := e.tryFileHashMetadata(ctx, adapter, library, settings, relPath, current, summary)
	if abortErr != nil {
		return abortErr
	}
	if ok {
		return nil
	}

	hint := FilmHint{
		Title: effectiveTitle(current, relPath),
		Year:  current.Effective.Year,
		IDs:   filmIDsFrom(current),
	}

	status, fields, err := adapter.MatchFilm(ctx, hint)
	if err != nil {
		e.logUnit("providers.metadata", adapter.Key(), library, relPath, "match failed", err)
		summary.Errors++
		if IsProviderAbort(err) {
			return fmt.Errorf("match film: %w", err)
		}

		return nil
	}
	if !countMatch(summary, status) {
		e.logSkip("providers.metadata", adapter.Key(), library, relPath, status)

		return nil
	}

	e.writeFields(ctx, library, settings, adapter.Key(), relPath, metadata.ProviderFields{
		Title:       fields.Title,
		Description: fields.Description,
		Genres:      fields.Genres,
		Studio:      fields.Studio,
		Year:        fields.Year,
		IMDBID:      fields.IDs.ImdbID,
		TMDBID:      fields.IDs.TmdbID,
		TVDBID:      fields.IDs.TvdbID,
		TvmazeID:    fields.IDs.TvmazeID,
	}, current, summary)

	return nil
}

func (e *Enricher) applyShowMetadata( //nolint:cyclop,funlen // catalog + per-file-hash + episode write + abort
	ctx context.Context,
	adapter MetadataProvider,
	library access.Library,
	settings Settings,
	show showGroup,
	summary *RunSummary,
) error {
	remaining := make([]string, 0, len(show.paths))
	for _, relPath := range show.paths {
		current, getErr := e.deps.Metadata.Get(ctx, relPath)
		if getErr != nil {
			e.logUnit("providers.metadata", adapter.Key(), library, relPath, "read metadata failed", getErr)
			summary.Errors++

			continue
		}
		if e.skipUserEdited(current, settings, library, relPath, summary) {
			continue
		}
		handled, abortErr := e.tryFileHashMetadata(ctx, adapter, library, settings, relPath, current, summary)
		if abortErr != nil {
			return abortErr
		}
		if handled {
			continue
		}
		remaining = append(remaining, relPath)
	}
	if len(remaining) == 0 {
		return nil
	}
	show.paths = remaining

	hint := e.showHint(ctx, show)
	status, fields, err := adapter.MatchShow(ctx, hint)
	if err != nil {
		e.logUnit("providers.metadata", adapter.Key(), library, show.posterPath, "match failed", err)
		summary.Errors++
		if IsProviderAbort(err) {
			return fmt.Errorf("match show: %w", err)
		}

		return nil
	}
	if !countMatch(summary, status) {
		e.logSkip("providers.metadata", adapter.Key(), library, show.posterPath, status)

		return nil
	}

	episodes := map[EpisodeKey]EpisodeInfo{}
	if catalog, ok := adapter.(EpisodeCatalogProvider); ok {
		fetched, fetchErr := catalog.FetchEpisodes(ctx, fields)
		if fetchErr != nil {
			e.logUnit("providers.metadata", adapter.Key(), library, show.posterPath, "episode catalog failed", fetchErr)
			summary.Errors++
			if IsProviderAbort(fetchErr) {
				return fmt.Errorf("fetch episodes: %w", fetchErr)
			}
		} else {
			episodes = fetched
		}
	}

	for _, relPath := range show.paths {
		current, getErr := e.deps.Metadata.Get(ctx, relPath)
		if getErr != nil {
			e.logUnit("providers.metadata", adapter.Key(), library, relPath, "read metadata failed", getErr)
			summary.Errors++

			continue
		}
		if e.skipUserEdited(current, settings, library, relPath, summary) {
			continue
		}

		patch := metadata.ProviderFields{
			Show:        fields.Title,
			Description: fields.Description,
			Genres:      fields.Genres,
			Studio:      fields.Studio,
			Year:        fields.Year,
			IMDBID:      fields.IDs.ImdbID,
			TMDBID:      fields.IDs.TmdbID,
			TVDBID:      fields.IDs.TvdbID,
			TvmazeID:    fields.IDs.TvmazeID,
		}
		if fields.IDs.TvmazeID == nil {
			patch.TvmazeID = fields.ExternalID
		}
		e.applyEpisodeCatalog(&patch, current, relPath, episodes)
		e.writeFields(ctx, library, settings, adapter.Key(), relPath, patch, current, summary)
	}

	return nil
}

// tryFileHashMetadata attempts FileHashProvider when ed2k + size are stored.
// handled=true means do not fall back to title match. abortErr stops the library run.
func (e *Enricher) tryFileHashMetadata( //nolint:cyclop // soft-fallback vs abort branches
	ctx context.Context,
	adapter MetadataProvider,
	library access.Library,
	settings Settings,
	relPath string,
	current metadata.MetadataResponse,
	summary *RunSummary,
) (bool, error) {
	hasher, ok := adapter.(FileHashProvider)
	if !ok {
		return false, nil
	}
	ed2k := ""
	if current.Effective.Ed2kHash != nil {
		ed2k = strings.TrimSpace(*current.Effective.Ed2kHash)
	}
	var hashSize int64
	if current.Effective.HashFileSize != nil {
		hashSize = *current.Effective.HashFileSize
	}
	if ed2k == "" || hashSize <= 0 {
		return false, nil
	}

	hint := FileHashHint{
		RelPath:      relPath,
		Ed2kHash:     ed2k,
		HashFileSize: hashSize,
	}
	if e.deps.ResolvePath != nil {
		abs, resolveErr := e.deps.ResolvePath(relPath)
		if resolveErr == nil {
			hint.AbsPath = abs
		}
	}

	status, fields, err := hasher.MatchFileHash(ctx, hint)
	if err != nil {
		e.logUnit("providers.metadata", adapter.Key(), library, relPath, "file hash match failed", err)
		summary.Errors++
		if IsProviderAbort(err) {
			return true, fmt.Errorf("match file hash: %w", err)
		}

		return false, nil
	}
	if status != MatchOK {
		return false, nil
	}
	if !countMatch(summary, status) {
		return true, nil
	}

	patch := metadata.ProviderFields{
		Title:        fields.Title,
		Show:         fields.Show,
		Description:  fields.Description,
		Genres:       fields.Genres,
		Studio:       fields.Studio,
		Year:         fields.Year,
		EpisodeTitle: fields.EpisodeTitle,
		Season:       fields.Season,
		Episode:      fields.Episode,
		IMDBID:       fields.IDs.ImdbID,
		TMDBID:       fields.IDs.TmdbID,
		TVDBID:       fields.IDs.TvdbID,
		TvmazeID:     fields.IDs.TvmazeID,
	}
	e.writeFields(ctx, library, settings, adapter.Key(), relPath, patch, current, summary)

	return true, nil
}

func (e *Enricher) showHint(ctx context.Context, show showGroup) ShowHint {
	hint := ShowHint{Show: show.name, Year: show.year}
	if e.deps.Metadata == nil {
		return hint
	}
	current, err := e.deps.Metadata.Get(ctx, show.posterPath)
	if err != nil {
		return hint
	}
	hint.IDs = ExternalIDs{
		ImdbID:   current.Effective.IMDBID,
		TmdbID:   current.Effective.TMDBID,
		TvdbID:   current.Effective.TVDBID,
		TvmazeID: current.Effective.TvmazeID,
	}

	return hint
}

func (e *Enricher) applyEpisodeCatalog(
	patch *metadata.ProviderFields,
	current metadata.MetadataResponse,
	relPath string,
	episodes map[EpisodeKey]EpisodeInfo,
) {
	if len(episodes) == 0 {
		return
	}
	season, episode := current.Effective.Season, current.Effective.Episode
	if season == nil || episode == nil {
		parsed := metadata.ParseSeriesIdentity(relPath)
		if season == nil {
			season = parsed.Season
		}
		if episode == nil {
			episode = parsed.Episode
		}
	}
	if season == nil || episode == nil {
		return
	}
	info, ok := episodes[EpisodeKey{Season: *season, Episode: *episode}]
	if !ok || strings.TrimSpace(info.Title) == "" {
		return
	}
	title := strings.TrimSpace(info.Title)
	patch.EpisodeTitle = &title
}

func (e *Enricher) skipUserEdited(
	current metadata.MetadataResponse,
	settings Settings,
	library access.Library,
	relPath string,
	summary *RunSummary,
) bool {
	if settings.AllowOverrideUserMetadata || !metadata.IsUserOverridden(current.OverriddenBy) {
		return false
	}

	summary.SkippedUser++
	slog.Debug("provider metadata skipped user-edited file",
		slog.String("action", "providers.metadata"),
		slog.String("library_id", library.ID),
		slog.String("path", relPath),
	)

	return true
}

func (e *Enricher) writeFields(
	ctx context.Context,
	library access.Library,
	settings Settings,
	providerKey, relPath string,
	fields metadata.ProviderFields,
	current metadata.MetadataResponse,
	summary *RunSummary,
) {
	if settings.MetadataApplyMode != ApplyModeFullRewrite {
		fields = fillMissingOnly(fields, current.Effective)
	}
	if providerFieldsEmpty(fields) {
		return
	}

	target := metadata.PatchTargetOverride
	if settings.MetadataWriteTarget == WriteTargetFile {
		target = metadata.PatchTargetFile
	}

	_, err := e.deps.Metadata.ApplyProviderFields(ctx, relPath, target, fields, providerKey)
	if err != nil {
		e.logUnit("providers.metadata", providerKey, library, relPath, "apply failed", err)
		summary.Errors++

		return
	}

	summary.Applied++
}

func (e *Enricher) runPosters(
	ctx context.Context,
	library access.Library,
	settings Settings,
) (RunSummary, error) {
	var summary RunSummary

	if settings.PosterProvider == nil {
		return summary, nil
	}

	adapter, ok := e.deps.Registry.PosterAdapter(*settings.PosterProvider)
	if !ok {
		return summary, fmt.Errorf("%w: %s", ErrProviderNotRegistered, *settings.PosterProvider)
	}
	if e.deps.Artifacts == nil || e.deps.Cache == nil || e.deps.ListVideos == nil {
		return summary, ErrEnricherUnavailable
	}
	summary.ProviderConfigured = true
	summary.ProviderKey = adapter.Key()

	paths, err := e.deps.ListVideos(library.RootPaths())
	if err != nil {
		return summary, fmt.Errorf("list library videos: %w", err)
	}

	targets := e.posterTargets(ctx, library, paths)
	for relPath, hint := range targets {
		err = e.fetchPoster(ctx, adapter, library, relPath, hint, &summary)
		if IsProviderAbort(err) {
			e.logAbort("providers.posters", adapter.Key(), library, err)

			return summary, err
		}
	}

	e.prunePosters(ctx, library, adapter.Key(), targets, &summary)

	return summary, nil
}

// posterHint carries whichever hint the adapter needs for one poster target.
type posterHint struct {
	film *FilmHint
	show *ShowHint
}

// posterTargets maps the rel paths that own a poster to their provider hint. Films are 1:1 with
// files; series reuse catalog's PosterPath convention (lexicographically-first episode of the
// show) so the catalog and the provider cache agree on which file represents a show.
func (e *Enricher) posterTargets(
	ctx context.Context,
	library access.Library,
	paths []string,
) map[string]posterHint {
	targets := make(map[string]posterHint, len(paths))

	if library.Type == access.LibraryTypeFilm {
		for _, relPath := range paths {
			hint := FilmHint{Title: metadata.ParseFilmIdentity(relPath).Title}
			if e.deps.Metadata != nil {
				current, err := e.deps.Metadata.Get(ctx, relPath)
				if err == nil {
					hint = FilmHint{
						Title: effectiveTitle(current, relPath),
						Year:  current.Effective.Year,
						IDs:   filmIDsFrom(current),
					}
				}
			}
			targets[relPath] = posterHint{film: &hint}
		}

		return targets
	}

	for _, show := range e.groupShows(ctx, paths) {
		hint := e.showHint(ctx, show)
		targets[show.posterPath] = posterHint{show: &hint}
	}

	return targets
}

func filmIDsFrom(current metadata.MetadataResponse) ExternalIDs {
	return ExternalIDs{
		ImdbID:   current.Effective.IMDBID,
		TmdbID:   current.Effective.TMDBID,
		TvdbID:   current.Effective.TVDBID,
		TvmazeID: current.Effective.TvmazeID,
	}
}

func (e *Enricher) fetchPoster(
	ctx context.Context,
	adapter PosterProvider,
	library access.Library,
	relPath string,
	hint posterHint,
	summary *RunSummary,
) error {
	if e.posterCached(ctx, library.ID, relPath, adapter.Key()) {
		return nil
	}

	status, art, err := e.fetchArt(ctx, adapter, hint)
	if err != nil {
		e.logUnit("providers.posters", adapter.Key(), library, relPath, "fetch failed", err)
		summary.Errors++
		if IsProviderAbort(err) {
			return fmt.Errorf("fetch poster: %w", err)
		}

		return nil
	}
	if !countMatch(summary, status) {
		e.logSkip("providers.posters", adapter.Key(), library, relPath, status)

		return nil
	}
	if len(art.Bytes) == 0 {
		summary.Unmatched++

		return nil
	}

	cachePath := PosterCachePath(library.ID, relPath, art.ContentType)

	err = e.deps.Cache.Write(cachePath, art.Bytes)
	if err != nil {
		e.logUnit("providers.posters", adapter.Key(), library, relPath, "cache write failed", err)
		summary.Errors++

		return nil
	}

	_, err = e.deps.Artifacts.UpsertArtifact(ctx, Artifact{
		LibraryID:   library.ID,
		RelPath:     relPath,
		Kind:        ArtifactKindPoster,
		ProviderKey: adapter.Key(),
		CachePath:   cachePath,
		ExternalID:  art.ExternalID,
		FetchedAt:   time.Now().UTC(),
	})
	if err != nil {
		e.logUnit("providers.posters", adapter.Key(), library, relPath, "artifact upsert failed", err)
		summary.Errors++

		return nil
	}

	summary.Applied++

	return nil
}

func (e *Enricher) fetchArt(
	ctx context.Context,
	adapter PosterProvider,
	hint posterHint,
) (MatchStatus, Art, error) {
	var (
		status MatchStatus
		art    Art
		err    error
	)

	switch {
	case hint.film != nil:
		status, art, err = adapter.FetchFilmPoster(ctx, *hint.film)
	case hint.show != nil:
		status, art, err = adapter.FetchShowPoster(ctx, *hint.show)
	default:
		return MatchNone, Art{}, nil
	}

	if err != nil {
		return status, Art{}, fmt.Errorf("fetch provider poster: %w", err)
	}

	return status, art, nil
}

// posterCached reports whether a usable artifact from the same provider already exists, so a
// re-run does not re-download every poster in the library.
func (e *Enricher) posterCached(
	ctx context.Context,
	libraryID, relPath, providerKey string,
) bool {
	existing, err := e.deps.Artifacts.GetArtifactByPath(ctx, relPath, ArtifactKindPoster, nil)
	if err != nil || existing.ProviderKey != providerKey || existing.LibraryID != libraryID {
		return false
	}

	absPath, err := e.deps.Cache.Path(existing.CachePath)
	if err != nil {
		return false
	}

	return fileExists(absPath)
}

// prunePosters drops rows (and cache files) for media that no longer exists under the library
// roots, or that is no longer the show's representative file after a rename (FI-1: stale
// artifact cleanup piggybacks on the scheduled task).
func (e *Enricher) prunePosters(
	ctx context.Context,
	library access.Library,
	providerKey string,
	targets map[string]posterHint,
	summary *RunSummary,
) {
	rows, err := e.deps.Artifacts.ListArtifacts(ctx, library.ID, ArtifactKindPoster)
	if err != nil {
		e.logUnit("providers.posters", providerKey, library, "", "list artifacts failed", err)
		summary.Errors++

		return
	}

	for _, row := range rows {
		if _, keep := targets[row.RelPath]; keep {
			continue
		}

		removeErr := e.deps.Cache.Remove(row.CachePath)
		if removeErr != nil {
			e.logUnit("providers.posters", providerKey, library, row.RelPath, "cache remove failed", removeErr)
		}

		err = e.deps.Artifacts.DeleteArtifact(ctx, row.ID)
		if err != nil {
			e.logUnit("providers.posters", providerKey, library, row.RelPath, "artifact delete failed", err)
			summary.Errors++

			continue
		}

		summary.Pruned++
	}
}

func (e *Enricher) runSubtitles(
	ctx context.Context,
	library access.Library,
	settings Settings,
) (RunSummary, error) {
	var summary RunSummary

	if settings.SubtitleProvider == nil || len(settings.SubtitleLanguages) == 0 {
		return summary, nil
	}

	adapter, ok := e.deps.Registry.SubtitleAdapter(*settings.SubtitleProvider)
	if !ok {
		return summary, fmt.Errorf("%w: %s", ErrProviderNotRegistered, *settings.SubtitleProvider)
	}
	if e.deps.Artifacts == nil || e.deps.Cache == nil || e.deps.ListVideos == nil {
		return summary, ErrEnricherUnavailable
	}
	summary.ProviderConfigured = true
	summary.ProviderKey = adapter.Key()

	paths, err := e.deps.ListVideos(library.RootPaths())
	if err != nil {
		return summary, fmt.Errorf("list library videos: %w", err)
	}

	keep := make(map[string]struct{}, len(paths)*len(settings.SubtitleLanguages))
	for _, relPath := range paths {
		e.collectSubtitleFetches(ctx, adapter, library, settings, relPath, keep, &summary)
	}

	e.pruneSubtitles(ctx, library, adapter.Key(), keep, &summary)

	return summary, nil
}

func (e *Enricher) collectSubtitleFetches(
	ctx context.Context,
	adapter SubtitleProvider,
	library access.Library,
	settings Settings,
	relPath string,
	keep map[string]struct{},
	summary *RunSummary,
) {
	localLangs := e.localSubtitleLanguages(ctx, library, adapter.Key(), relPath)
	for _, lang := range settings.SubtitleLanguages {
		if _, covered := localLangs[lang]; covered {
			slog.Info("provider subtitle skipped; local language present",
				slog.String("library_id", library.ID),
				slog.String("path", relPath),
				slog.String("lang", lang),
			)

			continue
		}
		keep[relPath+"|"+lang] = struct{}{}
		e.fetchSubtitle(ctx, adapter, library, settings, relPath, lang, summary)
	}
}

func (e *Enricher) localSubtitleLanguages(
	ctx context.Context,
	library access.Library,
	providerKey, relPath string,
) map[string]struct{} {
	if e.deps.LocalSubtitleLanguages == nil || e.deps.ResolvePath == nil {
		return nil
	}

	absPath, err := e.deps.ResolvePath(relPath)
	if err != nil {
		e.logUnit("providers.subtitles", providerKey, library, relPath, "resolve for local subtitle check failed", err)

		return nil
	}

	langs, err := e.deps.LocalSubtitleLanguages(ctx, absPath)
	if err != nil {
		e.logUnit("providers.subtitles", providerKey, library, relPath, "local subtitle check failed", err)

		return nil
	}

	return langs
}

func (e *Enricher) fetchSubtitle(
	ctx context.Context,
	adapter SubtitleProvider,
	library access.Library,
	settings Settings,
	relPath, lang string,
	summary *RunSummary,
) {
	_ = settings
	if e.subtitleCached(ctx, library.ID, relPath, lang, adapter.Key()) {
		return
	}

	hint := e.subtitleHint(ctx, relPath)
	status, file, err := adapter.FetchSubtitle(ctx, hint, lang)
	if err != nil {
		e.logUnit("providers.subtitles", adapter.Key(), library, relPath, "fetch failed", err)
		summary.Errors++

		return
	}
	if !countMatch(summary, status) {
		e.logSkip("providers.subtitles", adapter.Key(), library, relPath, status)

		return
	}
	if len(file.Bytes) == 0 {
		summary.Unmatched++

		return
	}

	cachePath := SubtitleCachePath(library.ID, relPath, lang)
	err = e.deps.Cache.Write(cachePath, file.Bytes)
	if err != nil {
		e.logUnit("providers.subtitles", adapter.Key(), library, relPath, "cache write failed", err)
		summary.Errors++

		return
	}

	langCopy := lang
	_, err = e.deps.Artifacts.UpsertArtifact(ctx, Artifact{
		LibraryID:   library.ID,
		RelPath:     relPath,
		Kind:        ArtifactKindSubtitle,
		Lang:        &langCopy,
		ProviderKey: adapter.Key(),
		CachePath:   cachePath,
		ExternalID:  file.ExternalID,
		FetchedAt:   time.Now().UTC(),
	})
	if err != nil {
		e.logUnit("providers.subtitles", adapter.Key(), library, relPath, "artifact upsert failed", err)
		summary.Errors++

		return
	}

	summary.Applied++
}

func (e *Enricher) subtitleHint(ctx context.Context, relPath string) SubtitleHint {
	hint := SubtitleHint{RelPath: relPath, Title: filepath.Base(relPath)}
	if e.deps.ResolvePath != nil {
		abs, resolveErr := e.deps.ResolvePath(relPath)
		if resolveErr == nil {
			hint.AbsPath = abs
		}
	}
	if e.deps.Metadata == nil {
		parsed := metadata.ParseSeriesIdentity(relPath)
		if parsed.Show != "" {
			show := parsed.Show
			hint.Show = &show
			hint.Season = parsed.Season
			hint.Episode = parsed.Episode
		} else {
			hint.Title = metadata.ParseFilmIdentity(relPath).Title
		}

		return hint
	}

	current, err := e.deps.Metadata.Get(ctx, relPath)
	if err != nil {
		return hint
	}

	hint.Title = effectiveTitle(current, relPath)
	hint.Show = current.Effective.Show
	hint.Season = current.Effective.Season
	hint.Episode = current.Effective.Episode
	hint.ImdbID = current.Effective.IMDBID
	hint.TmdbID = current.Effective.TMDBID
	if current.Effective.MovieHash != nil {
		hint.MovieHash = strings.TrimSpace(*current.Effective.MovieHash)
	}
	if current.Effective.HashFileSize != nil {
		hint.MovieHashSize = *current.Effective.HashFileSize
	}

	return hint
}

func (e *Enricher) subtitleCached(
	ctx context.Context,
	libraryID, relPath, lang, providerKey string,
) bool {
	existing, err := e.deps.Artifacts.GetArtifactByPath(ctx, relPath, ArtifactKindSubtitle, &lang)
	if err != nil || existing.ProviderKey != providerKey || existing.LibraryID != libraryID {
		return false
	}

	absPath, err := e.deps.Cache.Path(existing.CachePath)
	if err != nil {
		return false
	}

	return fileExists(absPath)
}

func (e *Enricher) pruneSubtitles(
	ctx context.Context,
	library access.Library,
	providerKey string,
	keep map[string]struct{},
	summary *RunSummary,
) {
	rows, err := e.deps.Artifacts.ListArtifacts(ctx, library.ID, ArtifactKindSubtitle)
	if err != nil {
		e.logUnit("providers.subtitles", providerKey, library, "", "list artifacts failed", err)
		summary.Errors++

		return
	}

	for _, row := range rows {
		lang := ""
		if row.Lang != nil {
			lang = *row.Lang
		}
		if _, ok := keep[row.RelPath+"|"+lang]; ok {
			continue
		}

		removeErr := e.deps.Cache.Remove(row.CachePath)
		if removeErr != nil {
			e.logUnit("providers.subtitles", providerKey, library, row.RelPath, "cache remove failed", removeErr)
		}

		err = e.deps.Artifacts.DeleteArtifact(ctx, row.ID)
		if err != nil {
			e.logUnit("providers.subtitles", providerKey, library, row.RelPath, "artifact delete failed", err)
			summary.Errors++

			continue
		}

		summary.Pruned++
	}
}

func (e *Enricher) logUnit(
	action, providerKey string,
	library access.Library,
	relPath, msg string,
	err error,
) {
	slog.Warn("provider task unit failed",
		slog.String("action", action),
		slog.String("provider", providerKey),
		slog.String("library_id", library.ID),
		slog.String("path", relPath),
		slog.String("reason", msg),
		slog.String("error", err.Error()),
	)
}

func (e *Enricher) logAbort(action, providerKey string, library access.Library, err error) {
	slog.Error("provider task aborted",
		slog.String("action", action),
		slog.String("provider", providerKey),
		slog.String("library_id", library.ID),
		slog.String("error", err.Error()),
	)
}

func (e *Enricher) logSkip(
	action, providerKey string,
	library access.Library,
	relPath string,
	status MatchStatus,
) {
	slog.Info("provider task skipped unmatched item",
		slog.String("action", action),
		slog.String("provider", providerKey),
		slog.String("library_id", library.ID),
		slog.String("path", relPath),
		slog.String("match", string(status)),
	)
}
