package maintenance

import "context"

// Store persists schedules and runs.
type Store interface {
	ListSchedules(ctx context.Context) ([]Schedule, error)
	ListGlobalSchedules(ctx context.Context) ([]Schedule, error)
	ListSchedulesForLibrary(ctx context.Context, libraryID string) ([]Schedule, error)
	UpsertSchedule(ctx context.Context, schedule Schedule) (Schedule, error)
	InsertRun(ctx context.Context, run Run) (Run, error)
	FinishRun(ctx context.Context, runID, status string, summary map[string]any) error
	LatestRun(ctx context.Context, action string, libraryID *string) (Run, error)
	ListRuns(ctx context.Context, limit int) ([]Run, error)
	MarkInterruptedRuns(ctx context.Context) (int64, error)
}
