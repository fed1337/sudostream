// Package access enforces per-library media permissions.
package access

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"unicode"
)

var (
	// ErrAccessDenied is returned when a user lacks permission for a path.
	ErrAccessDenied = errors.New("access denied")
	// ErrInvalidPermissions is returned when a grant has no permission flags set.
	ErrInvalidPermissions = errors.New("at least one permission is required")
	// ErrUnknownLibrary is returned when a grant references an unknown library.
	ErrUnknownLibrary = errors.New("unknown library")
	// ErrAdminGrantsImmutable is returned when trying to assign grants to an admin.
	ErrAdminGrantsImmutable = errors.New("admin users have implicit full access")
	// ErrInvalidLibraryName is returned when a display name is empty after trim.
	ErrInvalidLibraryName = errors.New("library name is required")
	// ErrInvalidLibraryPatch is returned when a library patch has no fields.
	ErrInvalidLibraryPatch = errors.New("library patch requires name and/or type")
)

// Library is an admin-defined set of media folders with a shared ACL and type.
type Library struct {
	ID      string      `json:"id"`
	Slug    string      `json:"slug"`
	RelPath string      `json:"relPath"`
	Name    string      `json:"name"`
	Type    LibraryType `json:"type"`
	Roots   []string    `json:"roots"`
}

