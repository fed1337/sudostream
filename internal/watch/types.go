// Package watch stores per-user watched state for media files.
package watch

import (
	"errors"
	"time"
)

var (
	// ErrNotFound indicates no watch row exists for the requested path.
	ErrNotFound = errors.New("watch state not found")
	// ErrUnknownLibrary indicates the media path does not belong to a known library.
	ErrUnknownLibrary = errors.New("unknown library")
	// ErrStoreUnavailable indicates the watch repository is not configured.
	ErrStoreUnavailable = errors.New("watch store unavailable")
)

// State holds per-user watched status and playback progress for one media file.
type State struct {
	Watched         bool     `json:"watched"`
	PositionSeconds *float64 `json:"positionSeconds,omitempty"`
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
}

// HasProgress reports whether a row should appear on Continue watching.
func (s State) HasProgress() bool {
	return !s.Watched && s.PositionSeconds != nil && ShouldSaveProgress(*s.PositionSeconds)
}

// Row is a watch entry used for user-level listings.
type Row struct {
	LibraryID       string
	RelPath         string
	WatchedAt       time.Time
	PositionSeconds *float64
	DurationSeconds *float64
}
