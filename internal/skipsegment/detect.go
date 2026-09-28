package skipsegment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/transcode"
	"time"
)

var errServiceUnavailable = errors.New("skipsegment service unavailable")

// Store persists detected intro segments.
type Store interface {
	Upsert(ctx context.Context, segment StoredSegment) error
	Get(ctx context.Context, libraryID, relPath string) (StoredSegment, bool, error)
	DeleteForPath(ctx context.Context, libraryID, relPath string) error
}

// Catalog lists series shows/seasons/episodes for offline detection.
type Catalog interface {
	ListCatalogShowsPage(
		ctx context.Context,
		libraryID string,
		limit, offset int,
	) ([]metadata.CatalogShowRow, int, error)
	GetCatalogShowAgg(
		ctx context.Context,
		libraryID, showKey string,
	) (metadata.CatalogShowAgg, bool, error)
	ListCatalogSeasonEpisodes(
		ctx context.Context,
		libraryID, showKey string,
		season, limit, offset int,
	) ([]metadata.CatalogEpisodeRow, int, error)
}

// ProbeDuration returns media duration seconds for scan-window sizing.
type ProbeDuration func(ctx context.Context, absPath string) (float64, error)

// ProbeSourceFn inspects a media file (chapters + duration) for detect.
type ProbeSourceFn func(ctx context.Context, absPath string) (transcode.SourceInfo, error)

// ExtractFn builds a chromaprint for the intro scan window.
type ExtractFn func(
	ctx context.Context,
	absPath string,
	durationSeconds float64,
) ([]uint32, float64, error)

// RunSummary counts for maintenance.
type RunSummary struct {
	Seasons     int `json:"seasons"`
	Episodes    int `json:"episodes"`
	AudioHits   int `json:"audioHits"`
	Skipped     int `json:"skipped"`
	Errors      int `json:"errors"`
	ChapterSkip int `json:"chapterSkip"`
}

// Map converts the summary for maintenance JSON.
func (s RunSummary) Map() map[string]any {
	return map[string]any{
		"seasons":     s.Seasons,
		"episodes":    s.Episodes,
		"audioHits":   s.AudioHits,
		"skipped":     s.Skipped,
		"errors":      s.Errors,
		"chapterSkip": s.ChapterSkip,
	}
}

// LibraryLookup loads a library for detect scoping.
type LibraryLookup interface {
	GetLibrary(ctx context.Context, libraryID string) (access.Library, error)
}

// Service runs offline intro detection and serves stored segments.
type Service struct {
	store   Store
	media   *mediafs.Service
	access  LibraryLookup
	catalog Catalog
	probe   ProbeDuration
	source  ProbeSourceFn
	extract ExtractFn
}

// NewService constructs a skip-intro detector.
func NewService(
	store Store,
	media *mediafs.Service,
	accessService LibraryLookup,
	catalog Catalog,
) *Service {
	return &Service{
		store:   store,
		media:   media,
		access:  accessService,
		catalog: catalog,
		probe:   defaultProbeDuration,
		source:  transcode.ProbeSource,
		extract: ExtractFingerprint,
	}
}

// SetProbeDuration replaces the duration probe (tests).
func (s *Service) SetProbeDuration(probe ProbeDuration) {
	if s == nil {
		return
	}
	s.probe = probe
}

// SetProbeSource replaces the media probe (tests).
func (s *Service) SetProbeSource(probe ProbeSourceFn) {
	if s == nil {
		return
	}
	s.source = probe
}

// SetExtract replaces chromaprint extraction (tests).
func (s *Service) SetExtract(extract ExtractFn) {
	if s == nil {
		return
	}
	s.extract = extract
}

// LookupIntro returns a stored audio intro for playback merge.
func (s *Service) LookupIntro(ctx context.Context, libraryID, relPath string) *Intro {
	if s == nil || s.store == nil || libraryID == "" || relPath == "" {
		return nil
	}

	segment, found, err := s.store.Get(ctx, libraryID, relPath)
	if err != nil {
		slog.Debug("skip intro store get failed",
			slog.String("library_id", libraryID),
			slog.String("path", relPath),
			slog.String("err", err.Error()),
		)

		return nil
	}
	if !found || segment.EngineVersion != EngineVersion {
		return nil
	}

	return segment.ToIntro()
}

