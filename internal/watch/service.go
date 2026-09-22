package watch

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
)

// Repository persists watched state rows.
type Repository interface {
	Get(ctx context.Context, userID, libraryID, relPath string) (State, error)
	Upsert(ctx context.Context, userID, libraryID, relPath string, state State) error
	Clear(ctx context.Context, userID, libraryID, relPath string) error
	ListForPaths(
		ctx context.Context,
		userID, libraryID string,
		relPaths []string,
	) (map[string]bool, error)
	DeleteForPath(ctx context.Context, libraryID, relPath string) error
}

// Service resolves media paths to libraries and stores watched state.
type Service struct {
	media  *mediafs.Service
	access *access.Service
	store  Repository
}

// NewService constructs a watch service.
func NewService(media *mediafs.Service, accessService *access.Service, store Repository) *Service {
	return &Service{
		media:  media,
		access: accessService,
		store:  store,
	}
}

// Get returns watched status for the current user and media path.
func (s *Service) Get(ctx context.Context, userID, rawPath string) (State, error) {
	if s == nil || s.store == nil {
		return State{}, ErrStoreUnavailable
	}

	libraryID, relPath, err := s.resolvePath(ctx, rawPath)
	if err != nil {
		return State{}, err
	}

	state, err := s.store.Get(ctx, userID, libraryID, relPath)
	if err != nil {
		return State{}, fmt.Errorf("get watch state: %w", err)
	}
	if !state.Watched && !state.HasProgress() {
		if legacy := pathEscapeSegments(relPath); legacy != relPath {
			state, err = s.store.Get(ctx, userID, libraryID, legacy)
			if err != nil {
				return State{}, fmt.Errorf("get watch state: %w", err)
			}
		}
	}

	return state, nil
}

// Set updates watched status for the current user and media path.
func (s *Service) Set(ctx context.Context, userID, rawPath string, watched bool) (State, error) {
	return s.Patch(ctx, userID, rawPath, Patch{Watched: &watched})
}

// Patch applies a partial watch-state update for the current user and media path.
func (s *Service) Patch(ctx context.Context, userID, rawPath string, patch Patch) (State, error) {
	if s == nil || s.store == nil {
		return State{}, ErrStoreUnavailable
	}

	libraryID, relPath, err := s.resolvePath(ctx, rawPath)
	if err != nil {
		return State{}, err
	}

	current, err := s.store.Get(ctx, userID, libraryID, relPath)
	if err != nil {
		return State{}, fmt.Errorf("get watch state: %w", err)
	}

	if patch.Watched != nil && !*patch.Watched {
		err = s.store.Clear(ctx, userID, libraryID, relPath)
		if err != nil {
			return State{}, fmt.Errorf("clear watch state: %w", err)
		}
		s.clearLegacy(ctx, userID, libraryID, relPath)

		return State{}, nil
	}

	next := mergeWatchPatch(current, patch)
	if skipProgressWrite(current, next) {
		return current, nil
	}

	err = s.store.Upsert(ctx, userID, libraryID, relPath, next)
	if err != nil {
		return State{}, fmt.Errorf("set watch state: %w", err)
	}
	s.clearLegacy(ctx, userID, libraryID, relPath)

	return next, nil
}

func mergeWatchPatch(current State, patch Patch) State {
	next := current
	if patch.PositionSeconds != nil {
		next.PositionSeconds = patch.PositionSeconds
	}
	if patch.DurationSeconds != nil {
		next.DurationSeconds = patch.DurationSeconds
	}
	if patch.Watched != nil && *patch.Watched {
		next.Watched = true
	}
	if IsComplete(derefSeconds(next.PositionSeconds), derefSeconds(next.DurationSeconds)) {
		next.Watched = true
	}

	return next
}

func skipProgressWrite(current, next State) bool {
	return !next.Watched &&
		!ShouldSaveProgress(derefSeconds(next.PositionSeconds)) &&
		!current.Watched &&
		current.PositionSeconds == nil
}

func derefSeconds(value *float64) float64 {
	if value == nil {
		return 0
	}

	return *value
}