// LibraryPermissions holds independent CRUD flags for a library grant.
type LibraryPermissions struct {
	Create bool `json:"create"`
	Read   bool `json:"read"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

// IsEmpty reports whether no permissions are granted.
func (p LibraryPermissions) IsEmpty() bool {
	return !p.Create && !p.Read && !p.Update && !p.Delete
}

// HasAny reports whether at least one permission is granted.
func (p LibraryPermissions) HasAny() bool {
	return !p.IsEmpty()
}

// GrantInput is used when updating user grants.
type GrantInput struct {
	LibraryID   string             `json:"libraryId"`
	Permissions LibraryPermissions `json:"permissions"`
}

// UserGrantView combines library metadata with a user's permissions.
type UserGrantView struct {
	Library     Library            `json:"library"`
	Permissions LibraryPermissions `json:"permissions"`
}

// Store persists libraries and grants.
type Store interface {
	ListLibraries(ctx context.Context) ([]Library, error)
	GetLibrary(ctx context.Context, libraryID string) (Library, error)
	CreateLibrary(ctx context.Context, library Library) (Library, error)
	UpsertLibrary(ctx context.Context, library Library) (Library, error)
	DeleteLibrary(ctx context.Context, libraryID string) error
	UpdateLibrary(
		ctx context.Context,
		libraryID string,
		name *string,
		libraryType *LibraryType,
	) (Library, error)
	AddRoot(ctx context.Context, libraryID, relPath string) (Library, error)
	RemoveRoot(ctx context.Context, libraryID, relPath string) (Library, error)
	GetUserGrantMap(ctx context.Context, userID string) (map[string]LibraryPermissions, error)
	ReplaceUserGrants(ctx context.Context, userID string, grants []GrantInput) error
}

// Service checks and manages library access.
type Service struct {
	store Store
}

// NewService constructs an access service.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// CanRead reports whether role/user may read the given media path.
func (s *Service) CanRead(ctx context.Context, user auth.PublicUser, rawPath string) (bool, error) {
	if user.Role == auth.RoleAdmin {
		return true, nil
	}

	perms, ok, err := s.permissionsForPath(ctx, user.ID, rawPath)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	return perms.Read, nil
}

// CanCreate reports whether role/user may create under the given media path.
func (s *Service) CanCreate(
	ctx context.Context,
	user auth.PublicUser,
	rawPath string,
) (bool, error) {
	return s.checkPermission(
		ctx,
		user,
		rawPath,
		func(p LibraryPermissions) bool { return p.Create },
	)
}

// CanUpdate reports whether role/user may update the given media path.
func (s *Service) CanUpdate(
	ctx context.Context,
	user auth.PublicUser,
	rawPath string,
) (bool, error) {
	return s.checkPermission(
		ctx,
		user,
		rawPath,
		func(p LibraryPermissions) bool { return p.Update },
	)
}

// CanDelete reports whether role/user may delete the given media path.
func (s *Service) CanDelete(
	ctx context.Context,
	user auth.PublicUser,
	rawPath string,
) (bool, error) {
	return s.checkPermission(
		ctx,
		user,
		rawPath,
		func(p LibraryPermissions) bool { return p.Delete },
	)
}

// CanBrowse reports whether the user may open a browse view for rawPath.
// Media root is always allowed (filtering yields an empty list without can_read grants).
func (s *Service) CanBrowse(
	ctx context.Context,
	user auth.PublicUser,
	rawPath string,
) (bool, error) {
	if user.Role == auth.RoleAdmin {
		return true, nil
	}

	canonical, err := CanonicalRelPath(rawPath)
	if err != nil {
		return false, err
	}
	if canonical == "" {
		return true, nil
	}

	return s.CanRead(ctx, user, canonical)
}

// FilterBrowseResponse removes unassigned folders and entries the user cannot read.
// Media root always returns 200 with an empty children list when no can_read grants exist.
func (s *Service) FilterBrowseResponse(
	ctx context.Context,
	user auth.PublicUser,
	response mediafs.BrowseResponse,
) (mediafs.BrowseResponse, error) {
	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return mediafs.BrowseResponse{}, fmt.Errorf("list libraries: %w", err)
	}

	filtered := make([]mediafs.Item, 0, len(response.Folder.Children))

	for _, child := range response.Folder.Children {
		if _, ok := MatchLibrary(libraries, child.Path); !ok {
			continue
		}
		if user.Role == auth.RoleAdmin {
			filtered = append(filtered, child)

			continue
		}

		allowed, readErr := s.CanRead(ctx, user, child.Path)
		if readErr != nil {
			return mediafs.BrowseResponse{}, readErr
		}
		if allowed {
			filtered = append(filtered, child)
		}
	}

	response.Folder.Children = filtered

	return response, nil
}

// ListLibraries returns registered libraries.
func (s *Service) ListLibraries(ctx context.Context) ([]Library, error) {
	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	return libraries, nil
}

// GetLibrary returns a registered library by ID.
func (s *Service) GetLibrary(ctx context.Context, libraryID string) (Library, error) {
	library, err := s.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return Library{}, fmt.Errorf("get library: %w", err)
	}

	return library, nil
}

// LibraryForRelPath returns the library that owns rawPath, if any.
func (s *Service) LibraryForRelPath(
	ctx context.Context,
	rawPath string,
) (Library, bool, error) {
	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return Library{}, false, fmt.Errorf("list libraries: %w", err)
	}

	library, ok := MatchLibrary(libraries, rawPath)

	return library, ok, nil
}

// CreateLibrary registers an empty library (roots added separately).
func (s *Service) CreateLibrary(
	ctx context.Context,
	name, slug string,
	libraryTypeRaw string,
) (Library, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return Library{}, ErrInvalidLibraryName
	}

	libraryType := LibraryTypeOther
	if strings.TrimSpace(libraryTypeRaw) != "" {
		parsed, err := ParseLibraryType(libraryTypeRaw)
		if err != nil {
			return Library{}, err
		}
		libraryType = parsed
	}

	trimmedSlug := strings.TrimSpace(slug)
	if trimmedSlug == "" {
		trimmedSlug = slugify(trimmedName)
	} else {
		trimmedSlug = slugify(trimmedSlug)
	}
	if trimmedSlug == "" {
		return Library{}, ErrInvalidLibraryName
	}

	library, err := s.store.CreateLibrary(ctx, Library{
		Slug: trimmedSlug,
		Name: trimmedName,
		Type: libraryType,
	})
	if err != nil {
		return Library{}, fmt.Errorf("create library: %w", err)
	}

	return library, nil
}

// AddRoot attaches a media-relative folder to a library (exclusive).
func (s *Service) AddRoot( //nolint:cyclop // exclusive merge vs overlap is one decision tree
	ctx context.Context,
	libraryID, relPath string,
) (Library, error) {
	cleaned, err := validateRootPath(relPath)
	if err != nil {
		return Library{}, err
	}

	_, err = s.store.GetLibrary(ctx, libraryID)
	if err != nil {
		return Library{}, fmt.Errorf("add library root: %w", err)
	}

	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return Library{}, fmt.Errorf("list libraries: %w", err)
	}

	for _, library := range libraries {
		for _, root := range library.RootPaths() {
			if root == cleaned {
				if library.ID == libraryID {
					return library, nil
				}

				merged, addErr := s.store.AddRoot(ctx, libraryID, cleaned)
				if addErr != nil {
					return Library{}, fmt.Errorf("add library root: %w", addErr)
				}

				return merged, nil
			}
			if RootsOverlap(root, cleaned) {
				return Library{}, ErrLibraryRootConflict
			}
		}
	}

	library, err := s.store.AddRoot(ctx, libraryID, cleaned)
	if err != nil {
		return Library{}, fmt.Errorf("add library root: %w", err)
	}

	return library, nil
}

// RemoveRoot detaches a folder from a library.
func (s *Service) RemoveRoot(ctx context.Context, libraryID, relPath string) (Library, error) {
	cleaned := NormalizeRelPath(relPath)
	if cleaned == "" {
		return Library{}, ErrInvalidLibraryRoot
	}

	library, err := s.store.RemoveRoot(ctx, libraryID, cleaned)
	if err != nil {
		return Library{}, fmt.Errorf("remove library root: %w", err)
	}

	return library, nil
}

// DeleteLibrary removes a library registry row without deleting media files.
func (s *Service) DeleteLibrary(ctx context.Context, libraryID string) error {
	err := s.store.DeleteLibrary(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("delete library: %w", err)
	}

	return nil
}

// SyncLibrariesResult summarizes a prune of missing library roots.
type SyncLibrariesResult struct {
	Libraries []Library `json:"libraries"`
	Added     int       `json:"added"`
	Removed   int       `json:"removed"`
	Kept      int       `json:"kept"`
}

// PruneMissingRoots drops roots whose directories disappeared.
// Libraries with no remaining roots are deleted. New folders are not auto-registered.
func (s *Service) PruneMissingRoots(
	ctx context.Context,
	mediaRoot string,
) (SyncLibrariesResult, error) {
	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return SyncLibrariesResult{}, fmt.Errorf("list libraries: %w", err)
	}

	var removed int
	keptLibraries := make([]Library, 0, len(libraries))
	for _, library := range libraries {
		pruned, dropped, pruneErr := s.pruneLibraryRoots(ctx, mediaRoot, library)
		if pruneErr != nil {
			return SyncLibrariesResult{}, pruneErr
		}
		removed += dropped
		if pruned.ID != "" {
			keptLibraries = append(keptLibraries, pruned)
		}
	}

	return SyncLibrariesResult{
		Libraries: keptLibraries,
		Added:     0,
		Removed:   removed,
		Kept:      len(keptLibraries),
	}, nil
}

// SyncLibraries prunes missing roots (name kept for maintenance callers).
func (s *Service) SyncLibraries(
	ctx context.Context,
	mediaRoot string,
) (SyncLibrariesResult, error) {
	return s.PruneMissingRoots(ctx, mediaRoot)
}

func (s *Service) pruneLibraryRoots( //nolint:funcorder // prune helper used by PruneMissingRoots
	ctx context.Context,
	mediaRoot string,
	library Library,
) (Library, int, error) {
	removed := 0
	current := library
	for _, root := range library.RootPaths() {
		abs := filepath.Join(mediaRoot, filepath.FromSlash(root))
		info, err := os.Stat(abs)
		if err == nil && info.IsDir() {
			continue
		}

		updated, removeErr := s.store.RemoveRoot(ctx, library.ID, root)
		if removeErr != nil {
			if errors.Is(removeErr, ErrLibraryNotFound) {
				return Library{}, removed + 1, nil
			}

			return Library{}, removed, fmt.Errorf("prune root %s: %w", root, removeErr)
		}
		removed++
		current = updated
	}

	return current, removed, nil
}

// GetUserGrants returns all libraries with the user's permissions.
func (s *Service) GetUserGrants(ctx context.Context, userID string) ([]UserGrantView, error) {
	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	grantMap, err := s.store.GetUserGrantMap(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load grants: %w", err)
	}

	views := make([]UserGrantView, 0, len(libraries))
	for _, library := range libraries {
		perms := grantMap[library.ID]

		views = append(views, UserGrantView{
			Library:     library,
			Permissions: perms,
		})
	}

	return views, nil
}

// ListReadableLibraries returns libraries the user may read.
func (s *Service) ListReadableLibraries(
	ctx context.Context,
	user auth.PublicUser,
) ([]Library, error) {
	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	if user.Role == auth.RoleAdmin {
		return libraries, nil
	}

	grantMap, err := s.store.GetUserGrantMap(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("load grants: %w", err)
	}

	readable := make([]Library, 0, len(libraries))
	for _, library := range libraries {
		perms, found := grantMap[library.ID]
		if found && perms.Read {
			readable = append(readable, library)
		}
	}

	return readable, nil
}

// UpdateLibrary changes display name and/or UI type. Name is display-only (not used for paths/ACL).
func (s *Service) UpdateLibrary(
	ctx context.Context,
	libraryID string,
	name *string,
	libraryTypeRaw *string,
) (Library, error) {
	if name == nil && libraryTypeRaw == nil {
		return Library{}, ErrInvalidLibraryPatch
	}

	var trimmedName *string
	if name != nil {
		value := strings.TrimSpace(*name)
		if value == "" {
			return Library{}, ErrInvalidLibraryName
		}
		trimmedName = &value
	}

	var libraryType *LibraryType
	if libraryTypeRaw != nil {
		parsed, err := ParseLibraryType(*libraryTypeRaw)
		if err != nil {
			return Library{}, err
		}
		libraryType = &parsed
	}

	library, err := s.store.UpdateLibrary(ctx, libraryID, trimmedName, libraryType)
	if err != nil {
		return Library{}, fmt.Errorf("update library: %w", err)
	}

	return library, nil
}

// SetUserGrants replaces grants for a user.
func (s *Service) SetUserGrants(
	ctx context.Context,
	userID string,
	userRole string,
	grants []GrantInput,
) error {
	if userRole == auth.RoleAdmin {
		return ErrAdminGrantsImmutable
	}

	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return fmt.Errorf("list libraries: %w", err)
	}

	err = validateGrantInputs(libraries, grants)
	if err != nil {
		return err
	}

	err = s.store.ReplaceUserGrants(ctx, userID, grants)
	if err != nil {
		return fmt.Errorf("replace grants: %w", err)
	}

	return nil
}

func validateGrantInputs(libraries []Library, grants []GrantInput) error {
	knownLibraries := make(map[string]struct{}, len(libraries))
	for _, library := range libraries {
		knownLibraries[library.ID] = struct{}{}
	}

	for _, grant := range grants {
		if _, ok := knownLibraries[grant.LibraryID]; !ok {
			return ErrUnknownLibrary
		}
	}

	return nil
}

func (s *Service) checkPermission(
	ctx context.Context,
	user auth.PublicUser,
	rawPath string,
	check func(LibraryPermissions) bool,
) (bool, error) {
	if user.Role == auth.RoleAdmin {
		return true, nil
	}

	perms, ok, err := s.permissionsForPath(ctx, user.ID, rawPath)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	return check(perms), nil
}

func (s *Service) permissionsForPath(
	ctx context.Context,
	userID, rawPath string,
) (LibraryPermissions, bool, error) {
	canonical, err := CanonicalRelPath(rawPath)
	if err != nil {
		return LibraryPermissions{}, false, err
	}
	if canonical == "" {
		return LibraryPermissions{}, false, nil
	}

	libraries, err := s.store.ListLibraries(ctx)
	if err != nil {
		return LibraryPermissions{}, false, fmt.Errorf("list libraries: %w", err)
	}

	library, ok := MatchLibrary(libraries, canonical)
	if !ok {
		return LibraryPermissions{}, false, nil
	}

	grants, err := s.store.GetUserGrantMap(ctx, userID)
	if err != nil {
		return LibraryPermissions{}, false, fmt.Errorf("load grants: %w", err)
	}

	perms, found := grants[library.ID]
	if !found {
		return LibraryPermissions{}, false, nil
	}

	return perms, true, nil
}

func validateRootPath(relPath string) (string, error) {
	cleaned := NormalizeRelPath(relPath)
	if cleaned == "" || cleaned == ".." || strings.Contains(cleaned, "/../") {
		return "", ErrInvalidLibraryRoot
	}
	if IsHiddenRelPath(cleaned) || IsTrashRelPath(cleaned) {
		return "", ErrInvalidLibraryRoot
	}

	return cleaned, nil
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	prevDash := false
	for _, runeValue := range value {
		if unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) {
			builder.WriteRune(runeValue)
			prevDash = false

			continue
		}
		if !prevDash && builder.Len() > 0 {
			builder.WriteByte('-')
			prevDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}