// DetectLibraryMap runs DetectLibrary and returns a maintenance summary map.
func (s *Service) DetectLibraryMap(ctx context.Context, libraryID string) (map[string]any, error) {
	summary, err := s.DetectLibrary(ctx, libraryID)

	return summary.Map(), err
}

// DetectLibrary runs audio intro detection for one series library.
func (s *Service) DetectLibrary(ctx context.Context, libraryID string) (RunSummary, error) {
	var summary RunSummary
	if s == nil || s.catalog == nil || s.access == nil || s.media == nil || s.store == nil {
		return summary, errServiceUnavailable
	}

	library, err := s.access.GetLibrary(ctx, libraryID)
	if err != nil {
		return summary, fmt.Errorf("get library: %w", err)
	}
	if library.Type != access.LibraryTypeSeries {
		summary.Skipped++

		return summary, nil
	}

	return s.detectAllShows(ctx, libraryID)
}

func (s *Service) detectAllShows(ctx context.Context, libraryID string) (RunSummary, error) {
	var summary RunSummary
	const pageSize = 50
	offset := 0
	for {
		err := ctx.Err()
		if err != nil {
			return summary, fmt.Errorf("skip intro detect canceled: %w", err)
		}
		shows, total, listErr := s.catalog.ListCatalogShowsPage(ctx, libraryID, pageSize, offset)
		if listErr != nil {
			return summary, fmt.Errorf("list shows: %w", listErr)
		}
		for _, show := range shows {
			s.accumulateShow(ctx, libraryID, show.ShowKey, &summary)
		}
		offset += len(shows)
		if offset >= total || len(shows) == 0 {
			break
		}
	}

	return summary, nil
}

func (s *Service) accumulateShow(
	ctx context.Context,
	libraryID, showKey string,
	summary *RunSummary,
) {
	agg, found, aggErr := s.catalog.GetCatalogShowAgg(ctx, libraryID, showKey)
	if aggErr != nil {
		summary.Errors++
		slog.Warn("skip intro show agg failed",
			slog.String("library_id", libraryID),
			slog.String("show_key", showKey),
			slog.String("err", aggErr.Error()),
		)

		return
	}
	if !found {
		return
	}
	for _, season := range agg.Seasons {
		seasonSummary, seasonErr := s.detectSeason(ctx, libraryID, showKey, season.Season)
		summary.Seasons++
		summary.Episodes += seasonSummary.Episodes
		summary.AudioHits += seasonSummary.AudioHits
		summary.Skipped += seasonSummary.Skipped
		summary.Errors += seasonSummary.Errors
		summary.ChapterSkip += seasonSummary.ChapterSkip
		if seasonErr != nil {
			summary.Errors++
			slog.Warn("skip intro season failed",
				slog.String("library_id", libraryID),
				slog.String("show_key", showKey),
				slog.Int("season", season.Season),
				slog.String("err", seasonErr.Error()),
			)
		}
	}
}

func (s *Service) detectSeason(
	ctx context.Context,
	libraryID, showKey string,
	season int,
) (RunSummary, error) {
	var summary RunSummary

	episodes, _, err := s.catalog.ListCatalogSeasonEpisodes(
		ctx, libraryID, showKey, season, 0, 0,
	)
	if err != nil {
		return summary, fmt.Errorf("list episodes: %w", err)
	}
	summary.Episodes = len(episodes)
	if len(episodes) < minEpisodesForAudio {
		summary.Skipped += len(episodes)

		return summary, nil
	}

	audio, extractSummary := s.extractSeasonAudio(ctx, episodes)
	summary.Skipped += extractSummary.Skipped
	summary.Errors += extractSummary.Errors
	summary.ChapterSkip += extractSummary.ChapterSkip
	if len(audio) < minEpisodesForAudio {
		summary.Skipped += len(audio)

		return summary, nil
	}

	hits := ConsensusIntros(audio)
	now := time.Now().UTC()
	for _, hit := range hits {
		segment := StoredSegment{
			LibraryID:     libraryID,
			RelPath:       hit.RelPath,
			Kind:          KindIntro,
			StartMs:       hit.StartMs,
			EndMs:         hit.EndMs,
			Source:        SourceAudio,
			Confidence:    hit.Confidence,
			EngineVersion: EngineVersion,
			ShowKey:       showKey,
			Season:        season,
			DetectedAt:    now,
		}
		upsertErr := s.store.Upsert(ctx, segment)
		if upsertErr != nil {
			summary.Errors++

			continue
		}
		summary.AudioHits++
	}

	return summary, nil
}

