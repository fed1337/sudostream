package provider

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/metadata"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testLibraryID   = "lib-1"
	anilistKey      = "anilist"
	providerTitle   = "Steins;Gate"
	enrichFilmPath  = "lib/Movie.mkv"
	enrichFilmAPath = "lib/a.mkv"
	enrichShowE01   = "lib/Show A/S01E01.mkv"
	enrichShowE02   = "lib/Show A/S01E02.mkv"
	contentTypeWebP = "image/webp"
	contentTypeJPEG = "image/jpeg"
)

var errStoreBoom = errors.New("store boom")

// --- fakes ---------------------------------------------------------------------------------

type fakeLibraries struct {
	library access.Library
	err     error
}

func (f fakeLibraries) GetLibrary(_ context.Context, _ string) (access.Library, error) {
	return f.library, f.err
}

type fakeSettings struct {
	settings Settings
	err      error
}

func (f fakeSettings) GetSettings(_ context.Context, _ string) (Settings, error) {
	return f.settings, f.err
}

// fakeMetadata stores per-path effective fields and records provider writes.
type fakeMetadata struct {
	rows     map[string]metadata.MetadataResponse
	getErr   error
	applyErr error
	writes   map[string]metadata.ProviderFields
	targets  map[string]metadata.PatchTarget
	actor    string
}

func newFakeMetadata() *fakeMetadata {
	return &fakeMetadata{
		rows:    map[string]metadata.MetadataResponse{},
		writes:  map[string]metadata.ProviderFields{},
		targets: map[string]metadata.PatchTarget{},
	}
}

func (f *fakeMetadata) Get(
	_ context.Context,
	rawPath string,
) (metadata.MetadataResponse, error) {
	if f.getErr != nil {
		return metadata.MetadataResponse{}, f.getErr
	}

	row, ok := f.rows[rawPath]
	if !ok {
		return metadata.MetadataResponse{Path: rawPath}, nil
	}

	return row, nil
}

func (f *fakeMetadata) ApplyProviderFields(
	_ context.Context,
	rawPath string,
	target metadata.PatchTarget,
	fields metadata.ProviderFields,
	providerKey string,
) (metadata.MetadataResponse, error) {
	if f.applyErr != nil {
		return metadata.MetadataResponse{}, f.applyErr
	}

	f.writes[rawPath] = fields
	f.targets[rawPath] = target
	f.actor = providerKey

	return metadata.MetadataResponse{Path: rawPath}, nil
}

// fakeArtifacts is an in-memory ArtifactStore keyed like the unique DB index.
type fakeArtifacts struct {
	rows    map[string]Artifact
	nextID  int
	listErr error
	putErr  error
	delErr  error
}

func newFakeArtifacts() *fakeArtifacts {
	return &fakeArtifacts{rows: map[string]Artifact{}}
}

func artifactKey(libraryID, relPath, kind string, lang *string) string {
	code := ""
	if lang != nil {
		code = *lang
	}

	return strings.Join([]string{libraryID, relPath, kind, code}, "|")
}

func (f *fakeArtifacts) GetArtifactByPath(
	_ context.Context,
	relPath, kind string,
	lang *string,
) (Artifact, error) {
	code := ""
	if lang != nil {
		code = *lang
	}

	for _, row := range f.rows {
		rowLang := ""
		if row.Lang != nil {
			rowLang = *row.Lang
		}
		if row.RelPath == relPath && row.Kind == kind && rowLang == code {
			return row, nil
		}
	}

	return Artifact{}, ErrNotFound
}

func (f *fakeArtifacts) ListArtifacts(
	_ context.Context,
	libraryID, kind string,
) ([]Artifact, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	rows := make([]Artifact, 0, len(f.rows))
	for _, row := range f.rows {
		if row.LibraryID == libraryID && row.Kind == kind {
			rows = append(rows, row)
		}
	}

	return rows, nil
}

func (f *fakeArtifacts) ListArtifactsByPath(
	_ context.Context,
	relPath, kind string,
) ([]Artifact, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	rows := make([]Artifact, 0, len(f.rows))
	for _, row := range f.rows {
		if row.RelPath == relPath && row.Kind == kind {
			rows = append(rows, row)
		}
	}

	return rows, nil
}

