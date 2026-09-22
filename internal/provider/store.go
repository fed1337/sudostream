package provider

import "context"

// Store persists per-library provider settings.
type Store interface {
	// GetSettings returns ErrNotFound when libraryID has no stored row.
	GetSettings(ctx context.Context, libraryID string) (Settings, error)
	// UpsertSettings inserts or replaces the settings row for settings.LibraryID.
	UpsertSettings(ctx context.Context, settings Settings) (Settings, error)
}
