package skipsegment

import (
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestDetectLibrary_WritesAudioConsensus(t *testing.T) {
	t.Parallel()

	allure.Test(t, "detect library writes consensus audio intros", func(a *allure.Context) {
		t := a.T()

		root := t.TempDir()
		for _, name := range []string{"a.mkv", "b.mkv", "c.mkv"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatal(err)
		}

		rng := rand.New(rand.NewPCG(9, 10))
		theme := randomFingerprint(rng, 120)
		fps := map[string][]uint32{
			"a.mkv": append(randomFingerprint(rng, 20), theme...),
			"b.mkv": append(randomFingerprint(rng, 30), theme...),
			"c.mkv": append(randomFingerprint(rng, 25), theme...),
		}

		store := &memorySegmentStore{rows: map[string]StoredSegment{}}
		catalog := &memoryCatalog{
			shows: []metadata.CatalogShowRow{{ShowKey: "demo", Name: "Demo", SeasonCount: 1}},
			agg: metadata.CatalogShowAgg{
				ShowKey: "demo",
				Name:    "Demo",
				Seasons: []metadata.CatalogSeasonCount{{Season: 1, EpisodeCount: 3}},
			},
			episodes: []metadata.CatalogEpisodeRow{
				{RelPath: "a.mkv"},
				{RelPath: "b.mkv"},
				{RelPath: "c.mkv"},
			},
		}
		libs := &memoryLibraries{lib: access.Library{
			ID: "lib-1", Type: access.LibraryTypeSeries, Slug: "series",
		}}

		svc := NewService(store, media, libs, catalog)
		svc.SetProbeSource(func(context.Context, string) (transcode.SourceInfo, error) {
			return transcode.SourceInfo{DurationSeconds: 600}, nil
		})
		svc.SetExtract(func(_ context.Context, absPath string, _ float64) ([]uint32, float64, error) {
			base := filepath.Base(absPath)

			return fps[base], 600, nil
		})

		summary, err := svc.DetectLibrary(context.Background(), "lib-1")
		if err != nil {
			t.Fatalf("detect: %v", err)
		}
		if summary.AudioHits != 3 {
			t.Fatalf("audioHits=%d summary=%+v store=%d", summary.AudioHits, summary, len(store.rows))
		}
		if got := svc.LookupIntro(context.Background(), "lib-1", "a.mkv"); got == nil || got.Source != SourceAudio {
			t.Fatalf("lookup: %+v", got)
		}
		mapped, err := svc.DetectLibraryMap(context.Background(), "lib-1")
		if err != nil || mapped["audioHits"].(int) != 3 {
			t.Fatalf("map: %+v err=%v", mapped, err)
		}
	})
}

func TestDetectLibrary_SkipsNonSeries(t *testing.T) {
	t.Parallel()

	allure.Test(t, "detect library skips non-series libraries", func(a *allure.Context) {
		t := a.T()

		root := t.TempDir()
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatal(err)
		}
		svc := NewService(
			&memorySegmentStore{rows: map[string]StoredSegment{}},
			media,
			&memoryLibraries{lib: access.Library{ID: "film", Type: access.LibraryTypeFilm}},
			&memoryCatalog{},
		)
		summary, err := svc.DetectLibrary(context.Background(), "film")
		if err != nil {
			t.Fatal(err)
		}
		if summary.Skipped != 1 || summary.AudioHits != 0 {
			t.Fatalf("%+v", summary)
		}
	})
}

func TestDetectLibrary_SkipsChapterHits(t *testing.T) {
	t.Parallel()

	allure.Test(t, "detect library skips episodes with chapter intros", func(a *allure.Context) {
		t := a.T()

		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "a.mkv"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "b.mkv"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		media, err := mediafs.New(root)
		if err != nil {
			t.Fatal(err)
		}
		store := &memorySegmentStore{rows: map[string]StoredSegment{}}
		svc := NewService(
			store,
			media,
			&memoryLibraries{lib: access.Library{ID: "lib-1", Type: access.LibraryTypeSeries}},
			&memoryCatalog{
				shows: []metadata.CatalogShowRow{{ShowKey: "demo"}},
				agg: metadata.CatalogShowAgg{
					ShowKey: "demo",
					Seasons: []metadata.CatalogSeasonCount{{Season: 1, EpisodeCount: 2}},
				},
				episodes: []metadata.CatalogEpisodeRow{{RelPath: "a.mkv"}, {RelPath: "b.mkv"}},
			},
		)
		svc.SetProbeSource(func(context.Context, string) (transcode.SourceInfo, error) {
			return transcode.SourceInfo{
				DurationSeconds: 600,
				Chapters: []transcode.ChapterInfo{
					{StartSeconds: 0, EndSeconds: 10, Title: "Cold open"},
					{StartSeconds: 10, EndSeconds: 70, Title: "Opening Theme"},
				},
			}, nil
		})
		summary, err := svc.DetectLibrary(context.Background(), "lib-1")
		if err != nil {
			t.Fatal(err)
		}
		if summary.ChapterSkip != 2 || summary.AudioHits != 0 {
			t.Fatalf("%+v", summary)
		}
	})
}