func (s *Service) extractSeasonAudio(
	ctx context.Context,
	episodes []metadata.CatalogEpisodeRow,
) ([]EpisodeAudio, RunSummary) {
	var summary RunSummary
	audio := make([]EpisodeAudio, 0, len(episodes))
	for _, episode := range episodes {
		err := ctx.Err()
		if err != nil {
			summary.Errors++

			break
		}
		item, status := s.extractEpisodeAudio(ctx, episode.RelPath)
		switch status {
		case extractOK:
			audio = append(audio, item)
		case extractChapter:
			summary.ChapterSkip++
		case extractSkip:
			summary.Skipped++
		case extractError:
			summary.Errors++
		}
	}

	return audio, summary
}

type extractStatus int

const (
	extractOK extractStatus = iota
	extractChapter
	extractSkip
	extractError
)

func (s *Service) extractEpisodeAudio(
	ctx context.Context,
	relPath string,
) (EpisodeAudio, extractStatus) {
	absPath, pathErr := s.media.FilePath(relPath)
	if pathErr != nil {
		return EpisodeAudio{}, extractError
	}

	sourceInfo, sourceErr := s.source(ctx, absPath)
	if sourceErr == nil {
		if chapterIntro := introFromProbe(sourceInfo); chapterIntro != nil {
			return EpisodeAudio{}, extractChapter
		}
	}

	duration, ok := s.durationFromProbe(ctx, absPath, sourceInfo, sourceErr)
	if !ok {
		return EpisodeAudio{}, extractError
	}

	fingerprint, window, fpErr := s.extract(ctx, absPath, duration)
	if fpErr != nil {
		if errors.Is(fpErr, ErrNoAudio) {
			return EpisodeAudio{}, extractSkip
		}

		return EpisodeAudio{}, extractError
	}

	return EpisodeAudio{
		RelPath:         relPath,
		DurationSeconds: duration,
		Fingerprint:     fingerprint,
		ScanWindowSec:   window,
	}, extractOK
}

func (s *Service) durationFromProbe(
	ctx context.Context,
	absPath string,
	sourceInfo transcode.SourceInfo,
	sourceErr error,
) (float64, bool) {
	if sourceErr == nil && sourceInfo.DurationSeconds > 0 {
		return sourceInfo.DurationSeconds, true
	}
	duration, probeErr := s.probe(ctx, absPath)
	if probeErr != nil || duration <= 0 {
		return 0, false
	}

	return duration, true
}

func introFromProbe(info transcode.SourceInfo) *Intro {
	if len(info.Chapters) == 0 || info.DurationSeconds <= 0 {
		return nil
	}
	cues := make([]ChapterCue, len(info.Chapters))
	for index, chapter := range info.Chapters {
		cues[index] = ChapterCue{
			StartSeconds: chapter.StartSeconds,
			EndSeconds:   chapter.EndSeconds,
			Title:        chapter.Title,
		}
	}

	return IntroFromChapters(cues, info.DurationSeconds)
}

func defaultProbeDuration(ctx context.Context, absPath string) (float64, error) {
	info, err := transcode.ProbeSource(ctx, absPath)
	if err != nil {
		return 0, fmt.Errorf("probe duration: %w", err)
	}

	return info.DurationSeconds, nil
}
