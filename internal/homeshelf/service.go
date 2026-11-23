// Package homeshelf builds personalized media shelves for the home screen.
package homeshelf

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/favorite"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/metadata"
	"sudoStream/internal/watch"

	"golang.org/x/sync/errgroup"
)

const (
	defaultLimit = 28
	maxLimit     = 100
	percentScale = 100
)

var (
	errUnavailable       = errors.New("home shelves unavailable")
	errAccessUnavailable = errors.New("access service unavailable")
)

// ShelfItem is a media item displayed in a home shelf.
type ShelfItem struct {
	Path            string   `json:"path"`
	Title           string   `json:"title"`
	LibrarySlug     string   `json:"librarySlug"`
	LibraryName     string   `json:"libraryName"`
	Watched         bool     `json:"watched"`
	Favorited       bool     `json:"favorited"`
	PosterURL       string   `json:"posterUrl,omitempty"`
	PositionSeconds *float64 `json:"positionSeconds,omitempty"`
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
}

// ItemPage is a paginated home shelf.
type ItemPage struct {
	Items  []ShelfItem `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// LibraryStat summarizes a user's watched progress in a library.
type LibraryStat struct {
	LibraryID      string  `json:"libraryId"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	WatchedCount   int64   `json:"watchedCount"`
	TotalCount     int64   `json:"totalCount"`
	WatchedPercent float64 `json:"watchedPercent"`
}

type favoriteLister interface {
	ListForUser(
		ctx context.Context,
		userID string,
		libraryIDs []string,
		limit, offset int,
	) ([]favorite.Row, error)
	CountForUser(ctx context.Context, userID string, libraryIDs []string) (int64, error)
}

type watchLister interface {
	ListForUser(
		ctx context.Context,
		userID string,
		libraryIDs []string,
		limit, offset int,
	) ([]watch.Row, error)
	ListContinue(
		ctx context.Context,
		userID string,
		libraryIDs []string,
		limit, offset int,
	) ([]watch.Row, error)
	CountForUser(ctx context.Context, userID string, libraryIDs []string) (int64, error)
	CountContinue(ctx context.Context, userID string, libraryIDs []string) (int64, error)
	CountByLibraries(
		ctx context.Context,
		userID string,
		libraryIDs []string,
	) (map[string]int64, error)
	ListForPaths(
		ctx context.Context,
		userID, libraryID string,
		relPaths []string,
	) (map[string]bool, error)
}

type metadataLister interface {
	CountByLibraries(ctx context.Context, libraryIDs []string) (map[string]int64, error)
	ListUnwatched(
		ctx context.Context,
		userID string,
		libraryIDs []string,
		limit, offset int,
	) ([]metadata.ShelfRow, error)
	CountUnwatched(ctx context.Context, userID string, libraryIDs []string) (int64, error)
	ListShelfItems(
		ctx context.Context,
		libraryIDs []string,
		relPaths []string,
	) ([]metadata.ShelfRow, error)
}

// PosterIndex reports which of the given media paths have a locally cached provider poster.
type PosterIndex interface {
	PosterPathsFor(
		ctx context.Context,
		libraryID string,
		relPaths []string,
	) (map[string]struct{}, error)
}

// Service collects home shelf data from ACL, state, and metadata stores.
type Service struct {
	access    *access.Service
	favorites favoriteLister
	watch     watchLister
	metadata  metadataLister
	posters   PosterIndex
}

// NewService constructs a home shelf service.
func NewService(
	accessService *access.Service,
	favorites favoriteLister,
	watchService watchLister,
	metadataService metadataLister,
) *Service {
	return &Service{
		access:    accessService,
		favorites: favorites,
		watch:     watchService,
		metadata:  metadataService,
	}
}

// SetPosterIndex enables provider poster URLs on shelf cards. Safe to call with nil.
func (s *Service) SetPosterIndex(posters PosterIndex) {
	if s == nil {
		return
	}
	s.posters = posters
}

// Favorites returns the user's newest favorites from readable libraries.
//

