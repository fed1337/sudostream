package maintenance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
	"sudoStream/internal/provider"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// LibraryIndexer indexes metadata for one library.
type LibraryIndexer interface {
	IndexLibrary(ctx context.Context, library access.Library) (int, error)
}

// PosterWarmer warms video posters for one library path.
type PosterWarmer interface {
	WarmLibrary(ctx context.Context, media *mediafs.Service, libraryRelPath string) (int, error)
}

// CachePurger deletes stale HLS cache directories and reports cache size.
type CachePurger interface {
	PurgeStaleCache(maxAge time.Duration) (int, int64, error)
	CacheDirBytes() int64
}

// LibraryAccess syncs and loads libraries for maintenance actions.
type LibraryAccess interface {
	SyncLibraries(ctx context.Context, mediaRoot string) (access.SyncLibrariesResult, error)
	GetLibrary(ctx context.Context, libraryID string) (access.Library, error)
}

// ProviderTasks runs the three FI-1 providers.* actions for one library.
type ProviderTasks interface {
	Enrich(
		ctx context.Context,
		libraryID string,
		kind provider.TaskKind,
	) (provider.RunSummary, error)
}

// Deps wires domain services into the maintenance runner.
type Deps struct {
	Access    LibraryAccess
	Media     *mediafs.Service
	MediaRoot string
	Indexer   LibraryIndexer
	Thumbs    PosterWarmer
	Purger    CachePurger
	Trash     TrashPurger
	Providers ProviderTasks
}

// TrashPurger permanently deletes expired recycle-bin items.
type TrashPurger interface {
	PurgeExpired(ctx context.Context) (int, error)
}

// Service owns schedules, cron, and run execution.
type Service struct {
	store Store
	deps  Deps

	mu       sync.Mutex
	locks    map[string]struct{}
	cron     *cron.Cron
	entryIDs []cron.EntryID
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewService constructs a maintenance service (call Start after wiring).
func NewService(store Store, deps Deps) *Service {
	return &Service{
		store:  store,
		deps:   deps,
		locks:  make(map[string]struct{}),
		stopCh: make(chan struct{}),
	}
}

// SetMediaDeps sets thumbnail/media/purge deps created during HTTP wiring.
func (s *Service) SetMediaDeps(media *mediafs.Service, thumbs PosterWarmer, purger CachePurger) {
	if s == nil {
		return
	}
	s.deps.Media = media
	s.deps.Thumbs = thumbs
	s.deps.Purger = purger
}

// Start marks interrupted runs and loads cron schedules.
func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}

	n, err := s.store.MarkInterruptedRuns(ctx)
	if err != nil {
		return fmt.Errorf("mark interrupted runs: %w", err)
	}
	if n > 0 {
		slog.Info("marked interrupted maintenance runs",
			slog.String("action", "maintenance.start"),
			slog.Int64("count", n),
		)
	}

	return s.ReloadSchedules(ctx)
}

// Shutdown stops the cron scheduler.
func (s *Service) Shutdown() {
	if s == nil {
		return
	}

	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.mu.Lock()
		cronEngine := s.cron
		s.cron = nil
		s.entryIDs = nil
		s.mu.Unlock()
		if cronEngine != nil {
			stopCtx := cronEngine.Stop()
			<-stopCtx.Done()
		}
	})
}

// ReloadSchedules rebuilds cron entries from the database.
func (s *Service) ReloadSchedules(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}

	schedules, err := s.store.ListSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list schedules: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	//nolint:contextcheck // cron callbacks intentionally use Background (no request scope)
	s.replaceCronLocked(schedules)

	return nil
}

// RunNow starts an action if not already running.
func (s *Service) RunNow(
	ctx context.Context,
	action, libraryID, trigger string,
) (Run, error) {
	err := s.validateRunNow(action, &libraryID, &trigger)
	if err != nil {
		return Run{}, err
	}

	key := LockKey(action, libraryID)
	if !s.tryLock(key) {
		return Run{}, ErrConflict
	}

	run, err := s.insertRunning(ctx, action, libraryID, trigger)
	if err != nil {
		s.unlock(key)

		return Run{}, err
	}

	observability.SetMaintenanceRunning(action, true)
	slog.Info("maintenance run started",
		slog.String("action", action),
		slog.String("library_id", libraryID),
		slog.String("trigger", trigger),
		slog.String("run_id", run.ID),
	)

	// Detach from request cancel so long jobs survive client disconnect.
	//nolint:contextcheck,gosec // G118: Background in execute is intentional for long jobs
	go s.execute(run, action, libraryID, trigger, key)

	return run, nil
}