// WatchedForPaths returns watched flags for browse items under one library folder.
func (s *Service) WatchedForPaths(
	ctx context.Context,
	userID, folderPath string,
	itemPaths []string,
) (map[string]bool, error) {
	if s == nil || s.store == nil || len(itemPaths) == 0 {
		return map[string]bool{}, nil
	}

	libraryID, _, err := s.resolvePath(ctx, folderPath)
	if err != nil {
		return map[string]bool{}, fmt.Errorf("resolve folder path: %w", err)
	}

	flags, err := s.store.ListForPaths(ctx, userID, libraryID, watchLookupPaths(itemPaths))
	if err != nil {
		return nil, fmt.Errorf("list watch state: %w", err)
	}

	return mapWatchedFlags(itemPaths, flags), nil
}

// watchLookupPaths expands decoded media paths with legacy percent-encoded variants.
func watchLookupPaths(itemPaths []string) []string {
	const pathVariants = 2
	relPaths := make([]string, 0, len(itemPaths)*pathVariants)
	seen := make(map[string]struct{}, len(itemPaths)*pathVariants)
	for _, itemPath := range itemPaths {
		relPath := normalizeMediaPath(itemPath)
		if relPath == "" {
			continue
		}
		for _, candidate := range []string{relPath, pathEscapeSegments(relPath)} {
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			relPaths = append(relPaths, candidate)
		}
	}

	return relPaths
}

func mapWatchedFlags(itemPaths []string, flags map[string]bool) map[string]bool {
	watchedNorm := make(map[string]bool, len(flags))
	for path, on := range flags {
		if on {
			watchedNorm[normalizeMediaPath(path)] = true
		}
	}

	out := make(map[string]bool, len(itemPaths))
	for _, itemPath := range itemPaths {
		if watchedNorm[normalizeMediaPath(itemPath)] {
			out[itemPath] = true
		}
	}

	return out
}

// DeleteForPath removes watch rows when media is deleted.
func (s *Service) DeleteForPath(ctx context.Context, rawPath string) error {
	if s == nil || s.store == nil {
		return nil
	}

	libraryID, relPath, resolveErr := s.resolvePath(ctx, rawPath)
	if resolveErr != nil {
		if errors.Is(resolveErr, ErrUnknownLibrary) {
			return nil
		}

		return fmt.Errorf("resolve watch path: %w", resolveErr)
	}

	err := s.store.DeleteForPath(ctx, libraryID, relPath)
	if err != nil {
		return fmt.Errorf("delete watch state: %w", err)
	}

	return nil
}

func (s *Service) clearLegacy(ctx context.Context, userID, libraryID, relPath string) {
	if legacy := pathEscapeSegments(relPath); legacy != relPath {
		_ = s.store.Clear(ctx, userID, libraryID, legacy)
	}
}

func (s *Service) resolvePath(ctx context.Context, rawPath string) (string, string, error) {
	relPath := normalizeMediaPath(rawPath)
	if relPath == "" {
		return "", "", fmt.Errorf("%w: empty path", mediafs.ErrInvalidPath)
	}

	if s.access == nil {
		return "", "", fmt.Errorf("%w: access unavailable", ErrUnknownLibrary)
	}

	libraries, err := s.access.ListLibraries(ctx)
	if err != nil {
		return "", "", fmt.Errorf("list libraries: %w", err)
	}

	library, ok := access.MatchLibrary(libraries, relPath)
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrUnknownLibrary, relPath)
	}

	return library.ID, relPath, nil
}

func normalizeMediaPath(rawPath string) string {
	cleaned := strings.TrimSpace(rawPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = filepath.ToSlash(cleaned)
	// openapi-fetch + encodeMediaPath used to leave %XX sequences in the Gin path param.
	for {
		unescaped, err := url.PathUnescape(cleaned)
		if err != nil || unescaped == cleaned {
			break
		}
		cleaned = unescaped
	}

	return cleaned
}

// pathEscapeSegments rebuilds the legacy double-encoded form for lookup/cleanup.
func pathEscapeSegments(relPath string) string {
	parts := strings.Split(relPath, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}

	return strings.Join(parts, "/")
}