func (s *Service) Favorites(
	ctx context.Context,
	user auth.PublicUser,
	limit, offset int,
) (ItemPage, error) {
	if s == nil || s.favorites == nil || s.metadata == nil {
		return ItemPage{}, errUnavailable
	}

	libraries, ids, err := s.readableLibraries(ctx, user)
	if err != nil {
		return ItemPage{}, err
	}
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)

	var total int64
	var rows []favorite.Row
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		count, countErr := s.favorites.CountForUser(groupCtx, user.ID, ids)
		if countErr != nil {
			return fmt.Errorf("count favorites: %w", countErr)
		}
		total = count

		return nil
	})
	group.Go(func() error {
		listed, listErr := s.favorites.ListForUser(groupCtx, user.ID, ids, limit, offset)
		if listErr != nil {
			return fmt.Errorf("list favorites: %w", listErr)
		}
		rows = listed

		return nil
	})
	waitErr := group.Wait()
	if waitErr != nil {
		return ItemPage{}, fmt.Errorf("favorites count/list: %w", waitErr)
	}

	items, err := s.itemsFromFavorites(ctx, libraries, ids, rows)
	if err != nil {
		return ItemPage{}, err
	}
	for index := range items {
		items[index].Favorited = true
	}
	s.applyWatchedFlags(ctx, user.ID, rowsToShelfKeys(rows), items)

	return ItemPage{Items: items, Total: int(total), Limit: limit, Offset: offset}, nil
}

// Watched returns the user's most recently watched media from readable libraries.
//

func (s *Service) Watched(
	ctx context.Context,
	user auth.PublicUser,
	limit, offset int,
) (ItemPage, error) {
	if s == nil || s.watch == nil || s.metadata == nil {
		return ItemPage{}, errUnavailable
	}

	libraries, ids, err := s.readableLibraries(ctx, user)
	if err != nil {
		return ItemPage{}, err
	}
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)

	var total int64
	var rows []watch.Row
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		count, countErr := s.watch.CountForUser(groupCtx, user.ID, ids)
		if countErr != nil {
			return fmt.Errorf("count watched: %w", countErr)
		}
		total = count

		return nil
	})
	group.Go(func() error {
		listed, listErr := s.watch.ListForUser(groupCtx, user.ID, ids, limit, offset)
		if listErr != nil {
			return fmt.Errorf("list watched: %w", listErr)
		}
		rows = listed

		return nil
	})
	waitErr := group.Wait()
	if waitErr != nil {
		return ItemPage{}, fmt.Errorf("watched count/list: %w", waitErr)
	}

	items, err := s.itemsFromWatchRows(ctx, libraries, ids, rows)
	if err != nil {
		return ItemPage{}, err
	}
	for index := range items {
		items[index].Watched = true
	}

	return ItemPage{Items: items, Total: int(total), Limit: limit, Offset: offset}, nil
}

// Continue returns in-progress titles for the Continue watching shelf.
//

func (s *Service) Continue(
	ctx context.Context,
	user auth.PublicUser,
	limit, offset int,
) (ItemPage, error) {
	if s == nil || s.watch == nil || s.metadata == nil {
		return ItemPage{}, errUnavailable
	}

	libraries, ids, err := s.readableLibraries(ctx, user)
	if err != nil {
		return ItemPage{}, err
	}
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)

	var total int64
	var rows []watch.Row
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		count, countErr := s.watch.CountContinue(groupCtx, user.ID, ids)
		if countErr != nil {
			return fmt.Errorf("count continue: %w", countErr)
		}
		total = count

		return nil
	})
	group.Go(func() error {
		listed, listErr := s.watch.ListContinue(groupCtx, user.ID, ids, limit, offset)
		if listErr != nil {
			return fmt.Errorf("list continue: %w", listErr)
		}
		rows = listed

		return nil
	})
	waitErr := group.Wait()
	if waitErr != nil {
		return ItemPage{}, fmt.Errorf("continue count/list: %w", waitErr)
	}

	items, err := s.itemsFromWatchRows(ctx, libraries, ids, rows)
	if err != nil {
		return ItemPage{}, err
	}

	return ItemPage{Items: items, Total: int(total), Limit: limit, Offset: offset}, nil
}

// Unwatched returns indexed, unread media from readable libraries.
func (s *Service) Unwatched(
	ctx context.Context,
	user auth.PublicUser,
	limit, offset int,
) (ItemPage, error) {
	if s == nil || s.metadata == nil {
		return ItemPage{}, errUnavailable
	}

	libraries, ids, err := s.readableLibraries(ctx, user)
	if err != nil {
		return ItemPage{}, err
	}
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)

	var total int64
	var rows []metadata.ShelfRow
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		count, countErr := s.metadata.CountUnwatched(groupCtx, user.ID, ids)
		if countErr != nil {
			return fmt.Errorf("count unwatched: %w", countErr)
		}
		total = count

		return nil
	})
	group.Go(func() error {
		listed, listErr := s.metadata.ListUnwatched(groupCtx, user.ID, ids, limit, offset)
		if listErr != nil {
			return fmt.Errorf("list unwatched: %w", listErr)
		}
		rows = listed

		return nil
	})
	waitErr := group.Wait()
	if waitErr != nil {
		return ItemPage{}, fmt.Errorf("unwatched count/list: %w", waitErr)
	}

	return ItemPage{
		Items:  s.itemsFromShelfRows(ctx, libraries, rows),
		Total:  int(total),
		Limit:  limit,
		Offset: offset,
	}, nil
}