// CacheDirBytes returns total HLS cache bytes when a purger is wired.
func (s *Service) CacheDirBytes() int64 {
	if s == nil || s.deps.Purger == nil {
		return 0
	}

	return s.deps.Purger.CacheDirBytes()
}

// GetGlobalMaintenance returns status for global actions.
func (s *Service) GetGlobalMaintenance(ctx context.Context) ([]ActionStatus, error) {
	return s.actionStatuses(
		ctx,
		[]string{ActionLibrariesScan, ActionPlaybackCachePurge, ActionTrashPurge},
		nil,
	)
}

// GetLibraryMaintenance returns status for per-library actions.
func (s *Service) GetLibraryMaintenance(
	ctx context.Context,
	libraryID string,
) ([]ActionStatus, error) {
	return s.actionStatuses(ctx, []string{
		ActionMetadataScan, ActionThumbnailsWarm,
		ActionProvidersMetadata, ActionProvidersPosters, ActionProvidersSubtitles,
	}, &libraryID)
}

// PatchGlobalSchedules upserts global schedules and reloads cron.
func (s *Service) PatchGlobalSchedules(ctx context.Context, inputs []ScheduleInput) error {
	for _, input := range inputs {
		if !IsGlobalAction(input.Action) {
			return ErrInvalidAction
		}
		err := ValidateCron(input.Cron)
		if err != nil {
			return err
		}
		_, err = s.store.UpsertSchedule(ctx, Schedule{
			Action:  input.Action,
			Cron:    input.Cron,
			Enabled: input.Enabled,
			Config:  input.Config,
		})
		if err != nil {
			return fmt.Errorf("upsert global schedule: %w", err)
		}
	}

	return s.ReloadSchedules(ctx)
}

// PatchLibrarySchedules upserts per-library schedules and reloads cron.
func (s *Service) PatchLibrarySchedules(
	ctx context.Context,
	libraryID string,
	inputs []ScheduleInput,
) error {
	if libraryID == "" {
		return ErrLibraryRequired
	}

	libID := libraryID
	for _, input := range inputs {
		if !IsLibraryAction(input.Action) {
			return ErrInvalidAction
		}
		err := ValidateCron(input.Cron)
		if err != nil {
			return err
		}
		_, err = s.store.UpsertSchedule(ctx, Schedule{
			Action:    input.Action,
			LibraryID: &libID,
			Cron:      input.Cron,
			Enabled:   input.Enabled,
			Config:    input.Config,
		})
		if err != nil {
			return fmt.Errorf("upsert library schedule: %w", err)
		}
	}

	return s.ReloadSchedules(ctx)
}

// ListRuns returns recent runs.
func (s *Service) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	runs, err := s.store.ListRuns(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	return runs, nil
}

func (s *Service) validateRunNow(action string, libraryID, trigger *string) error {
	if s == nil || s.store == nil {
		return ErrInvalidAction
	}
	if !ValidAction(action) {
		return ErrInvalidAction
	}
	if IsLibraryAction(action) && *libraryID == "" {
		return ErrLibraryRequired
	}
	if IsGlobalAction(action) {
		*libraryID = ""
	}
	if *trigger == "" {
		*trigger = TriggerManual
	}

	return nil
}

func (s *Service) insertRunning(
	ctx context.Context,
	action, libraryID, trigger string,
) (Run, error) {
	var libPtr *string
	if libraryID != "" {
		libPtr = &libraryID
	}

	run, err := s.store.InsertRun(ctx, Run{
		Action:    action,
		LibraryID: libPtr,
		Trigger:   trigger,
		Status:    StatusRunning,
		StartedAt: time.Now().UTC(),
		Summary:   map[string]any{},
	})
	if err != nil {
		return Run{}, fmt.Errorf("insert run: %w", err)
	}

	return run, nil
}

func (s *Service) replaceCronLocked(schedules []Schedule) {
	if s.cron != nil {
		stopCtx := s.cron.Stop()
		<-stopCtx.Done()
	}

	//nolint:gosmopolitan // FI-6: schedules use server local timezone by design
	s.cron = cron.New(
		cron.WithParser(cronParser),
		cron.WithLocation(time.Local),
	)
	s.entryIDs = nil

	for _, schedule := range schedules {
		s.addCronEntryLocked(schedule)
	}

	s.cron.Start()
}

