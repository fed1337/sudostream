package maintenance

import (
	"context"
	"errors"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/provider"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

var (
	errSyncBoom     = errors.New("sync boom")
	errIndexBoom    = errors.New("index boom")
	errWarmBoom     = errors.New("warm boom")
	errGetLib       = errors.New("library missing")
	errSettingsBoom = errors.New("settings boom")
)

type stubLibraryAccess struct {
	syncResult access.SyncLibrariesResult
	syncErr    error
	library    access.Library
	getErr     error
}

func (s stubLibraryAccess) SyncLibraries(
	_ context.Context,
	_ string,
) (access.SyncLibrariesResult, error) {
	return s.syncResult, s.syncErr
}

func (s stubLibraryAccess) GetLibrary(_ context.Context, _ string) (access.Library, error) {
	if s.getErr != nil {
		return access.Library{}, s.getErr
	}

	return s.library, nil
}

type stubIndexer struct {
	files int
	err   error
}

func (s stubIndexer) IndexLibrary(_ context.Context, _ access.Library) (int, error) {
	return s.files, s.err
}

type stubThumbs struct {
	generated int
	err       error
}

func (s stubThumbs) WarmLibrary(
	_ context.Context,
	_ *mediafs.Service,
	_ string,
) (int, error) {
	return s.generated, s.err
}

type okPurger struct{}

func (okPurger) PurgeStaleCache(_ time.Duration) (int, int64, error) {
	return 2, 100, nil
}

func (okPurger) CacheDirBytes() int64 { return 42 }

type stubTrash struct {
	purged int
	err    error
}

func (s stubTrash) PurgeExpired(_ context.Context) (int, error) {
	return s.purged, s.err
}

type stubProviderTasks struct {
	summary provider.RunSummary
	err     error
}

func (s stubProviderTasks) Enrich(
	_ context.Context,
	_ string,
	_ provider.TaskKind,
) (provider.RunSummary, error) {
	return s.summary, s.err
}

func testLibrary() access.Library {
	return access.Library{
		ID:      "lib-1",
		Slug:    "movies",
		RelPath: "movies",
		Name:    "Movies",
		Type:    access.LibraryTypeFilm,
	}
}

func TestRunAction_CoversRunners(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"runAction executes scan/index/warm/purge success paths",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			store := &memoryStore{}
			mediaRoot := t.TempDir()
			media, err := mediafs.New(mediaRoot)
			if err != nil {
				t.Fatalf("mediafs: %v", err)
			}

			lib := testLibrary()
			svc := NewService(store, Deps{
				Access: stubLibraryAccess{
					syncResult: access.SyncLibrariesResult{
						Libraries: []access.Library{lib},
						Added:     1,
					},
					library: lib,
				},
				Media:     media,
				MediaRoot: mediaRoot,
				Indexer:   stubIndexer{files: 4},
				Thumbs:    stubThumbs{generated: 3},
				Purger:    okPurger{},
				Trash:     stubTrash{purged: 5},
			})

			assertRunSummary(ctx, t, svc, ActionLibrariesScan, "", "added", 1)
			assertRunSummary(ctx, t, svc, ActionMetadataScan, lib.ID, "filesIndexed", 4)
			assertRunSummary(ctx, t, svc, ActionThumbnailsWarm, lib.ID, "generated", 3)

			_, err = store.UpsertSchedule(ctx, Schedule{
				Action: ActionPlaybackCachePurge,
				Config: map[string]any{
					ConfigKeyMaxAgeHours: float64(12),
				}, // ignored; retention is fixed
			})
			if err != nil {
				t.Fatalf("upsert purge schedule: %v", err)
			}
			summary, err := svc.runAction(ctx, ActionPlaybackCachePurge, "")
			if err != nil || summary["deleted"] != 2 ||
				summary[ConfigKeyMaxAgeHours] != PurgeRetentionHours {
				t.Fatalf("purge: summary=%v err=%v", summary, err)
			}

			assertRunSummary(ctx, t, svc, ActionTrashPurge, "", "purged", 5)

			providerSvc := NewService(store, Deps{
				Access: stubLibraryAccess{library: lib},
				Providers: stubProviderTasks{summary: provider.RunSummary{
					ProviderConfigured: true,
					Applied:            7,
				}},
			})
			for _, action := range []string{
				ActionProvidersMetadata,
				ActionProvidersPosters,
				ActionProvidersSubtitles,
			} {
				assertRunSummary(ctx, t, providerSvc, action, lib.ID, "providerConfigured", true)
				assertRunSummary(ctx, t, providerSvc, action, lib.ID, "applied", 7)
			}

			_, err = svc.runAction(ctx, "unknown", "")
			if !errors.Is(err, ErrInvalidAction) {
				t.Fatalf("want ErrInvalidAction, got %v", err)
			}
		},
	)
}

