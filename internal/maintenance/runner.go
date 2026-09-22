// Package maintenance runs scheduled and manual library maintenance actions.
package maintenance

import (
	"context"
	"errors"
	"fmt"
	"sudoStream/internal/provider"
	"time"
)

var (
	errAccessUnavailable    = errors.New("access service unavailable")
	errIndexerUnavailable   = errors.New("metadata indexer unavailable")
	errThumbsUnavailable    = errors.New("thumbnail warmer unavailable")
	errPurgerUnavailable    = errors.New("cache purger unavailable")
	errTrashUnavailable     = errors.New("trash service unavailable")
	errProvidersUnavailable = errors.New("provider settings service unavailable")
)

func (s *Service) runAction(
	ctx context.Context,
	action, libraryID string,
) (map[string]any, error) {
	switch action {
	case ActionLibrariesScan:
		return s.runLibrariesScan(ctx)
	case ActionMetadataScan:
		return s.runMetadataScan(ctx, libraryID)
	case ActionThumbnailsWarm:
		return s.runThumbnailsWarm(ctx, libraryID)
	case ActionPlaybackCachePurge:
		return s.runPlaybackCachePurge(ctx)
	case ActionTrashPurge:
		return s.runTrashPurge(ctx)
	case ActionProvidersMetadata:
		return s.runProviderTask(ctx, libraryID, provider.TaskMetadata)
	case ActionProvidersPosters:
		return s.runProviderTask(ctx, libraryID, provider.TaskPoster)
	case ActionProvidersSubtitles:
		return s.runProviderTask(ctx, libraryID, provider.TaskSubtitle)
	default:
		return nil, ErrInvalidAction
	}
}

func (s *Service) runLibrariesScan(ctx context.Context) (map[string]any, error) {
	if s.deps.Access == nil {
		return nil, errAccessUnavailable
	}

	result, err := s.deps.Access.SyncLibraries(ctx, s.deps.MediaRoot)
	if err != nil {
		return nil, fmt.Errorf("sync libraries: %w", err)
	}

	return map[string]any{
		"added":   result.Added,
		"removed": result.Removed,
		"kept":    result.Kept,
		"total":   len(result.Libraries),
	}, nil
}

func (s *Service) runMetadataScan(ctx context.Context, libraryID string) (map[string]any, error) {
	if s.deps.Indexer == nil || s.deps.Access == nil {
		return nil, errIndexerUnavailable
	}

	library, err := s.deps.Access.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	files, err := s.deps.Indexer.IndexLibrary(ctx, library)
	if err != nil {
		return map[string]any{"filesIndexed": files}, fmt.Errorf("index library: %w", err)
	}

	return map[string]any{"filesIndexed": files}, nil
}

func (s *Service) runThumbnailsWarm(ctx context.Context, libraryID string) (map[string]any, error) {
	if s.deps.Thumbs == nil || s.deps.Media == nil || s.deps.Access == nil {
		return nil, errThumbsUnavailable
	}

	library, err := s.deps.Access.GetLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("get library: %w", err)
	}

	generated := 0
	for _, root := range library.RootPaths() {
		count, warmErr := s.deps.Thumbs.WarmLibrary(ctx, s.deps.Media, root)
		generated += count
		if warmErr != nil {
			return map[string]any{
					"generated": generated,
				}, fmt.Errorf(
					"warm thumbnails: %w",
					warmErr,
				)
		}
	}

	return map[string]any{"generated": generated}, nil
}

func (s *Service) runPlaybackCachePurge(_ context.Context) (map[string]any, error) {
	if s.deps.Purger == nil {
		return nil, errPurgerUnavailable
	}

	deleted, bytesRemoved, err := s.deps.Purger.PurgeStaleCache(
		time.Duration(PurgeRetentionHours) * time.Hour,
	)
	if err != nil {
		return nil, fmt.Errorf("purge cache: %w", err)
	}

	return map[string]any{
		"deleted":            deleted,
		"bytesRemoved":       bytesRemoved,
		ConfigKeyMaxAgeHours: PurgeRetentionHours,
	}, nil
}

func (s *Service) runTrashPurge(ctx context.Context) (map[string]any, error) {
	if s.deps.Trash == nil {
		return nil, errTrashUnavailable
	}

	purged, err := s.deps.Trash.PurgeExpired(ctx)
	if err != nil {
		return nil, fmt.Errorf("purge trash: %w", err)
	}

	return map[string]any{"purged": purged}, nil
}

// runProviderTask handles all three providers.* actions (FI-1 L14/L20). Per-file failures are
// counted in the summary rather than aborting the run. Provider-wide blocks (HTTP 403 / exhausted
// rate limits via provider.ErrProviderUnavailable) fail the whole run so we stop hammering.
func (s *Service) runProviderTask(
	ctx context.Context,
	libraryID string,
	kind provider.TaskKind,
) (map[string]any, error) {
	if s.deps.Providers == nil {
		return nil, errProvidersUnavailable
	}

	summary, err := s.deps.Providers.Enrich(ctx, libraryID, kind)
	if err != nil {
		return summary.Map(), fmt.Errorf("provider %s task: %w", kind, err)
	}

	return summary.Map(), nil
}