func (s *Service) addCronEntryLocked(schedule Schedule) {
	if !schedule.Enabled || strings.TrimSpace(schedule.Cron) == "" {
		return
	}

	sched := schedule
	entryID, addErr := s.cron.AddFunc(sched.Cron, func() {
		s.fireScheduled(sched)
	})
	if addErr != nil {
		slog.Warn("skip invalid maintenance schedule",
			slog.String("action", sched.Action),
			slog.String("cron", sched.Cron),
			slog.String("error", addErr.Error()),
		)

		return
	}
	s.entryIDs = append(s.entryIDs, entryID)
}

func (s *Service) fireScheduled(sched Schedule) {
	libraryID := ""
	if sched.LibraryID != nil {
		libraryID = *sched.LibraryID
	}
	// Cron ticks have no request context; Background is intentional.
	_, runErr := s.RunNow(
		context.Background(),
		sched.Action,
		libraryID,
		TriggerSchedule,
	)
	if errors.Is(runErr, ErrConflict) {
		slog.Info("scheduled maintenance skipped; already running",
			slog.String("action", sched.Action),
			slog.String("library_id", libraryID),
		)

		return
	}
	if runErr != nil {
		slog.Error("scheduled maintenance failed to start",
			slog.String("action", sched.Action),
			slog.String("library_id", libraryID),
			slog.String("error", runErr.Error()),
		)
	}
}

func (s *Service) execute(run Run, action, libraryID, trigger, key string) {
	defer s.unlock(key)
	defer observability.SetMaintenanceRunning(action, false)

	started := time.Now()
	summary, err := s.runAction(context.Background(), action, libraryID)
	status := StatusSuccess
	if err != nil {
		status = StatusFailed
		if summary == nil {
			summary = map[string]any{}
		}
		summary["error"] = err.Error()
	}

	finishErr := s.store.FinishRun(context.Background(), run.ID, status, summary)
	if finishErr != nil {
		slog.Error("finish maintenance run failed",
			slog.String("run_id", run.ID),
			slog.String("error", finishErr.Error()),
		)
	}

	duration := time.Since(started)
	observability.RecordMaintenanceRun(action, status, trigger, duration)
	slog.Info("maintenance run finished",
		slog.String("action", action),
		slog.String("library_id", libraryID),
		slog.String("trigger", trigger),
		slog.String("status", status),
		slog.Duration("duration", duration),
		slog.Any("summary", summary),
	)
}

func (s *Service) tryLock(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.locks[key]; ok {
		return false
	}
	s.locks[key] = struct{}{}

	return true
}

func (s *Service) unlock(key string) {
	s.mu.Lock()
	delete(s.locks, key)
	s.mu.Unlock()
}

func (s *Service) isLocked(action, libraryID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.locks[LockKey(action, libraryID)]

	return ok
}

func (s *Service) actionStatuses(
	ctx context.Context,
	actions []string,
	libraryID *string,
) ([]ActionStatus, error) {
	schedules, err := s.loadSchedules(ctx, libraryID)
	if err != nil {
		return nil, err
	}

	byAction := make(map[string]Schedule, len(schedules))
	for _, schedule := range schedules {
		byAction[schedule.Action] = schedule
	}

	out := make([]ActionStatus, 0, len(actions))
	for _, action := range actions {
		status, statusErr := s.buildActionStatus(ctx, action, libraryID, byAction)
		if statusErr != nil {
			return nil, statusErr
		}
		out = append(out, status)
	}

	return out, nil
}

func (s *Service) loadSchedules(
	ctx context.Context,
	libraryID *string,
) ([]Schedule, error) {
	var (
		schedules []Schedule
		err       error
	)
	if libraryID == nil {
		schedules, err = s.store.ListGlobalSchedules(ctx)
	} else {
		schedules, err = s.store.ListSchedulesForLibrary(ctx, *libraryID)
	}
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}

	return schedules, nil
}

func (s *Service) buildActionStatus(
	ctx context.Context,
	action string,
	libraryID *string,
	byAction map[string]Schedule,
) (ActionStatus, error) {
	status := ActionStatus{Action: action}
	if schedule, ok := byAction[action]; ok {
		copied := schedule
		status.Schedule = &copied
	}

	run, runErr := s.store.LatestRun(ctx, action, libraryID)
	if runErr == nil {
		status.LatestRun = &run
	} else if !errors.Is(runErr, ErrNotFound) {
		return ActionStatus{}, fmt.Errorf("latest run: %w", runErr)
	}

	lib := ""
	if libraryID != nil {
		lib = *libraryID
	}
	status.Running = s.isLocked(action, lib)

	return status, nil
}
