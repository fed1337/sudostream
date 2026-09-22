package maintenance

import (
	"errors"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Action identifiers for maintenance jobs.
const (
	ActionLibrariesScan      = "libraries.scan"
	ActionMetadataScan       = "metadata.scan"
	ActionThumbnailsWarm     = "thumbnails.warm"
	ActionPlaybackCachePurge = "playback.cache.purge"
	ActionTrashPurge         = "trash.purge"
	// ActionProvidersMetadata runs the FI-1 metadata provider task for a library (whole library, L20).
	ActionProvidersMetadata = "providers.metadata"
	// ActionProvidersPosters runs the FI-1 poster provider task for a library.
	ActionProvidersPosters = "providers.posters"
	// ActionProvidersSubtitles runs the FI-1 subtitle provider task for a library.
	ActionProvidersSubtitles = "providers.subtitles"
)

// Trigger identifies how a run was started.
const (
	TriggerManual   = "manual"
	TriggerSchedule = "schedule"
)

// Run status values.
const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// PurgeRetentionHours is how old an HLS cache dir must be before purge deletes it.
// Hardcoded so schedules only control when purge runs, not how aggressive it is —
// keeps in-flight transcodes (active cache dirs) safe.
const PurgeRetentionHours = 6

// ConfigKeyMaxAgeHours is reported in purge run summaries (not admin-configurable).
const ConfigKeyMaxAgeHours = "maxAgeHours"

var (
	// ErrConflict is returned when the same action+library is already running.
	ErrConflict = errors.New("maintenance action already running")
	// ErrInvalidCron is returned when a cron expression cannot be parsed.
	ErrInvalidCron = errors.New("invalid cron expression")
	// ErrLibraryRequired is returned when a per-library action lacks libraryID.
	ErrLibraryRequired = errors.New("libraryId required for this action")
	// ErrInvalidAction is returned for unknown action keys.
	ErrInvalidAction = errors.New("invalid maintenance action")
	// ErrNotFound is returned when a schedule or run is missing.
	ErrNotFound = errors.New("maintenance record not found")
)

// Schedule is a persisted cron schedule for one action (optionally per library).
type Schedule struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"`
	LibraryID *string        `json:"libraryId,omitempty"`
	Cron      string         `json:"cron"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config,omitempty"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// ScheduleInput is an admin PATCH body for upserting a schedule.
type ScheduleInput struct {
	Action    string         `json:"action"`
	LibraryID *string        `json:"libraryId,omitempty"`
	Cron      string         `json:"cron"`
	Enabled   bool           `json:"enabled"`
	Config    map[string]any `json:"config,omitempty"`
}

// Run is one execution of a maintenance action.
type Run struct {
	ID         string         `json:"id"`
	Action     string         `json:"action"`
	LibraryID  *string        `json:"libraryId,omitempty"`
	Trigger    string         `json:"trigger"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
	Summary    map[string]any `json:"summary,omitempty"`
}

// ActionStatus combines schedule + latest run for API responses.
type ActionStatus struct {
	Action    string    `json:"action"`
	Schedule  *Schedule `json:"schedule,omitempty"`
	LatestRun *Run      `json:"latestRun,omitempty"`
	Running   bool      `json:"running"`
}

// cronParser is the 5-field standard parser (minute hour dom month dow).
//
//nolint:gochecknoglobals // shared parser
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// IsGlobalAction reports whether action has no library scope.
func IsGlobalAction(action string) bool {
	switch action {
	case ActionLibrariesScan, ActionPlaybackCachePurge, ActionTrashPurge:
		return true
	default:
		return false
	}
}

// IsLibraryAction reports whether action requires a library ID.
func IsLibraryAction(action string) bool {
	switch action {
	case ActionMetadataScan, ActionThumbnailsWarm,
		ActionProvidersMetadata, ActionProvidersPosters, ActionProvidersSubtitles:
		return true
	default:
		return false
	}
}

// ValidAction reports whether action is a known catalog key.
func ValidAction(action string) bool {
	return IsGlobalAction(action) || IsLibraryAction(action)
}

// ValidateCron parses a 5-field cron expression. Empty cron is allowed (manual-only).
func ValidateCron(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}

	_, err := cronParser.Parse(expr)
	if err != nil {
		return ErrInvalidCron
	}

	return nil
}

// LockKey builds the in-process lock key for an action+library pair.
func LockKey(action string, libraryID string) string {
	return action + "|" + libraryID
}