func (f *fakeArtifacts) UpsertArtifact(_ context.Context, artifact Artifact) (Artifact, error) {
	if f.putErr != nil {
		return Artifact{}, f.putErr
	}

	f.nextID++
	if artifact.ID == "" {
		artifact.ID = "artifact-" + strconv.Itoa(f.nextID)
	}
	f.rows[artifactKey(artifact.LibraryID, artifact.RelPath, artifact.Kind, artifact.Lang)] = artifact

	return artifact, nil
}

func (f *fakeArtifacts) DeleteArtifact(_ context.Context, id string) error {
	if f.delErr != nil {
		return f.delErr
	}

	for key, row := range f.rows {
		if row.ID == id {
			delete(f.rows, key)
		}
	}

	return nil
}

// --- helpers -------------------------------------------------------------------------------

type enricherFixture struct {
	enricher  *Enricher
	metadata  *fakeMetadata
	artifacts *fakeArtifacts
	adapter   *fakeAdapter
	cache     *Cache
}

func newFixture(
	t *testing.T,
	libraryType access.LibraryType,
	settings Settings,
	paths []string,
) enricherFixture {
	t.Helper()

	adapter := newFakeAdapter(anilistKey)
	registry := NewRegistry()
	registry.RegisterMetadata(adapter)
	registry.RegisterPoster(adapter)

	cache, err := NewCache(filepath.Join(t.TempDir(), "providers"))
	if err != nil {
		t.Fatalf("cache: %v", err)
	}

	meta := newFakeMetadata()
	artifacts := newFakeArtifacts()

	enricher := NewEnricher(EnricherDeps{
		Libraries: fakeLibraries{library: access.Library{
			ID:      testLibraryID,
			Slug:    "lib",
			RelPath: "lib",
			Type:    libraryType,
		}},
		Settings:   fakeSettings{settings: settings},
		Registry:   registry,
		Metadata:   meta,
		Artifacts:  artifacts,
		Cache:      cache,
		ListVideos: func(_ []string) ([]string, error) { return paths, nil },
	})

	return enricherFixture{
		enricher:  enricher,
		metadata:  meta,
		artifacts: artifacts,
		adapter:   adapter,
		cache:     cache,
	}
}

func metadataSettings(mode, target string, allowOverride bool) Settings {
	key := anilistKey

	return Settings{
		LibraryID:                 testLibraryID,
		MetadataProvider:          &key,
		MetadataApplyMode:         mode,
		MetadataWriteTarget:       target,
		AllowOverrideUserMetadata: allowOverride,
	}
}

// --- tests ---------------------------------------------------------------------------------

func TestEnricher_Metadata_FilmFillMissing(t *testing.T) {
	t.Parallel()

	allure.Test(t, "film metadata fills only empty effective fields", func(a *allure.Context) {
		t := a.T()
		fixture := newFixture(
			t,
			access.LibraryTypeFilm,
			metadataSettings(ApplyModeFillMissing, WriteTargetDB, false),
			[]string{enrichFilmPath},
		)
		fixture.adapter.filmFields = FilmFields{
			Title:       new(providerTitle),
			Description: new("plot"),
			Studio:      new("White Fox"),
			Genres:      []string{"Sci-Fi"},
		}
		fixture.metadata.rows[enrichFilmPath] = metadata.MetadataResponse{
			Effective: metadata.VideoFields{Title: new("Local Title")},
		}

		summary, err := fixture.enricher.Enrich(context.Background(), testLibraryID, TaskMetadata)
		if err != nil {
			t.Fatalf("enrich: %v", err)
		}
		if summary.Applied != 1 || !summary.ProviderConfigured {
			t.Fatalf("summary: %+v", summary)
		}

		written := fixture.metadata.writes[enrichFilmPath]
		if written.Title != nil {
			t.Fatal("fill_missing must not overwrite an existing effective title")
		}
		if written.Description == nil || written.Studio == nil || len(written.Genres) != 1 {
			t.Fatalf("want empty fields filled, got %+v", written)
		}
		if fixture.metadata.targets[enrichFilmPath] != metadata.PatchTargetOverride {
			t.Fatalf("want db write target, got %s", fixture.metadata.targets[enrichFilmPath])
		}
		if fixture.metadata.actor != anilistKey {
			t.Fatalf("want provider provenance, got %q", fixture.metadata.actor)
		}
	})
}

