// Package postgres implements access persistence with PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sudoStream/internal/access"

	"gorm.io/gorm"
)

const (
	mediaMetadataTable = "media_metadata"
	watchStateTable    = "watch_state"
	favoritesTable     = "favorites"
)

// Store persists libraries and grants.
type Store struct {
	db *gorm.DB
}

// NewStore constructs a PostgreSQL access store.
func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) preloadLibraries( //nolint:funcorder // used by List/Get
	db *gorm.DB,
) *gorm.DB {
	return db.Preload("Roots", func(query *gorm.DB) *gorm.DB {
		return query.Order("rel_path ASC")
	})
}

// ListLibraries returns all registered libraries.
func (s *Store) ListLibraries(ctx context.Context) ([]access.Library, error) {
	var models []libraryModel

	err := s.preloadLibraries(s.db.WithContext(ctx)).
		Order("name ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	libraries := make([]access.Library, 0, len(models))
	for _, model := range models {
		libraries = append(libraries, libraryFromModel(model))
	}

	return libraries, nil
}

// GetLibrary returns a library by ID.
func (s *Store) GetLibrary(ctx context.Context, libraryID string) (access.Library, error) {
	var model libraryModel

	err := s.preloadLibraries(s.db.WithContext(ctx)).First(&model, "id = ?", libraryID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return access.Library{}, access.ErrLibraryNotFound
		}

		return access.Library{}, fmt.Errorf("get library: %w", err)
	}

	return libraryFromModel(model), nil
}

// CreateLibrary inserts a library with no roots.
func (s *Store) CreateLibrary(ctx context.Context, library access.Library) (access.Library, error) {
	if library.Type == "" {
		library.Type = access.LibraryTypeOther
	}

	slug, err := s.allocateSlug(ctx, s.db, library.Slug)
	if err != nil {
		return access.Library{}, err
	}

	model := libraryModel{
		Slug: slug,
		Name: library.Name,
		Type: library.Type,
	}

	err = s.db.WithContext(ctx).Omit("ID", "Roots").Create(&model).Error
	if err != nil {
		return access.Library{}, fmt.Errorf("create library: %w", err)
	}

	return s.GetLibrary(ctx, model.ID)
}

// UpsertLibrary creates a library for RelPath or returns the library that already owns that root.
func (s *Store) UpsertLibrary(ctx context.Context, library access.Library) (access.Library, error) {
	root := access.NormalizeRelPath(library.RelPath)
	if root == "" && len(library.Roots) > 0 {
		root = access.NormalizeRelPath(library.Roots[0])
	}
	if root == "" {
		return access.Library{}, access.ErrInvalidLibraryRoot
	}

	existing, err := s.libraryIDForExactRoot(ctx, s.db, root)
	if err != nil {
		return access.Library{}, err
	}
	if existing != "" {
		return s.GetLibrary(ctx, existing)
	}

	if library.Slug == "" {
		library.Slug = root
	}
	if library.Name == "" {
		library.Name = root
	}

	created, err := s.CreateLibrary(ctx, library)
	if err != nil {
		return access.Library{}, err
	}

	return s.AddRoot(ctx, created.ID, root)
}