// Stats returns watch progress for each readable library.
func (s *Service) Stats(ctx context.Context, user auth.PublicUser) ([]LibraryStat, error) {
	if s == nil || s.watch == nil || s.metadata == nil {
		return nil, errUnavailable
	}

	libraries, ids, err := s.readableLibraries(ctx, user)
	if err != nil {
		return nil, err
	}

	var totals map[string]int64
	var watched map[string]int64
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		counts, countErr := s.metadata.CountByLibraries(groupCtx, ids)
		if countErr != nil {
			return fmt.Errorf("count indexed media: %w", countErr)
		}
		totals = counts

		return nil
	})
	group.Go(func() error {
		counts, countErr := s.watch.CountByLibraries(groupCtx, user.ID, ids)
		if countErr != nil {
			return fmt.Errorf("count watched media: %w", countErr)
		}
		watched = counts

		return nil
	})
	waitErr := group.Wait()
	if waitErr != nil {
		return nil, fmt.Errorf("stats counts: %w", waitErr)
	}

	stats := make([]LibraryStat, 0, len(libraries))
	for _, library := range libraries {
		total := totals[library.ID]
		count := min(watched[library.ID], total)
		percent := float64(0)
		if total > 0 {
			percent = float64(count) / float64(total) * percentScale
		}
		stats = append(stats, LibraryStat{
			LibraryID:      library.ID,
			Name:           library.Name,
			Slug:           library.Slug,
			WatchedCount:   count,
			TotalCount:     total,
			WatchedPercent: percent,
		})
	}

	return stats, nil
}

func (s *Service) readableLibraries(
	ctx context.Context,
	user auth.PublicUser,
) ([]access.Library, []string, error) {
	if s.access == nil {
		return nil, nil, errAccessUnavailable
	}

	libraries, err := s.access.ListReadableLibraries(ctx, user)
	if err != nil {
		return nil, nil, fmt.Errorf("list readable libraries: %w", err)
	}

	ids := make([]string, 0, len(libraries))
	for _, library := range libraries {
		ids = append(ids, library.ID)
	}

	return libraries, ids, nil
}

func (s *Service) itemsFromFavorites(
	ctx context.Context,
	libraries []access.Library,
	libraryIDs []string,
	rows []favorite.Row,
) ([]ShelfItem, error) {
	shelfRows := make([]metadata.ShelfRow, 0, len(rows))
	for _, row := range rows {
		shelfRows = append(shelfRows, metadata.ShelfRow{
			LibraryID: row.LibraryID,
			RelPath:   row.RelPath,
		})
	}

	return s.itemsFromRows(ctx, libraries, libraryIDs, shelfRows)
}

func (s *Service) itemsFromRows(
	ctx context.Context,
	libraries []access.Library,
	libraryIDs []string,
	rows []metadata.ShelfRow,
) ([]ShelfItem, error) {
	paths := make([]string, 0, len(rows))
	for _, row := range rows {
		paths = append(paths, row.RelPath)
	}
	metadataRows, err := s.metadata.ListShelfItems(ctx, libraryIDs, paths)
	if err != nil {
		return nil, fmt.Errorf("list shelf metadata: %w", err)
	}

	titles := make(map[string]string, len(metadataRows))
	for _, row := range metadataRows {
		titles[row.LibraryID+"\x00"+row.RelPath] = row.Title
	}
	for index := range rows {
		rows[index].Title = titles[rows[index].LibraryID+"\x00"+rows[index].RelPath]
	}

	return s.itemsFromShelfRows(ctx, libraries, rows), nil
}

func (s *Service) itemsFromShelfRows(
	ctx context.Context,
	libraries []access.Library,
	rows []metadata.ShelfRow,
) []ShelfItem {
	byLibrary := librariesByID(libraries)
	postersByLibrary := s.posterPathsForPage(ctx, rows)
	items := make([]ShelfItem, 0, len(rows))
	for _, row := range rows {
		item := shelfItem(byLibrary, row)
		item.PosterURL = posterURLFor(postersByLibrary[row.LibraryID], row.RelPath)
		items = append(items, item)
	}

	return items
}