func TestEnricher_Metadata_FilmFullRewriteToFile(t *testing.T) {
	t.Parallel()

	allure.Test(t, "full_rewrite overwrites effective values via file target", func(a *allure.Context) {
		t := a.T()
		fixture := newFixture(
			t,
			access.LibraryTypeFilm,
			metadataSettings(ApplyModeFullRewrite, WriteTargetFile, false),
			[]string{enrichFilmPath},
		)
		fixture.adapter.filmFields = FilmFields{Title: new(providerTitle)}
		fixture.metadata.rows[enrichFilmPath] = metadata.MetadataResponse{
			Effective: metadata.VideoFields{Title: new("Local Title")},
		}

		summary, err := fixture.enricher.Enrich(context.Background(), testLibraryID, TaskMetadata)
		if err != nil || summary.Applied != 1 {
			t.Fatalf("summary=%+v err=%v", summary, err)
		}

		written := fixture.metadata.writes[enrichFilmPath]
		if written.Title == nil || *written.Title != providerTitle {
			t.Fatalf("want provider title written, got %+v", written.Title)
		}
		if fixture.metadata.targets[enrichFilmPath] != metadata.PatchTargetFile {
			t.Fatal("want file write target")
		}
	})
}

func TestEnricher_Metadata_SkipsUserEditedUnlessAllowed(t *testing.T) {
	t.Parallel()

	allure.Test(t, "user-edited rows are skipped by default and written when allowed",
		func(a *allure.Context) {
			t := a.T()

			for _, allowOverride := range []bool{false, true} {
				fixture := newFixture(
					t,
					access.LibraryTypeFilm,
					metadataSettings(ApplyModeFullRewrite, WriteTargetDB, allowOverride),
					[]string{enrichFilmPath},
				)
				fixture.adapter.filmFields = FilmFields{Title: new(providerTitle)}
				fixture.metadata.rows[enrichFilmPath] = metadata.MetadataResponse{
					OverriddenBy: new("user:42"),
				}

				summary, err := fixture.enricher.Enrich(
					context.Background(),
					testLibraryID,
					TaskMetadata,
				)
				if err != nil {
					t.Fatalf("allowOverride=%v: %v", allowOverride, err)
				}
				if allowOverride && summary.Applied != 1 {
					t.Fatalf("allowOverride: want applied, got %+v", summary)
				}
				if !allowOverride && (summary.SkippedUser != 1 || summary.Applied != 0) {
					t.Fatalf("default: want skippedUser, got %+v", summary)
				}
			}
		})
}

func TestEnricher_Metadata_SeriesMatchesOncePerShow(t *testing.T) {
	t.Parallel()

	allure.Test(t, "series metadata matches per show and broadcasts show fields",
		func(a *allure.Context) {
			t := a.T()
			paths := []string{
				enrichShowE01,
				enrichShowE02,
				"lib/Show B/S01E01.mkv",
			}
			fixture := newFixture(
				t,
				access.LibraryTypeSeries,
				metadataSettings(ApplyModeFullRewrite, WriteTargetDB, false),
				paths,
			)
			fixture.adapter.showFields = ShowFields{
				Title:       new(providerTitle),
				Description: new("plot"),
			}
			for _, relPath := range paths {
				show := "Show A"
				if strings.Contains(relPath, "Show B") {
					show = "Show B"
				}
				fixture.metadata.rows[relPath] = metadata.MetadataResponse{
					Effective: metadata.VideoFields{Show: &show},
				}
			}

			summary, err := fixture.enricher.Enrich(
				context.Background(),
				testLibraryID,
				TaskMetadata,
			)
			if err != nil {
				t.Fatalf("enrich: %v", err)
			}
			if summary.Applied != len(paths) {
				t.Fatalf("want every episode written, got %+v", summary)
			}
			if fixture.adapter.matchCalls != 2 {
				t.Fatalf("want one match per show, got %d calls", fixture.adapter.matchCalls)
			}

			written := fixture.metadata.writes[enrichShowE02]
			if written.Show == nil || *written.Show != providerTitle {
				t.Fatalf("want provider title mapped to show, got %+v", written.Show)
			}
			if written.Title != nil {
				t.Fatal("series episodes must not receive a provider title field")
			}
		})
}