// DeleteLibrary removes a library by ID (CASCADE metadata, grants, watch).
func (s *Store) DeleteLibrary(ctx context.Context, libraryID string) error {
	result := s.db.WithContext(ctx).Delete(&libraryModel{}, "id = ?", libraryID)
	if result.Error != nil {
		return fmt.Errorf("delete library: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return access.ErrLibraryNotFound
	}

	return nil
}

// UpdateLibrary sets display name and/or UI type for a library.
func (s *Store) UpdateLibrary(
	ctx context.Context,
	libraryID string,
	name *string,
	libraryType *access.LibraryType,
) (access.Library, error) {
	updates := map[string]any{}
	if name != nil {
		updates["name"] = *name
	}
	if libraryType != nil {
		updates["type"] = *libraryType
	}
	if len(updates) == 0 {
		return access.Library{}, access.ErrInvalidLibraryPatch
	}

	result := s.db.WithContext(ctx).
		Model(&libraryModel{}).
		Where("id = ?", libraryID).
		Updates(updates)
	if result.Error != nil {
		return access.Library{}, fmt.Errorf("update library: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return access.Library{}, access.ErrLibraryNotFound
	}

	return s.GetLibrary(ctx, libraryID)
}

// AddRoot attaches relPath to libraryID, merging another library that owns the exact root.
func (s *Store) AddRoot( //nolint:cyclop,gocognit // exclusive merge + reassign in one transaction
	ctx context.Context,
	libraryID, relPath string,
) (access.Library, error) {
	err := s.db.WithContext(ctx).Transaction(func(txn *gorm.DB) error {
		dest, destErr := s.getLibraryTx(txn, libraryID)
		if destErr != nil {
			return destErr
		}

		for _, existing := range dest.RootPaths() {
			if existing == relPath {
				return nil
			}
			if access.PathWithinRoot(relPath, existing) {
				return access.ErrLibraryRootConflict
			}
		}

		ownerID, ownerErr := s.libraryIDForExactRoot(ctx, txn, relPath)
		if ownerErr != nil {
			return ownerErr
		}
		if ownerID != "" && ownerID != libraryID { //nolint:nestif // merge existing root owner
			reassignErr := s.reassignPrefix(txn, ownerID, libraryID, relPath)
			if reassignErr != nil {
				return reassignErr
			}
			delErr := txn.Where("library_id = ? AND rel_path = ?", ownerID, relPath).
				Delete(&libraryRootModel{}).Error
			if delErr != nil {
				return fmt.Errorf("move root: %w", delErr)
			}
			emptyErr := s.deleteLibraryIfEmpty(txn, ownerID)
			if emptyErr != nil {
				return emptyErr
			}
		} else if ownerID == "" {
			conflict, conflictErr := s.conflictingLibraryID(txn, libraryID, relPath)
			if conflictErr != nil {
				return conflictErr
			}
			if conflict != "" {
				return access.ErrLibraryRootConflict
			}
		}

		for _, existing := range dest.RootPaths() {
			if access.PathWithinRoot(existing, relPath) && existing != relPath {
				delErr := txn.Where("library_id = ? AND rel_path = ?", libraryID, existing).
					Delete(&libraryRootModel{}).Error
				if delErr != nil {
					return fmt.Errorf("subsume child root: %w", delErr)
				}
			}
		}

		insertErr := txn.Create(&libraryRootModel{LibraryID: libraryID, RelPath: relPath}).Error
		if insertErr != nil {
			return fmt.Errorf("insert library root: %w", insertErr)
		}

		return nil
	})
	if err != nil {
		return access.Library{}, fmt.Errorf("add library root: %w", err)
	}

	return s.GetLibrary(ctx, libraryID)
}

// RemoveRoot detaches relPath and deletes prefix metadata/watch/favorites for that folder.
func (s *Store) RemoveRoot(
	ctx context.Context,
	libraryID, relPath string,
) (access.Library, error) {
	var remaining int64

	err := s.db.WithContext(ctx).Transaction(func(txn *gorm.DB) error {
		result := txn.Where("library_id = ? AND rel_path = ?", libraryID, relPath).
			Delete(&libraryRootModel{})
		if result.Error != nil {
			return fmt.Errorf("delete library root: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return access.ErrLibraryRootNotFound
		}

		delErr := s.deletePrefixRows(txn, libraryID, relPath)
		if delErr != nil {
			return delErr
		}

		countErr := txn.Model(&libraryRootModel{}).
			Where("library_id = ?", libraryID).
			Count(&remaining).Error
		if countErr != nil {
			return fmt.Errorf("count roots: %w", countErr)
		}
		if remaining == 0 {
			return txn.Delete(&libraryModel{}, "id = ?", libraryID).Error
		}

		return nil
	})
	if err != nil {
		return access.Library{}, fmt.Errorf("remove library root: %w", err)
	}

	if remaining == 0 {
		return access.Library{}, access.ErrLibraryNotFound
	}

	return s.GetLibrary(ctx, libraryID)
}

// GetUserGrantMap returns library_id -> permissions for a user.
func (s *Store) GetUserGrantMap(
	ctx context.Context,
	userID string,
) (map[string]access.LibraryPermissions, error) {
	type grantRow struct {
		LibraryID string
		CanCreate bool
		CanRead   bool
		CanUpdate bool
		CanDelete bool
	}

	var rows []grantRow

	err := s.db.WithContext(ctx).
		Table("library_grants").
		Select("library_id, can_create, can_read, can_update, can_delete").
		Where("user_id = ?", userID).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("select grants: %w", err)
	}

	grants := make(map[string]access.LibraryPermissions, len(rows))
	for _, row := range rows {
		grants[row.LibraryID] = access.LibraryPermissions{
			Create: row.CanCreate,
			Read:   row.CanRead,
			Update: row.CanUpdate,
			Delete: row.CanDelete,
		}
	}

	return grants, nil
}

// ReplaceUserGrants replaces all grants for a user.
func (s *Store) ReplaceUserGrants(
	ctx context.Context,
	userID string,
	grants []access.GrantInput,
) error {
	err := s.db.WithContext(ctx).Transaction(func(txn *gorm.DB) error {
		err := txn.Where("user_id = ?", userID).Delete(&libraryGrantModel{}).Error
		if err != nil {
			return fmt.Errorf("clear grants: %w", err)
		}

		for _, grant := range grants {
			if grant.Permissions.IsEmpty() {
				continue
			}

			row := libraryGrantModel{
				UserID:    userID,
				LibraryID: grant.LibraryID,
				CanCreate: grant.Permissions.Create,
				CanRead:   grant.Permissions.Read,
				CanUpdate: grant.Permissions.Update,
				CanDelete: grant.Permissions.Delete,
			}

			err = txn.Create(&row).Error
			if err != nil {
				return fmt.Errorf("insert grant: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("replace grants: %w", err)
	}

	return nil
}

func (s *Store) getLibraryTx(txn *gorm.DB, libraryID string) (access.Library, error) {
	var model libraryModel

	err := s.preloadLibraries(txn).First(&model, "id = ?", libraryID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return access.Library{}, access.ErrLibraryNotFound
		}

		return access.Library{}, fmt.Errorf("get library: %w", err)
	}

	return libraryFromModel(model), nil
}

func (s *Store) libraryIDForExactRoot(
	ctx context.Context,
	db *gorm.DB,
	relPath string,
) (string, error) {
	_ = ctx

	var root libraryRootModel

	err := db.Where("rel_path = ?", relPath).First(&root).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lookup library root: %w", err)
	}

	return root.LibraryID, nil
}

func (s *Store) conflictingLibraryID(txn *gorm.DB, destID, relPath string) (string, error) {
	var roots []libraryRootModel

	err := txn.Find(&roots).Error
	if err != nil {
		return "", fmt.Errorf("list library roots: %w", err)
	}

	for _, root := range roots {
		if root.LibraryID == destID {
			continue
		}
		if access.RootsOverlap(root.RelPath, relPath) {
			return root.LibraryID, nil
		}
	}

	return "", nil
}

func (s *Store) deleteLibraryIfEmpty(txn *gorm.DB, libraryID string) error {
	var remaining int64

	err := txn.Model(&libraryRootModel{}).Where("library_id = ?", libraryID).Count(&remaining).Error
	if err != nil {
		return fmt.Errorf("count roots: %w", err)
	}
	if remaining > 0 {
		return nil
	}

	err = txn.Delete(&libraryModel{}, "id = ?", libraryID).Error
	if err != nil {
		return fmt.Errorf("delete emptied library: %w", err)
	}

	return nil
}

func (s *Store) reassignPrefix(txn *gorm.DB, srcID, destID, prefix string) error {
	like := prefix + "/%"

	err := txn.Exec(
		`UPDATE `+mediaMetadataTable+` AS src SET library_id = ?
		 WHERE src.library_id = ?
		   AND (src.rel_path = ? OR src.rel_path LIKE ?)
		   AND NOT EXISTS (
		     SELECT 1 FROM `+mediaMetadataTable+` dest
		     WHERE dest.library_id = ? AND dest.rel_path = src.rel_path
		   )`,
		destID, srcID, prefix, like, destID,
	).Error
	if err != nil {
		return fmt.Errorf("reassign metadata: %w", err)
	}

	for _, table := range []string{watchStateTable, favoritesTable} {
		err = txn.Exec(
			`UPDATE `+table+` AS src SET library_id = ?
			 WHERE src.library_id = ?
			   AND (src.rel_path = ? OR src.rel_path LIKE ?)
			   AND NOT EXISTS (
			     SELECT 1 FROM `+table+` dest
			     WHERE dest.library_id = ?
			       AND dest.user_id = src.user_id
			       AND dest.rel_path = src.rel_path
			   )`,
			destID, srcID, prefix, like, destID,
		).Error
		if err != nil {
			return fmt.Errorf("reassign %s: %w", table, err)
		}
	}

	for _, table := range []string{mediaMetadataTable, watchStateTable, favoritesTable} {
		err = txn.Exec(
			`DELETE FROM `+table+`
			 WHERE library_id = ? AND (rel_path = ? OR rel_path LIKE ?)`,
			srcID, prefix, like,
		).Error
		if err != nil {
			return fmt.Errorf("delete leftover %s: %w", table, err)
		}
	}

	return nil
}

func (s *Store) deletePrefixRows(txn *gorm.DB, libraryID, prefix string) error {
	like := prefix + "/%"
	for _, table := range []string{mediaMetadataTable, watchStateTable, favoritesTable} {
		err := txn.Exec(
			`DELETE FROM `+table+`
			 WHERE library_id = ? AND (rel_path = ? OR rel_path LIKE ?)`,
			libraryID, prefix, like,
		).Error
		if err != nil {
			return fmt.Errorf("delete prefix %s: %w", table, err)
		}
	}

	return nil
}

func (s *Store) allocateSlug(ctx context.Context, conn *gorm.DB, base string) (string, error) {
	_ = ctx
	slug := base
	if slug == "" {
		slug = "library"
	}

	var used []string

	err := conn.Model(&libraryModel{}).
		Where("slug = ? OR slug LIKE ?", slug, slug+"-%").
		Pluck("slug", &used).Error
	if err != nil {
		return "", fmt.Errorf("list slugs: %w", err)
	}

	taken := make(map[string]struct{}, len(used))
	for _, value := range used {
		taken[value] = struct{}{}
	}
	if _, exists := taken[slug]; !exists {
		return slug, nil
	}

	for index := 2; index < 1000; index++ {
		candidate := fmt.Sprintf("%s-%d", slug, index)
		if _, exists := taken[candidate]; !exists {
			return candidate, nil
		}
	}

	sort.Strings(used)

	return "", fmt.Errorf("%w: %s", access.ErrLibrarySlugConflict, strings.Join(used, ","))
}