func (s *Service) posterPathsForPage(
	ctx context.Context,
	rows []metadata.ShelfRow,
) map[string]map[string]struct{} {
	if s == nil || s.posters == nil || len(rows) == 0 {
		return nil
	}

	pathsByLibrary := map[string][]string{}
	for _, row := range rows {
		if row.LibraryID == "" || row.RelPath == "" {
			continue
		}
		pathsByLibrary[row.LibraryID] = append(pathsByLibrary[row.LibraryID], row.RelPath)
	}

	out := make(map[string]map[string]struct{}, len(pathsByLibrary))
	for libraryID, paths := range pathsByLibrary {
		listed, err := s.posters.PosterPathsFor(ctx, libraryID, paths)
		if err != nil {
			continue
		}
		out[libraryID] = listed
	}

	return out
}

func posterURLFor(paths map[string]struct{}, relPath string) string {
	if paths == nil {
		return ""
	}
	if _, hit := paths[relPath]; !hit {
		return ""
	}

	return "/api/provider-poster/" + mediafs.EscapePathSegments(relPath)
}

func librariesByID(libraries []access.Library) map[string]access.Library {
	byID := make(map[string]access.Library, len(libraries))
	for _, library := range libraries {
		byID[library.ID] = library
	}

	return byID
}

func shelfItem(libraries map[string]access.Library, row metadata.ShelfRow) ShelfItem {
	title := row.Title
	if title == "" {
		title = path.Base(row.RelPath)
	}
	library := libraries[row.LibraryID]
	libraryName := library.Name
	if libraryName == "" {
		libraryName = library.Slug
	}

	return ShelfItem{
		Path:        row.RelPath,
		Title:       title,
		LibrarySlug: library.Slug,
		LibraryName: libraryName,
	}
}

type shelfKey struct {
	LibraryID string
	RelPath   string
}

func rowsToShelfKeys(rows []favorite.Row) []shelfKey {
	keys := make([]shelfKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, shelfKey{LibraryID: row.LibraryID, RelPath: row.RelPath})
	}

	return keys
}

func (s *Service) applyWatchedFlags(
	ctx context.Context,
	userID string,
	keys []shelfKey,
	items []ShelfItem,
) {
	if s == nil || s.watch == nil || userID == "" || len(keys) == 0 || len(keys) != len(items) {
		return
	}

	for libraryID, indexes := range groupShelfIndexes(keys) {
		s.fillWatchedForLibrary(ctx, userID, libraryID, indexes, keys, items)
	}
}

func (s *Service) fillWatchedForLibrary(
	ctx context.Context,
	userID, libraryID string,
	indexes []int,
	keys []shelfKey,
	items []ShelfItem,
) {
	paths := make([]string, 0, len(indexes))
	for _, index := range indexes {
		paths = append(paths, keys[index].RelPath)
	}
	flags, err := s.watch.ListForPaths(ctx, userID, libraryID, paths)
	if err != nil || flags == nil {
		return
	}
	for _, index := range indexes {
		items[index].Watched = flags[keys[index].RelPath]
	}
}

func groupShelfIndexes(keys []shelfKey) map[string][]int {
	byLibrary := map[string][]int{}
	for index, key := range keys {
		if key.LibraryID == "" || key.RelPath == "" {
			continue
		}
		byLibrary[key.LibraryID] = append(byLibrary[key.LibraryID], index)
	}

	return byLibrary
}

func (s *Service) itemsFromWatchRows(
	ctx context.Context,
	libraries []access.Library,
	libraryIDs []string,
	rows []watch.Row,
) ([]ShelfItem, error) {
	shelfRows := make([]metadata.ShelfRow, 0, len(rows))
	for _, row := range rows {
		shelfRows = append(shelfRows, metadata.ShelfRow{
			LibraryID: row.LibraryID,
			RelPath:   row.RelPath,
		})
	}
	items, err := s.itemsFromRows(ctx, libraries, libraryIDs, shelfRows)
	if err != nil {
		return nil, err
	}
	for index := range items {
		if index >= len(rows) {
			break
		}
		items[index].PositionSeconds = rows[index].PositionSeconds
		items[index].DurationSeconds = rows[index].DurationSeconds
	}

	return items, nil
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}

	return offset
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}

	return limit
}