func TestEnricher_Metadata_CountsMatchOutcomes(t *testing.T) { //nolint:cyclop // table of match outcomes
	t.Parallel()

	allure.Test(t, "unmatched, uncertain, and failing units are counted not applied",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			settings := metadataSettings(ApplyModeFullRewrite, WriteTargetDB, false)

			uncertain := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
			uncertain.adapter.filmStatus = MatchUncertain
			summary, err := uncertain.enricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if err != nil || summary.SkippedUncertain != 1 || summary.Applied != 0 {
				t.Fatalf("uncertain: %+v %v", summary, err)
			}

			none := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
			none.adapter.filmStatus = MatchNone
			summary, err = none.enricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if err != nil || summary.Unmatched != 1 {
				t.Fatalf("none: %+v %v", summary, err)
			}

			matchErr := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
			matchErr.adapter.filmErr = errStoreBoom
			summary, err = matchErr.enricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if err != nil || summary.Errors != 1 {
				t.Fatalf("match error: %+v %v", summary, err)
			}

			abort := newFixture(
				t,
				access.LibraryTypeFilm,
				settings,
				[]string{enrichFilmAPath, enrichFilmPath},
			)
			abort.adapter.filmErr = ErrProviderUnavailable
			summary, err = abort.enricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if !errors.Is(err, ErrProviderUnavailable) {
				t.Fatalf("want abort: %+v %v", summary, err)
			}
			if summary.Errors != 1 || abort.adapter.matchCalls != 1 {
				t.Fatalf("want stop after first unavailable: summary=%+v calls=%d",
					summary, abort.adapter.matchCalls)
			}

			applyErr := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
			applyErr.adapter.filmFields = FilmFields{Title: new(providerTitle)}
			applyErr.metadata.applyErr = errStoreBoom
			summary, err = applyErr.enricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if err != nil || summary.Errors != 1 {
				t.Fatalf("apply error: %+v %v", summary, err)
			}

			readErr := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
			readErr.metadata.getErr = errStoreBoom
			summary, err = readErr.enricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if err != nil || summary.Errors != 1 {
				t.Fatalf("read error: %+v %v", summary, err)
			}
		})
}

