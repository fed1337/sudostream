package maintenance

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sudoStream/internal/access"
	"sync"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type memoryStore struct {
	mu        sync.Mutex
	schedules []Schedule
	runs      []Run
}

func (m *memoryStore) ListSchedules(_ context.Context) ([]Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]Schedule(nil), m.schedules...)

	return out, nil
}

func (m *memoryStore) ListGlobalSchedules(ctx context.Context) ([]Schedule, error) {
	all, err := m.ListSchedules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Schedule, 0)
	for _, schedule := range all {
		if schedule.LibraryID == nil {
			out = append(out, schedule)
		}
	}

	return out, nil
}

func (m *memoryStore) ListSchedulesForLibrary(
	ctx context.Context,
	libraryID string,
) ([]Schedule, error) {
	all, err := m.ListSchedules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Schedule, 0)
	for _, schedule := range all {
		if schedule.LibraryID != nil && *schedule.LibraryID == libraryID {
			out = append(out, schedule)
		}
	}

	return out, nil
}

func (m *memoryStore) UpsertSchedule(
	_ context.Context,
	schedule Schedule,
) (Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, existing := range m.schedules {
		sameLib := (existing.LibraryID == nil && schedule.LibraryID == nil) ||
			(existing.LibraryID != nil && schedule.LibraryID != nil &&
				*existing.LibraryID == *schedule.LibraryID)
		if existing.Action == schedule.Action && sameLib {
			schedule.ID = existing.ID
			schedule.UpdatedAt = time.Now().UTC()
			m.schedules[i] = schedule

			return schedule, nil
		}
	}

	schedule.ID = "sched-" + schedule.Action
	schedule.UpdatedAt = time.Now().UTC()
	m.schedules = append(m.schedules, schedule)

	return schedule, nil
}

func (m *memoryStore) InsertRun(_ context.Context, run Run) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run.ID = fmt.Sprintf("run-%d", len(m.runs)+1)
	m.runs = append(m.runs, run)

	return run, nil
}

func (m *memoryStore) GetRun(_ context.Context, runID string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.runs {
		if run.ID == runID {
			return run, nil
		}
	}

	return Run{}, ErrNotFound
}

func (m *memoryStore) FinishRun(
	_ context.Context,
	runID, status string,
	summary map[string]any,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, run := range m.runs {
		if run.ID == runID {
			now := time.Now().UTC()
			m.runs[i].Status = status
			m.runs[i].FinishedAt = &now
			m.runs[i].Summary = summary

			return nil
		}
	}

	return ErrNotFound
}

func (m *memoryStore) LatestRun(
	_ context.Context,
	action string,
	libraryID *string,
) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range slices.Backward(m.runs) {
		run := v
		if run.Action != action {
			continue
		}
		sameLib := (run.LibraryID == nil && libraryID == nil) ||
			(run.LibraryID != nil && libraryID != nil && *run.LibraryID == *libraryID)
		if sameLib {
			return run, nil
		}
	}

	return Run{}, ErrNotFound
}

func (m *memoryStore) ListRuns(_ context.Context, limit int, status string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := make([]Run, 0, len(m.runs))
	for _, run := range m.runs {
		if status == "" || run.Status == status {
			filtered = append(filtered, run)
		}
	}
	if limit <= 0 || limit > len(filtered) {
		limit = len(filtered)
	}
	start := max(len(filtered)-limit, 0)
	out := append([]Run(nil), filtered[start:]...)

	return out, nil
}

func (m *memoryStore) MarkInterruptedRuns(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	now := time.Now().UTC()
	for i, run := range m.runs {
		if run.Status == StatusRunning {
			m.runs[i].Status = StatusFailed
			m.runs[i].FinishedAt = &now
			m.runs[i].Summary = map[string]any{"error": "interrupted by restart"}
			count++
		}
	}

	return count, nil
}

type blockingPurger struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingPurger) PurgeStaleCache(_ time.Duration) (int, int64, error) {
	close(p.started)
	<-p.release

	return 1, 10, nil
}

func (p *blockingPurger) CacheDirBytes() int64 { return 0 }

var errPurgeBoom = errors.New("purge boom")

type errPurger struct{}

func (errPurger) PurgeStaleCache(_ time.Duration) (int, int64, error) {
	return 0, 0, errPurgeBoom
}

func (errPurger) CacheDirBytes() int64 { return 0 }

