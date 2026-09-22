// Package postgres implements maintenance.Store with GORM/PostgreSQL.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sudoStream/internal/maintenance"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Store persists maintenance schedules and runs in PostgreSQL.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a maintenance postgres store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

type scheduleModel struct {
	ID        string         `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	Action    string         `gorm:"column:action"`
	LibraryID *string        `gorm:"column:library_id;type:uuid"`
	Cron      string         `gorm:"column:cron"`
	Enabled   bool           `gorm:"column:enabled"`
	Config    datatypes.JSON `gorm:"column:config"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
}

func (scheduleModel) TableName() string { return "maintenance_schedules" }

type runModel struct {
	ID         string         `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	Action     string         `gorm:"column:action"`
	LibraryID  *string        `gorm:"column:library_id;type:uuid"`
	Trigger    string         `gorm:"column:trigger"`
	Status     string         `gorm:"column:status"`
	StartedAt  time.Time      `gorm:"column:started_at"`
	FinishedAt *time.Time     `gorm:"column:finished_at"`
	Summary    datatypes.JSON `gorm:"column:summary"`
}

func (runModel) TableName() string { return "maintenance_runs" }

// ListSchedules returns all schedules.
func (s *Store) ListSchedules(ctx context.Context) ([]maintenance.Schedule, error) {
	var models []scheduleModel
	err := s.db.WithContext(ctx).Order("action ASC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list maintenance schedules: %w", err)
	}

	out := make([]maintenance.Schedule, 0, len(models))
	for _, model := range models {
		out = append(out, scheduleFromModel(model))
	}

	return out, nil
}

// ListGlobalSchedules returns schedules with null library_id.
func (s *Store) ListGlobalSchedules(ctx context.Context) ([]maintenance.Schedule, error) {
	var models []scheduleModel
	err := s.db.WithContext(ctx).
		Where("library_id IS NULL").
		Order("action ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list global maintenance schedules: %w", err)
	}

	out := make([]maintenance.Schedule, 0, len(models))
	for _, model := range models {
		out = append(out, scheduleFromModel(model))
	}

	return out, nil
}

// ListSchedulesForLibrary returns schedules for one library.
func (s *Store) ListSchedulesForLibrary(
	ctx context.Context,
	libraryID string,
) ([]maintenance.Schedule, error) {
	var models []scheduleModel
	err := s.db.WithContext(ctx).
		Where("library_id = ?", libraryID).
		Order("action ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list library maintenance schedules: %w", err)
	}

	out := make([]maintenance.Schedule, 0, len(models))
	for _, model := range models {
		out = append(out, scheduleFromModel(model))
	}

	return out, nil
}

// UpsertSchedule inserts or updates a schedule by (action, library_id).
func (s *Store) UpsertSchedule(
	ctx context.Context,
	schedule maintenance.Schedule,
) (maintenance.Schedule, error) {
	configRaw, err := json.Marshal(schedule.Config)
	if err != nil {
		return maintenance.Schedule{}, fmt.Errorf("marshal schedule config: %w", err)
	}
	if schedule.Config == nil {
		configRaw = []byte("{}")
	}

	now := time.Now().UTC()
	var existing scheduleModel
	query := s.db.WithContext(ctx).Where("action = ?", schedule.Action)
	if schedule.LibraryID == nil {
		query = query.Where("library_id IS NULL")
	} else {
		query = query.Where("library_id = ?", *schedule.LibraryID)
	}

	err = query.First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		model := scheduleModel{
			ID:        uuid.NewString(),
			Action:    schedule.Action,
			LibraryID: schedule.LibraryID,
			Cron:      schedule.Cron,
			Enabled:   schedule.Enabled,
			Config:    datatypes.JSON(configRaw),
			UpdatedAt: now,
		}
		err = s.db.WithContext(ctx).Create(&model).Error
		if err != nil {
			return maintenance.Schedule{}, fmt.Errorf("create schedule: %w", err)
		}

		return scheduleFromModel(model), nil
	}
	if err != nil {
		return maintenance.Schedule{}, fmt.Errorf("load schedule: %w", err)
	}

	existing.Cron = schedule.Cron
	existing.Enabled = schedule.Enabled
	existing.Config = datatypes.JSON(configRaw)
	existing.UpdatedAt = now
	err = s.db.WithContext(ctx).Save(&existing).Error
	if err != nil {
		return maintenance.Schedule{}, fmt.Errorf("update schedule: %w", err)
	}

	return scheduleFromModel(existing), nil
}

// InsertRun creates a running (or initial) run row.
func (s *Store) InsertRun(ctx context.Context, run maintenance.Run) (maintenance.Run, error) {
	summaryRaw, err := json.Marshal(run.Summary)
	if err != nil {
		return maintenance.Run{}, fmt.Errorf("marshal run summary: %w", err)
	}
	if run.Summary == nil {
		summaryRaw = []byte("{}")
	}

	model := runModel{
		ID:         uuid.NewString(),
		Action:     run.Action,
		LibraryID:  run.LibraryID,
		Trigger:    run.Trigger,
		Status:     run.Status,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
		Summary:    datatypes.JSON(summaryRaw),
	}
	if model.StartedAt.IsZero() {
		model.StartedAt = time.Now().UTC()
	}
	if model.Status == "" {
		model.Status = maintenance.StatusRunning
	}

	err = s.db.WithContext(ctx).Create(&model).Error
	if err != nil {
		return maintenance.Run{}, fmt.Errorf("insert run: %w", err)
	}

	return runFromModel(model), nil
}

// FinishRun updates status, finished_at, and summary.
func (s *Store) FinishRun(
	ctx context.Context,
	runID, status string,
	summary map[string]any,
) error {
	summaryRaw, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal run summary: %w", err)
	}
	if summary == nil {
		summaryRaw = []byte("{}")
	}

	now := time.Now().UTC()
	result := s.db.WithContext(ctx).
		Model(&runModel{}).
		Where("id = ?", runID).
		Updates(map[string]any{
			"status":      status,
			"finished_at": now,
			"summary":     datatypes.JSON(summaryRaw),
		})
	if result.Error != nil {
		return fmt.Errorf("finish run: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return maintenance.ErrNotFound
	}

	return nil
}

// LatestRun returns the most recent run for action (+ optional library).
func (s *Store) LatestRun(
	ctx context.Context,
	action string,
	libraryID *string,
) (maintenance.Run, error) {
	var model runModel
	query := s.db.WithContext(ctx).Where("action = ?", action)
	if libraryID == nil {
		query = query.Where("library_id IS NULL")
	} else {
		query = query.Where("library_id = ?", *libraryID)
	}

	err := query.Order("started_at DESC").First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return maintenance.Run{}, maintenance.ErrNotFound
	}
	if err != nil {
		return maintenance.Run{}, fmt.Errorf("latest run: %w", err)
	}

	return runFromModel(model), nil
}

// ListRuns returns recent runs newest-first.
func (s *Store) ListRuns(ctx context.Context, limit int) ([]maintenance.Run, error) {
	if limit <= 0 {
		limit = 50
	}

	var models []runModel
	err := s.db.WithContext(ctx).
		Order("started_at DESC").
		Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	out := make([]maintenance.Run, 0, len(models))
	for _, model := range models {
		out = append(out, runFromModel(model))
	}

	return out, nil
}

// MarkInterruptedRuns marks any running rows as failed after process restart.
func (s *Store) MarkInterruptedRuns(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	type interruptSummary struct {
		Error string `json:"error"`
	}
	summary, err := json.Marshal(interruptSummary{Error: "interrupted by restart"})
	if err != nil {
		return 0, fmt.Errorf("marshal interrupt summary: %w", err)
	}
	result := s.db.WithContext(ctx).Model(&runModel{}).
		Where("status = ?", maintenance.StatusRunning).
		Updates(map[string]any{
			"status":      maintenance.StatusFailed,
			"finished_at": now,
			"summary":     datatypes.JSON(summary),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("mark interrupted runs: %w", result.Error)
	}

	return result.RowsAffected, nil
}

func scheduleFromModel(model scheduleModel) maintenance.Schedule {
	config := map[string]any{}
	if len(model.Config) > 0 {
		_ = json.Unmarshal(model.Config, &config)
	}

	return maintenance.Schedule{
		ID:        model.ID,
		Action:    model.Action,
		LibraryID: model.LibraryID,
		Cron:      model.Cron,
		Enabled:   model.Enabled,
		Config:    config,
		UpdatedAt: model.UpdatedAt,
	}
}

func runFromModel(model runModel) maintenance.Run {
	summary := map[string]any{}
	if len(model.Summary) > 0 {
		_ = json.Unmarshal(model.Summary, &summary)
	}

	return maintenance.Run{
		ID:         model.ID,
		Action:     model.Action,
		LibraryID:  model.LibraryID,
		Trigger:    model.Trigger,
		Status:     model.Status,
		StartedAt:  model.StartedAt,
		FinishedAt: model.FinishedAt,
		Summary:    summary,
	}
}
