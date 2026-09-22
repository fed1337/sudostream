package provider

import (
	"slices"
	"sort"
)

// Registry holds the adapters compiled into this build, so settings validation and the admin
// UI dropdown only ever offer real implementations (FI-1: validity lives in code, not in a DB
// CHECK constraint, so a new adapter needs no migration).
type Registry struct {
	metadata map[string]MetadataProvider
	poster   map[string]PosterProvider
	subtitle map[string]SubtitleProvider
}

// NewRegistry returns an empty registry. Adapters are registered during wiring
// (cmd/server/main.go).
func NewRegistry() *Registry {
	return &Registry{
		metadata: map[string]MetadataProvider{},
		poster:   map[string]PosterProvider{},
		subtitle: map[string]SubtitleProvider{},
	}
}

// RegisterMetadata makes adapter selectable as metadata_provider.
func (r *Registry) RegisterMetadata(adapter MetadataProvider) {
	if adapter == nil || adapter.Key() == "" {
		return
	}
	r.metadata[adapter.Key()] = adapter
}

// RegisterPoster makes adapter selectable as poster_provider.
func (r *Registry) RegisterPoster(adapter PosterProvider) {
	if adapter == nil || adapter.Key() == "" {
		return
	}
	r.poster[adapter.Key()] = adapter
}

// RegisterSubtitle makes adapter selectable as subtitle_provider.
func (r *Registry) RegisterSubtitle(adapter SubtitleProvider) {
	if adapter == nil || adapter.Key() == "" {
		return
	}
	r.subtitle[adapter.Key()] = adapter
}

// Metadata returns registered metadata provider keys.
func (r *Registry) Metadata() []string { return sortedKeys(r.metadata) }

// Poster returns registered poster provider keys.
func (r *Registry) Poster() []string { return sortedKeys(r.poster) }

// Subtitle returns registered subtitle provider keys.
func (r *Registry) Subtitle() []string { return sortedKeys(r.subtitle) }

// SupportsMetadata reports whether key is a registered metadata provider.
func (r *Registry) SupportsMetadata(key string) bool { return r.metadata[key] != nil }

// SupportsPoster reports whether key is a registered poster provider.
func (r *Registry) SupportsPoster(key string) bool { return r.poster[key] != nil }

// SupportsSubtitle reports whether key is a registered subtitle provider.
func (r *Registry) SupportsSubtitle(key string) bool { return r.subtitle[key] != nil }

// MetadataAdapter returns the metadata adapter for key, if registered.
//
//nolint:ireturn // registry boundary returns category interfaces by design
func (r *Registry) MetadataAdapter(key string) (MetadataProvider, bool) {
	adapter, ok := r.metadata[key]

	return adapter, ok
}

// PosterAdapter returns the poster adapter for key, if registered.
//
//nolint:ireturn // registry boundary returns category interfaces by design
func (r *Registry) PosterAdapter(key string) (PosterProvider, bool) {
	adapter, ok := r.poster[key]

	return adapter, ok
}

// SubtitleAdapter returns the subtitle adapter for key, if registered.
//
//nolint:ireturn // registry boundary returns category interfaces by design
func (r *Registry) SubtitleAdapter(key string) (SubtitleProvider, bool) {
	adapter, ok := r.subtitle[key]

	return adapter, ok
}

func sortedKeys[T any](adapters map[string]T) []string {
	keys := make([]string, 0, len(adapters))
	for key := range adapters {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return slices.Clip(keys)
}