func TestService_RunNowConflict(t *testing.T) {
	t.Parallel()

	allure.Test(t, "RunNow returns conflict while action is locked", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{}
		purger := &blockingPurger{
			started: make(chan struct{}),
			release: make(chan struct{}),
		}
		svc := NewService(store, Deps{Purger: purger})

		run, err := svc.RunNow(
			context.Background(),
			ActionPlaybackCachePurge,
			"",
			TriggerManual,
		)
		if err != nil {
			t.Fatalf("first run: %v", err)
		}
		if run.Status != StatusRunning {
			t.Fatalf("status=%s", run.Status)
		}

		<-purger.started
		_, err = svc.RunNow(
			context.Background(),
			ActionPlaybackCachePurge,
			"",
			TriggerManual,
		)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("want ErrConflict, got %v", err)
		}

		close(purger.release)
		deadline := time.After(2 * time.Second)
		for {
			select {
			case <-deadline:
				t.Fatal("run did not finish")
			default:
			}
			runs, listErr := svc.ListRuns(context.Background(), 10, "")
			if listErr != nil {
				t.Fatal(listErr)
			}
			if len(runs) > 0 && runs[len(runs)-1].Status != StatusRunning {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

//nolint:cyclop,funlen,gocognit // multi-assertion maintenance flow
func TestService_PatchAndStatus(
	t *testing.T,
) {
	t.Parallel()

	allure.Test(
		t,
		"patch schedules, status, start/shutdown, invalid cron",
		func(a *allure.Context) {
			t := a.T()
			store := &memoryStore{}
			svc := NewService(store, Deps{Purger: errPurger{}})

			err := svc.PatchGlobalSchedules(context.Background(), []ScheduleInput{{
				Action:  ActionLibrariesScan,
				Cron:    "not-cron",
				Enabled: true,
			}})
			if !errors.Is(err, ErrInvalidCron) {
				t.Fatalf("want invalid cron, got %v", err)
			}

			err = svc.PatchGlobalSchedules(context.Background(), []ScheduleInput{{
				Action:  ActionLibrariesScan,
				Cron:    "0 3 * * *",
				Enabled: true,
			}, {
				Action:  ActionPlaybackCachePurge,
				Cron:    "0 4 * * *",
				Enabled: false,
			}})
			if err != nil {
				t.Fatalf("patch global: %v", err)
			}

			libID := "lib-1"
			err = svc.PatchLibrarySchedules(context.Background(), libID, []ScheduleInput{{
				Action:  ActionMetadataScan,
				Cron:    "15 * * * *",
				Enabled: false,
			}})
			if err != nil {
				t.Fatalf("patch library: %v", err)
			}

			global, err := svc.GetGlobalMaintenance(context.Background())
			if err != nil {
				t.Fatalf("global status: %v", err)
			}
			if len(global) != 3 {
				t.Fatalf("global actions=%d", len(global))
			}

			libStatus, err := svc.GetLibraryMaintenance(context.Background(), libID)
			if err != nil {
				t.Fatalf("library status: %v", err)
			}
			if len(libStatus) != 6 {
				t.Fatalf("library actions=%d", len(libStatus))
			}

			store.runs = append(store.runs, Run{
				ID:        "running-1",
				Action:    ActionLibrariesScan,
				Status:    StatusRunning,
				StartedAt: time.Now().UTC(),
			})
			err = svc.Start(context.Background())
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			svc.Shutdown()
			svc.Shutdown()

			runs, err := svc.ListRuns(context.Background(), 5, "")
			if err != nil {
				t.Fatalf("list runs: %v", err)
			}
			foundFailed := false
			for _, run := range runs {
				if run.ID == "running-1" && run.Status == StatusFailed {
					foundFailed = true
				}
			}
			if !foundFailed {
				t.Fatal("expected interrupted run marked failed")
			}

			_, err = svc.RunNow(context.Background(), "nope", "", TriggerManual)
			if !errors.Is(err, ErrInvalidAction) {
				t.Fatalf("want invalid action, got %v", err)
			}
			_, err = svc.RunNow(context.Background(), ActionMetadataScan, "", TriggerManual)
			if !errors.Is(err, ErrLibraryRequired) {
				t.Fatalf("want library required, got %v", err)
			}

			_, err = svc.RunNow(
				context.Background(),
				ActionPlaybackCachePurge,
				"",
				TriggerManual,
			)
			if err != nil {
				t.Fatalf("purge run start: %v", err)
			}
			deadline := time.After(2 * time.Second)
			for {
				select {
				case <-deadline:
					t.Fatal("purge run did not finish")
				default:
				}
				latest, latestErr := store.LatestRun(
					context.Background(),
					ActionPlaybackCachePurge,
					nil,
				)
				if latestErr == nil && latest.Status == StatusFailed {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		},
	)
}

func TestService_FireScheduledSkip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "scheduled fire skips when locked", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{}
		svc := NewService(store, Deps{})
		if !svc.tryLock(LockKey(ActionLibrariesScan, ""), func() {}) {
			t.Fatal("lock")
		}
		svc.fireScheduled(Schedule{Action: ActionLibrariesScan, Cron: "0 * * * *"})
		svc.unlock(LockKey(ActionLibrariesScan, ""))
	})
}

type blockingSkipIntro struct {
	started chan struct{}
}

func (b *blockingSkipIntro) DetectLibraryMap(
	ctx context.Context,
	_ string,
) (map[string]any, error) {
	close(b.started)
	<-ctx.Done()

	return nil, ctx.Err()
}

type cancelAccessStub struct{}

func (cancelAccessStub) SyncLibraries(
	_ context.Context,
	_ string,
) (access.SyncLibrariesResult, error) {
	return access.SyncLibrariesResult{}, nil
}

func (cancelAccessStub) GetLibrary(_ context.Context, libraryID string) (access.Library, error) {
	return access.Library{ID: libraryID, Name: "Lib One"}, nil
}

func TestService_CancelLiveRun(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Cancel mid-run finishes as canceled and releases lock", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{}
		blocker := &blockingSkipIntro{started: make(chan struct{})}
		svc := NewService(store, Deps{
			Access:    cancelAccessStub{},
			SkipIntro: blocker,
		})

		run, err := svc.RunNow(
			context.Background(),
			ActionPlaybackSkipDetect,
			"lib-1",
			TriggerManual,
		)
		if err != nil {
			t.Fatalf("run: %v", err)
		}

		<-blocker.started
		err = svc.Cancel(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("cancel: %v", err)
		}

		deadline := time.After(2 * time.Second)
		for {
			select {
			case <-deadline:
				t.Fatal("run did not finish canceled")
			default:
			}
			got, getErr := store.GetRun(context.Background(), run.ID)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if got.Status == StatusCanceled {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if svc.isLocked(ActionPlaybackSkipDetect, "lib-1") {
			t.Fatal("lock still held after cancel")
		}
	})

	allure.Test(t, "ListRuns status filter and library name enrich", func(a *allure.Context) {
		t := a.T()
		libID := "lib-1"
		store := &memoryStore{runs: []Run{
			{
				ID:        "r1",
				Action:    ActionMetadataScan,
				LibraryID: &libID,
				Status:    StatusSuccess,
				StartedAt: time.Now().UTC(),
			},
			{
				ID:        "r2",
				Action:    ActionLibrariesScan,
				Status:    StatusRunning,
				StartedAt: time.Now().UTC(),
			},
		}}
		svc := NewService(store, Deps{Access: cancelAccessStub{}})

		_, err := svc.ListRuns(context.Background(), 10, "not-a-status")
		if !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("want invalid status, got %v", err)
		}

		running, err := svc.ListRuns(context.Background(), 10, StatusRunning)
		if err != nil || len(running) != 1 || running[0].ID != "r2" {
			t.Fatalf("running filter: %v %+v", err, running)
		}

		all, err := svc.ListRuns(context.Background(), 10, "")
		if err != nil {
			t.Fatal(err)
		}
		foundNamed := false
		for _, run := range all {
			if run.ID == "r1" && run.LibraryName == "Lib One" {
				foundNamed = true
			}
		}
		if !foundNamed {
			t.Fatalf("expected libraryName on lib run: %+v", all)
		}
	})

	allure.Test(t, "Cancel unknown / finished / orphaned running", func(a *allure.Context) {
		t := a.T()
		store := &memoryStore{}
		svc := NewService(store, Deps{})

		err := svc.Cancel(context.Background(), "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}

		finished, err := store.InsertRun(context.Background(), Run{
			Action:    ActionLibrariesScan,
			Trigger:   TriggerManual,
			Status:    StatusSuccess,
			StartedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		err = svc.Cancel(context.Background(), finished.ID)
		if !errors.Is(err, ErrNotRunning) {
			t.Fatalf("want ErrNotRunning, got %v", err)
		}

		orphan, err := store.InsertRun(context.Background(), Run{
			Action:    ActionLibrariesScan,
			Trigger:   TriggerManual,
			Status:    StatusRunning,
			StartedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		err = svc.Cancel(context.Background(), orphan.ID)
		if err != nil {
			t.Fatalf("orphan cancel: %v", err)
		}
		got, err := store.GetRun(context.Background(), orphan.ID)
		if err != nil || got.Status != StatusCanceled {
			t.Fatalf("orphan status=%s err=%v", got.Status, err)
		}
	})
}
