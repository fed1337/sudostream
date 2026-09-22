// Package favorite stores per-user favorite flags for media files.
package favorite

import (
	"errors"
	"time"
)

var (
	// ErrUnknownLibrary indicates the media path does not belong to a known library.
	ErrUnknownLibrary = errors.New("unknown library")
	// ErrStoreUnavailable indicates the favorite repository is not configured.
	ErrStoreUnavailable = errors.New("favorite store unavailable")
)

// State holds per-user favorite status for one media file.
type State struct {
	Favorited bool `json:"favorited"`
}

// Row is a favorite entry used for user-level listings.
type Row struct {
	LibraryID string
	RelPath   string
	CreatedAt time.Time
}