func TestEnricher_Posters_CachesAndPrunes(t *testing.T) { //nolint:cyclop // poster cache + prune flow
	t.Parallel()

	allure.Test(t, "poster task caches art once per show and prunes stale rows",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			key := anilistKey
			settings := Settings{
				LibraryID:           testLibraryID,
				PosterProvider:      &key,
				MetadataApplyMode:   ApplyModeFillMissing,
				MetadataWriteTarget: WriteTargetDB,
			}
			fixture := newFixture(
				t,
				access.LibraryTypeSeries,
				settings,
				[]string{enrichShowE02, enrichShowE01},
			)
			fixture.adapter.art = Art{Bytes: []byte("webp"), ContentType: contentTypeWebP}
			for _, relPath := range []string{enrichShowE01, enrichShowE02} {
				fixture.metadata.rows[relPath] = metadata.MetadataResponse{
					Effective: metadata.VideoFields{Show: new("Show A")},
				}
			}

			// A row for a deleted file must be pruned along with its cache file.
			stale := PosterCachePath(testLibraryID, "lib/Gone/S01E01.mkv", contentTypeWebP)
			err := fixture.cache.Write(stale, []byte("old"))
			if err != nil {
				t.Fatalf("seed stale cache: %v", err)
			}
			_, err = fixture.artifacts.UpsertArtifact(ctx, Artifact{
				ID:          "stale-1",
				LibraryID:   testLibraryID,
				RelPath:     "lib/Gone/S01E01.mkv",
				Kind:        ArtifactKindPoster,
				ProviderKey: anilistKey,
				CachePath:   stale,
				FetchedAt:   time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("seed stale row: %v", err)
			}

			summary, err := fixture.enricher.Enrich(ctx, testLibraryID, TaskPoster)
			if err != nil {
				t.Fatalf("enrich posters: %v", err)
			}
			if summary.Applied != 1 || summary.Pruned != 1 {
				t.Fatalf("summary: %+v", summary)
			}
			if fixture.adapter.posterCalls != 1 {
				t.Fatalf("want one fetch per show, got %d", fixture.adapter.posterCalls)
			}

			// Lexicographically-first episode owns the poster (catalog PosterPath convention).
			row, err := fixture.artifacts.GetArtifactByPath(
				ctx,
				enrichShowE01,
				ArtifactKindPoster,
				nil,
			)
			if err != nil {
				t.Fatalf("poster row: %v", err)
			}
			_, pathErr := fixture.cache.Path(row.CachePath)
			if pathErr != nil {
				t.Fatalf("cache path: %v", pathErr)
			}
			_, staleErr := fixture.cache.Path(stale)
			if staleErr != nil {
				t.Fatalf("stale path: %v", staleErr)
			}

			// Re-running does not re-download an existing poster from the same provider.
			summary, err = fixture.enricher.Enrich(ctx, testLibraryID, TaskPoster)
			if err != nil || summary.Applied != 0 {
				t.Fatalf("rerun: %+v %v", summary, err)
			}
			if fixture.adapter.posterCalls != 1 {
				t.Fatalf("want cached poster reused, got %d calls", fixture.adapter.posterCalls)
			}
		})
}

func TestEnricher_Posters_FilmPerFileAndFailures(t *testing.T) { //nolint:cyclop // failure branches
	t.Parallel()

	allure.Test(t, "film posters fetch per file and count failures", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		key := anilistKey
		settings := Settings{
			LibraryID:           testLibraryID,
			PosterProvider:      &key,
			MetadataApplyMode:   ApplyModeFillMissing,
			MetadataWriteTarget: WriteTargetDB,
		}

		fixture := newFixture(
			t,
			access.LibraryTypeFilm,
			settings,
			[]string{enrichFilmAPath, "lib/b.mkv"},
		)
		fixture.adapter.art = Art{Bytes: []byte("jpg"), ContentType: contentTypeJPEG}
		summary, err := fixture.enricher.Enrich(ctx, testLibraryID, TaskPoster)
		if err != nil || summary.Applied != 2 || fixture.adapter.posterCalls != 2 {
			t.Fatalf("film posters: %+v %v", summary, err)
		}

		emptyArt := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
		summary, err = emptyArt.enricher.Enrich(ctx, testLibraryID, TaskPoster)
		if err != nil || summary.Unmatched != 1 {
			t.Fatalf("empty art: %+v %v", summary, err)
		}

		fetchErr := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
		fetchErr.adapter.artErr = errStoreBoom
		summary, err = fetchErr.enricher.Enrich(ctx, testLibraryID, TaskPoster)
		if err != nil || summary.Errors != 1 {
			t.Fatalf("fetch error: %+v %v", summary, err)
		}

		upsertErr := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
		upsertErr.adapter.art = Art{Bytes: []byte("jpg"), ContentType: contentTypeJPEG}
		upsertErr.artifacts.putErr = errStoreBoom
		summary, err = upsertErr.enricher.Enrich(ctx, testLibraryID, TaskPoster)
		if err != nil || summary.Errors != 1 {
			t.Fatalf("upsert error: %+v %v", summary, err)
		}

		listErr := newFixture(t, access.LibraryTypeFilm, settings, []string{enrichFilmAPath})
		listErr.adapter.art = Art{Bytes: []byte("jpg"), ContentType: contentTypeJPEG}
		listErr.artifacts.listErr = errStoreBoom
		summary, err = listErr.enricher.Enrich(ctx, testLibraryID, TaskPoster)
		if err != nil || summary.Errors != 1 || summary.Applied != 1 {
			t.Fatalf("list error: %+v %v", summary, err)
		}
	})
}

