package provider

import (
	"context"
	"time"
)

// Artifact kinds stored in provider_artifacts.
const (
	// ArtifactKindPoster is a cached poster image for a file or show.
	ArtifactKindPoster = "poster"
	// ArtifactKindSubtitle is a cached subtitle track for one language (E3).
	ArtifactKindSubtitle = "subtitle"
)

// Artifact maps one locally cached provider file to the media path it belongs to. Browse,
// catalog, and player read the cache through these rows and never call a provider API (L7).
type Artifact struct {
	ID          string
	LibraryID   string
	RelPath     string
	Kind        string
	Lang        *string
	ProviderKey string
	CachePath   string
	ExternalID  *string
	FetchedAt   time.Time
}

// ArtifactStore persists the provider cache index.
type ArtifactStore interface {
	// GetArtifactByPath returns ErrNotFound when no row matches. Media paths are relative to
	// the media root and library roots are exclusive, so a path identifies at most one row per
	// (kind, lang) without needing the library id.
	GetArtifactByPath(
		ctx context.Context,
		relPath, kind string,
		lang *string,
	) (Artifact, error)
	// ListArtifacts returns every row of kind for a library.
	ListArtifacts(ctx context.Context, libraryID, kind string) ([]Artifact, error)
	// ListArtifactsByPath returns every row of kind for a media path (all langs).
	ListArtifactsByPath(ctx context.Context, relPath, kind string) ([]Artifact, error)
	// UpsertArtifact inserts or replaces a row, keyed by (library, path, kind, lang).
	UpsertArtifact(ctx context.Context, artifact Artifact) (Artifact, error)
	// DeleteArtifact removes one row by id.
	DeleteArtifact(ctx context.Context, id string) error
}