func TestStoredSegment_ToIntro(t *testing.T) {
	t.Parallel()

	allure.Test(t, "stored segment to intro rejects inverted range", func(a *allure.Context) {
		t := a.T()

		if (&StoredSegment{StartMs: 10, EndMs: 5}).ToIntro() != nil {
			t.Fatal("expected nil")
		}
		got := StoredSegment{StartMs: 1, EndMs: 2, Source: SourceAudio}.ToIntro()
		if got == nil || got.Source != SourceAudio {
			t.Fatalf("%+v", got)
		}
	})
}

func TestExtractFingerprint_NoAudioOrMissingBinary(t *testing.T) {
	t.Parallel()

	allure.Test(t, "extract fingerprint handles missing file", func(a *allure.Context) {
		t := a.T()

		_, _, err := ExtractFingerprint(context.Background(), "/no/such/file.mkv", 120)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	allure.Test(t, "extract fingerprint rejects short scan window", func(a *allure.Context) {
		t := a.T()

		_, _, err := ExtractFingerprint(context.Background(), "/tmp/x", 5)
		if err == nil {
			t.Fatal("expected short window error")
		}
	})
}

func TestTruncateErrAndFormatSeconds(t *testing.T) {
	t.Parallel()

	allure.Test(t, "truncateErr and formatSeconds helpers", func(a *allure.Context) {
		t := a.T()

		if formatSeconds(1.5) != "1.500" {
			t.Fatal(formatSeconds(1.5))
		}
		long := make([]byte, 300)
		for i := range long {
			long[i] = 'a'
		}
		if len(truncateErr(string(long))) != maxFFmpegErrLogChars {
			t.Fatal(len(truncateErr(string(long))))
		}
	})
}

type memorySegmentStore struct {
	rows map[string]StoredSegment
}

func (m *memorySegmentStore) Upsert(_ context.Context, segment StoredSegment) error {
	m.rows[segment.LibraryID+"|"+segment.RelPath] = segment

	return nil
}

func (m *memorySegmentStore) Get(_ context.Context, libraryID, relPath string) (StoredSegment, bool, error) {
	segment, ok := m.rows[libraryID+"|"+relPath]

	return segment, ok, nil
}

func (m *memorySegmentStore) DeleteForPath(_ context.Context, libraryID, relPath string) error {
	delete(m.rows, libraryID+"|"+relPath)

	return nil
}

type memoryLibraries struct {
	lib access.Library
}

func (m *memoryLibraries) GetLibrary(_ context.Context, _ string) (access.Library, error) {
	return m.lib, nil
}

type memoryCatalog struct {
	shows    []metadata.CatalogShowRow
	agg      metadata.CatalogShowAgg
	episodes []metadata.CatalogEpisodeRow
}

func (m *memoryCatalog) ListCatalogShowsPage(
	context.Context, string, int, int,
) ([]metadata.CatalogShowRow, int, error) {
	return m.shows, len(m.shows), nil
}

func (m *memoryCatalog) GetCatalogShowAgg(
	context.Context, string, string,
) (metadata.CatalogShowAgg, bool, error) {
	if m.agg.ShowKey == "" {
		return metadata.CatalogShowAgg{}, false, nil
	}

	return m.agg, true, nil
}

func (m *memoryCatalog) ListCatalogSeasonEpisodes(
	context.Context, string, string, int, int, int,
) ([]metadata.CatalogEpisodeRow, int, error) {
	return m.episodes, len(m.episodes), nil
}