func TestEnricher_NoopsAndSetupFailures(t *testing.T) { //nolint:cyclop // setup failure branches
	t.Parallel()

	allure.Test(t, "unconfigured slots no-op; setup problems fail the run",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			empty := Settings{
				LibraryID:           testLibraryID,
				MetadataApplyMode:   ApplyModeFillMissing,
				MetadataWriteTarget: WriteTargetDB,
			}

			fixture := newFixture(t, access.LibraryTypeFilm, empty, []string{enrichFilmAPath})
			for _, kind := range []TaskKind{TaskMetadata, TaskPoster, TaskSubtitle} {
				summary, err := fixture.enricher.Enrich(ctx, testLibraryID, kind)
				if err != nil || summary.ProviderConfigured || summary.Applied != 0 {
					t.Fatalf("%s no-op: %+v %v", kind, summary, err)
				}
			}

			unknown := metadataSettings(ApplyModeFillMissing, WriteTargetDB, false)
			unknown.MetadataProvider = new("tmdb")
			unknown.PosterProvider = new("tmdb")
			missing := newFixture(t, access.LibraryTypeFilm, unknown, nil)
			for _, kind := range []TaskKind{TaskMetadata, TaskPoster} {
				_, err := missing.enricher.Enrich(ctx, testLibraryID, kind)
				if !errors.Is(err, ErrProviderNotRegistered) {
					t.Fatalf("%s: want ErrProviderNotRegistered, got %v", kind, err)
				}
			}

			photos := newFixture(t, access.LibraryTypePhotos, empty, nil)
			_, err := photos.enricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if !errors.Is(err, ErrUnsupportedLibraryType) {
				t.Fatalf("want ErrUnsupportedLibraryType, got %v", err)
			}

			var nilEnricher *Enricher
			_, err = nilEnricher.Enrich(ctx, testLibraryID, TaskMetadata)
			if !errors.Is(err, ErrEnricherUnavailable) {
				t.Fatalf("nil enricher: %v", err)
			}

			libErr := NewEnricher(EnricherDeps{
				Libraries: fakeLibraries{err: errStoreBoom},
				Settings:  fakeSettings{},
				Registry:  NewRegistry(),
			})
			_, enrichErr := libErr.Enrich(ctx, testLibraryID, TaskMetadata)
			if !errors.Is(enrichErr, errStoreBoom) {
				t.Fatalf("library error: %v", enrichErr)
			}

			settingsErr := NewEnricher(EnricherDeps{
				Libraries: fakeLibraries{library: access.Library{
					ID:   testLibraryID,
					Type: access.LibraryTypeFilm,
				}},
				Settings: fakeSettings{err: errStoreBoom},
				Registry: NewRegistry(),
			})
			_, enrichErr = settingsErr.Enrich(ctx, testLibraryID, TaskMetadata)
			if !errors.Is(enrichErr, errStoreBoom) {
				t.Fatalf("settings error: %v", enrichErr)
			}
		})
}

