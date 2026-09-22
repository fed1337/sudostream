package favorite

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

// Repository persists favorite rows.
type Repository interface {
	Get(ctx context.Context, userID, libraryID, relPath string) (bool, error)
	Set(ctx context.Context, userID, libraryID, relPath string, favorited bool) error
	ListForPaths(
		ctx context.Context,
		userID, libraryID string,
		relPaths []string,
	) (map[string]bool, error)
	DeleteForPath(ctx context.Context, libraryID, relPath string) error
}

// Service resolves media paths to libraries and stores favorites.
type Service struct {
	media  *mediafs.Service
	access *access.Service
	store  Repository
}

// NewService constructs a favorite service.
func NewService(media *mediafs.Service, accessService *access.Service, store Repository) *Service {
	return &Service{
		media:  media,
		access: accessService,
		store:  store,
	}
}

// Get returns favorite status for the current user and media path.
func (s *Service) Get(ctx context.Context, userID, rawPath string) (State, error) {
	if s == nil || s.store == nil {
		return State{}, ErrStoreUnavailable
	}

	libraryID, relPath, err := s.resolvePath(ctx, rawPath)
	if err != nil {
		return State{}, err
	}

	favorited, err := s.store.Get(ctx, userID, libraryID, relPath)
	if err != nil {
		return State{}, fmt.Errorf("get favorite: %w", err)
	}

	return State{Favorited: favorited}, nil
}

// Set updates favorite status for the current user and media path.
func (s *Service) Set(
	ctx context.Context,
	userID, rawPath string,
	favorited bool,
) (State, error) {
	if s == nil || s.store == nil {
		return State{}, ErrStoreUnavailable
	}

	libraryID, relPath, err := s.resolvePath(ctx, rawPath)
	if err != nil {
		return State{}, err
	}

	err = s.store.Set(ctx, userID, libraryID, relPath, favorited)
	if err != nil {
		return State{}, fmt.Errorf("set favorite: %w", err)
	}

	return State{Favorited: favorited}, nil
}

// FavoritedForPaths returns favorite flags for browse items under one library folder.
func (s *Service) FavoritedForPaths(
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

	relPaths := make([]string, 0, len(itemPaths))
	for _, itemPath := range itemPaths {
		relPath := normalizeMediaPath(itemPath)
		if relPath == "" {
			continue
		}
		relPaths = append(relPaths, relPath)
	}

	flags, err := s.store.ListForPaths(ctx, userID, libraryID, relPaths)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}

	out := make(map[string]bool, len(itemPaths))
	for _, itemPath := range itemPaths {
		if flags[normalizeMediaPath(itemPath)] {
			out[itemPath] = true
		}
	}

	return out, nil
}

// DeleteForPath removes favorite rows when media is deleted.
func (s *Service) DeleteForPath(ctx context.Context, rawPath string) error {
	if s == nil || s.store == nil {
		return nil
	}

	libraryID, relPath, resolveErr := s.resolvePath(ctx, rawPath)
	if resolveErr != nil {
		if errors.Is(resolveErr, ErrUnknownLibrary) {
			return nil
		}

		return fmt.Errorf("resolve favorite path: %w", resolveErr)
	}

	err := s.store.DeleteForPath(ctx, libraryID, relPath)
	if err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}

	return nil
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
	for {
		unescaped, err := url.PathUnescape(cleaned)
		if err != nil || unescaped == cleaned {
			break
		}
		cleaned = unescaped
	}

	return cleaned
}