func assertRunSummary(
	ctx context.Context,
	t *testing.T,
	svc *Service,
	action, libraryID, key string,
	want any,
) {
	t.Helper()

	summary, err := svc.runAction(ctx, action, libraryID)
	if err != nil || summary[key] != want {
		t.Fatalf("%s: summary=%v err=%v", action, summary, err)
	}
}

//nolint:cyclop // sequential unavailable/failure branch coverage
func TestRunAction_UnavailableAndFailurePaths(t *testing.T) {
	t.Parallel()

	allure.Test(t, "runner reports unavailable deps and wrapped failures", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		store := &memoryStore{}
		empty := NewService(store, Deps{})

		summary, err := empty.runLibrariesScan(ctx)
		assertUnavailable(t, summary, err, errAccessUnavailable)
		summary, err = empty.runMetadataScan(ctx, "lib")
		assertUnavailable(t, summary, err, errIndexerUnavailable)
		summary, err = empty.runThumbnailsWarm(ctx, "lib")
		assertUnavailable(t, summary, err, errThumbsUnavailable)
		summary, err = empty.runPlaybackCachePurge(ctx)
		assertUnavailable(t, summary, err, errPurgerUnavailable)
		summary, err = empty.runProviderTask(ctx, "lib", provider.TaskMetadata)
		assertUnavailable(t, summary, err, errProvidersUnavailable)

		summary, err = NewService(store, Deps{
			Providers: stubProviderTasks{
				summary: provider.RunSummary{ProviderConfigured: true, Errors: 1},
				err:     errSettingsBoom,
			},
		}).runProviderTask(ctx, "lib-1", provider.TaskMetadata)
		if !errors.Is(err, errSettingsBoom) {
			t.Fatalf("want settings boom, got %v", err)
		}
		if summary["errors"] != 1 {
			t.Fatalf("want partial summary preserved on failure, got %v", summary)
		}

		_, err = NewService(store, Deps{
			Access: stubLibraryAccess{syncErr: errSyncBoom},
		}).runLibrariesScan(ctx)
		if !errors.Is(err, errSyncBoom) {
			t.Fatalf("want sync boom, got %v", err)
		}

		_, err = NewService(store, Deps{
			Access:  stubLibraryAccess{getErr: errGetLib},
			Indexer: stubIndexer{},
		}).runMetadataScan(ctx, "missing")
		if !errors.Is(err, errGetLib) {
			t.Fatalf("want get library error, got %v", err)
		}

		summary, err = NewService(store, Deps{
			Access:  stubLibraryAccess{library: access.Library{ID: "1", RelPath: "x"}},
			Indexer: stubIndexer{files: 1, err: errIndexBoom},
		}).runMetadataScan(ctx, "1")
		if !errors.Is(err, errIndexBoom) || summary["filesIndexed"] != 1 {
			t.Fatalf("index fail: summary=%v err=%v", summary, err)
		}

		media, err := mediafs.New(t.TempDir())
		if err != nil {
			t.Fatalf("mediafs: %v", err)
		}
		summary, err = NewService(store, Deps{
			Access: stubLibraryAccess{library: access.Library{ID: "1", RelPath: "x"}},
			Media:  media,
			Thumbs: stubThumbs{generated: 2, err: errWarmBoom},
		}).runThumbnailsWarm(ctx, "1")
		if !errors.Is(err, errWarmBoom) || summary["generated"] != 2 {
			t.Fatalf("warm fail: summary=%v err=%v", summary, err)
		}

		_, err = NewService(store, Deps{Purger: errPurger{}}).runPlaybackCachePurge(ctx)
		if !errors.Is(err, errPurgeBoom) {
			t.Fatalf("want purge boom, got %v", err)
		}
	})
}

func assertUnavailable(t *testing.T, summary map[string]any, err, want error) {
	t.Helper()

	if summary != nil || !errors.Is(err, want) {
		t.Fatalf("want %v with nil summary, got summary=%v err=%v", want, summary, err)
	}
}