func TestEnricher_Subtitles_SkipsOnlyCoveredLanguages(t *testing.T) {
	t.Parallel()

	allure.Test(t, "subtitle provider skips local en but still fetches ru",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			key := "opensubtitles"
			settings := Settings{
				LibraryID:           testLibraryID,
				SubtitleProvider:    &key,
				SubtitleLanguages:   []string{"en", "ru"},
				MetadataApplyMode:   ApplyModeFillMissing,
				MetadataWriteTarget: WriteTargetDB,
			}

			adapter := newFakeAdapter(key)
			adapter.subFile = SubtitleFile{Lang: "ru", Bytes: []byte("WEBVTT\n"), ExternalID: new("9")}
			registry := NewRegistry()
			registry.RegisterSubtitle(adapter)

			cache, err := NewCache(filepath.Join(t.TempDir(), "providers"))
			if err != nil {
				t.Fatalf("cache: %v", err)
			}
			artifacts := newFakeArtifacts()
			langEN := "en"
			_, err = artifacts.UpsertArtifact(ctx, Artifact{
				LibraryID:   testLibraryID,
				RelPath:     enrichShowE01,
				Kind:        ArtifactKindSubtitle,
				Lang:        &langEN,
				ProviderKey: key,
				CachePath:   SubtitleCachePath(testLibraryID, enrichShowE01, langEN),
				FetchedAt:   time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("seed artifact: %v", err)
			}
			err = cache.Write(SubtitleCachePath(testLibraryID, enrichShowE01, langEN), []byte("old-en"))
			if err != nil {
				t.Fatalf("seed cache: %v", err)
			}

			enricher := NewEnricher(EnricherDeps{
				Libraries: fakeLibraries{library: access.Library{
					ID:   testLibraryID,
					Type: access.LibraryTypeSeries,
				}},
				Settings:   fakeSettings{settings: settings},
				Registry:   registry,
				Artifacts:  artifacts,
				Cache:      cache,
				ListVideos: func(_ []string) ([]string, error) { return []string{enrichShowE01}, nil },
				ResolvePath: func(relPath string) (string, error) {
					return "/media/" + relPath, nil
				},
				LocalSubtitleLanguages: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{"en": {}}, nil
				},
			})

			summary, err := enricher.Enrich(ctx, testLibraryID, TaskSubtitle)
			if err != nil {
				t.Fatalf("enrich: %v", err)
			}
			if adapter.subCalls != 1 {
				t.Fatalf("want one fetch (ru only), got %d calls", adapter.subCalls)
			}
			if summary.Applied != 1 {
				t.Fatalf("want applied=1 for ru, got %+v", summary)
			}
			if summary.Pruned != 1 {
				t.Fatalf("want pruned stale en provider artifact, got %+v", summary)
			}
		})
}

func TestEnricher_Subtitles_FetchesWhenNoLocal(t *testing.T) {
	t.Parallel()

	allure.Test(t, "subtitle provider fetches when no local language-tagged track",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			key := "opensubtitles"
			settings := Settings{
				LibraryID:           testLibraryID,
				SubtitleProvider:    &key,
				SubtitleLanguages:   []string{"en"},
				MetadataApplyMode:   ApplyModeFillMissing,
				MetadataWriteTarget: WriteTargetDB,
			}

			adapter := newFakeAdapter(key)
			adapter.subFile = SubtitleFile{Lang: "en", Bytes: []byte("WEBVTT\n"), ExternalID: new("9")}
			registry := NewRegistry()
			registry.RegisterSubtitle(adapter)

			cache, err := NewCache(filepath.Join(t.TempDir(), "providers"))
			if err != nil {
				t.Fatalf("cache: %v", err)
			}

			enricher := NewEnricher(EnricherDeps{
				Libraries: fakeLibraries{library: access.Library{
					ID:   testLibraryID,
					Type: access.LibraryTypeFilm,
				}},
				Settings:   fakeSettings{settings: settings},
				Registry:   registry,
				Artifacts:  newFakeArtifacts(),
				Cache:      cache,
				ListVideos: func(_ []string) ([]string, error) { return []string{enrichFilmPath}, nil },
				ResolvePath: func(relPath string) (string, error) {
					return "/media/" + relPath, nil
				},
				LocalSubtitleLanguages: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{}, nil
				},
			})

			summary, err := enricher.Enrich(ctx, testLibraryID, TaskSubtitle)
			if err != nil {
				t.Fatalf("enrich: %v", err)
			}
			if adapter.subCalls != 1 {
				t.Fatalf("want one fetch, got %d", adapter.subCalls)
			}
			if summary.Applied != 1 {
				t.Fatalf("want applied=1, got %+v", summary)
			}
		})
}

func TestRunSummary_Map(t *testing.T) {
	t.Parallel()

	allure.Test(t, "run summary renders the documented maintenance keys", func(a *allure.Context) {
		t := a.T()
		rendered := RunSummary{ProviderConfigured: true, Applied: 3, Pruned: 1}.Map()

		for key, want := range map[string]any{
			"providerConfigured": true,
			"applied":            3,
			"pruned":             1,
			"skippedUser":        0,
			"unmatched":          0,
			"skippedUncertain":   0,
			"errors":             0,
		} {
			if rendered[key] != want {
				t.Fatalf("%s: want %v, got %v", key, want, rendered[key])
			}
		}
	})
}
