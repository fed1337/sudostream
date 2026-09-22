package trash

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when a trash item id is unknown.
	ErrNotFound = errors.New("trash item not found")
	// ErrUnavailable is returned when the trash service is not wired.
	ErrUnavailable = errors.New("trash service unavailable")
	// ErrSettingsUnavailable is returned when the settings KV is missing.
	ErrSettingsUnavailable = errors.New("trash settings store unavailable")
	// ErrInvalidRetention is returned when retentionDays is out of range.
	ErrInvalidRetention = errors.New("invalid retentionDays")
)

const (
	settingsKey      = "recycle_bin"
	maxRetentionDays = 36500
)

// Item is one recycle-bin operation (file or folder root).
type Item struct {
	ID              string    `json:"id"`
	OriginalRelPath string    `json:"originalRelPath"`
	TrashRelPath    string    `json:"trashRelPath"`
	LibraryID       string    `json:"libraryId,omitempty"`
	DeletedAt       time.Time `json:"deletedAt"`
	DeletedBy       string    `json:"deletedBy,omitempty"`
}

// Settings is stored under settings.key = recycle_bin.
type Settings struct {
	RetentionDays int `json:"retentionDays"`
}

// DefaultSettings keeps items until an admin empties trash.
func DefaultSettings() Settings {
	return Settings{RetentionDays: 0}
}

// Store persists trash_items rows.
type Store interface {
	Insert(ctx context.Context, item Item) error
	Get(ctx context.Context, id string) (Item, error)
	List(ctx context.Context) ([]Item, error)
	Delete(ctx context.Context, id string) error
	OriginalPrefixes(ctx context.Context) ([]string, error)
}

// SettingsKV reads and writes opaque JSON blobs in the shared settings table.
type SettingsKV interface {
	GetSettingValue(ctx context.Context, key string) ([]byte, error)
	SaveSettingValue(ctx context.Context, key string, value []byte) error
}

// Cleaner runs after a permanent delete while the trash copy may still exist.
type Cleaner interface {
	OnPermanentDelete(ctx context.Context, item Item) error
}
