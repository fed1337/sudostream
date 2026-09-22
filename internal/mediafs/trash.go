package mediafs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// TrashDirName is the media-root folder that holds recycled items.
	TrashDirName  = ".trash"
	trashInfoName = "info.json"
	dirPerm       = 0o750
	filePerm      = 0o600
)

var (
	// ErrTrashForbidden is returned when deleting the media root or anything under .trash.
	ErrTrashForbidden = errors.New("cannot delete trash or media root")
	// ErrRestoreConflict is returned when the original path already exists.
	ErrRestoreConflict = errors.New("restore destination exists")
	// ErrTrashItemMissing is returned when a trash item directory is gone.
	ErrTrashItemMissing = errors.New("trash item not found")
)

// TrashInfo is persisted beside a recycled tree so a lost DB row can be recovered.
type TrashInfo struct {
	ID              string `json:"id"`
	OriginalRelPath string `json:"originalRelPath"`
	TrashRelPath    string `json:"trashRelPath"`
	LibraryID       string `json:"libraryId,omitempty"`
	DeletedAt       string `json:"deletedAt"`
	DeletedBy       string `json:"deletedBy,omitempty"`
}

// MoveToTrash relocates a file or directory under .trash/<itemID>/<original rel path>.
func (s *Service) MoveToTrash(rawPath, itemID string) (string, string, error) {
	err := validateTrashItemID(itemID)
	if err != nil {
		return "", "", err
	}

	fullPath, relPath, err := s.resolve(rawPath)
	if err != nil {
		return "", "", err
	}
	if relPath == "" || isTrashRelPath(relPath) {
		return "", "", ErrTrashForbidden
	}

	_, err = os.Stat(fullPath)
	if err != nil {
		return "", "", fmt.Errorf("stat trash source: %w", err)
	}

	trashRel := path.Join(TrashDirName, itemID, relPath)
	destFull, _, err := s.resolve(trashRel)
	if err != nil {
		return "", "", err
	}

	err = os.MkdirAll(filepath.Dir(destFull), dirPerm)
	if err != nil {
		return "", "", fmt.Errorf("create trash parent: %w", err)
	}

	err = os.Rename(fullPath, destFull)
	if err != nil {
		return "", "", deleteError(relPath, err)
	}

	return relPath, trashRel, nil
}

// RestoreFromTrash moves a trash tree back to originalRel. Collision returns ErrRestoreConflict.
func (s *Service) RestoreFromTrash(originalRel, trashRel, itemID string) error {
	err := validateTrashItemID(itemID)
	if err != nil {
		return err
	}

	originalRel = filepath.ToSlash(strings.TrimPrefix(originalRel, "/"))
	trashRel = filepath.ToSlash(strings.TrimPrefix(trashRel, "/"))
	if originalRel == "" || isTrashRelPath(originalRel) || !isTrashRelPath(trashRel) {
		return ErrInvalidPath
	}

	srcFull, destFull, err := s.resolveRestorePaths(originalRel, trashRel)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(destFull), dirPerm)
	if err != nil {
		return fmt.Errorf("create restore parent: %w", err)
	}

	err = os.Rename(srcFull, destFull)
	if err != nil {
		return deleteError(originalRel, err)
	}

	return s.RemoveTrashItem(itemID)
}

func (s *Service) resolveRestorePaths(originalRel, trashRel string) (string, string, error) {
	srcFull, _, err := s.resolve(trashRel)
	if err != nil {
		return "", "", err
	}
	_, err = os.Stat(srcFull)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", ErrTrashItemMissing
		}

		return "", "", fmt.Errorf("stat trash item: %w", err)
	}

	destFull, _, err := s.resolve(originalRel)
	if err != nil {
		return "", "", err
	}
	_, err = os.Stat(destFull)
	if err == nil {
		return "", "", ErrRestoreConflict
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("stat restore destination: %w", err)
	}

	return srcFull, destFull, nil
}

// RemoveTrashItem deletes .trash/<itemID> including info.json.
func (s *Service) RemoveTrashItem(itemID string) error {
	err := validateTrashItemID(itemID)
	if err != nil {
		return err
	}

	fullPath, _, err := s.resolve(path.Join(TrashDirName, itemID))
	if err != nil {
		return err
	}

	err = os.RemoveAll(fullPath)
	if err != nil {
		return fmt.Errorf("remove trash item: %w", err)
	}

	return nil
}

// WriteTrashInfo stores info.json under .trash/<itemID>/.
func (s *Service) WriteTrashInfo(itemID string, info TrashInfo) error {
	err := validateTrashItemID(itemID)
	if err != nil {
		return err
	}

	dirFull, _, err := s.resolve(path.Join(TrashDirName, itemID))
	if err != nil {
		return err
	}
	err = os.MkdirAll(dirFull, dirPerm)
	if err != nil {
		return fmt.Errorf("create trash item dir: %w", err)
	}

	payload, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("encode trash info: %w", err)
	}

	infoPath := filepath.Join(dirFull, trashInfoName)
	// #nosec G304 -- infoPath is under media root after Service.resolve
	err = os.WriteFile(infoPath, payload, filePerm)
	if err != nil {
		return fmt.Errorf("write trash info: %w", err)
	}

	return nil
}

// ReadTrashInfo loads info.json for a trash item.
func (s *Service) ReadTrashInfo(itemID string) (TrashInfo, error) {
	err := validateTrashItemID(itemID)
	if err != nil {
		return TrashInfo{}, err
	}

	dirFull, _, err := s.resolve(path.Join(TrashDirName, itemID))
	if err != nil {
		return TrashInfo{}, err
	}

	infoPath := filepath.Join(dirFull, trashInfoName)
	// #nosec G304 -- infoPath is under media root after Service.resolve
	payload, err := os.ReadFile(infoPath)
	if err != nil {
		return TrashInfo{}, fmt.Errorf("read trash info: %w", err)
	}

	var info TrashInfo
	err = json.Unmarshal(payload, &info)
	if err != nil {
		return TrashInfo{}, fmt.Errorf("decode trash info: %w", err)
	}

	return info, nil
}

// ListTrashItemIDs returns UUID directory names under .trash.
func (s *Service) ListTrashItemIDs() ([]string, error) {
	dirFull, _, err := s.resolve(TrashDirName)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dirFull)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("read trash dir: %w", err)
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if validateTrashItemID(entry.Name()) != nil {
			continue
		}
		ids = append(ids, entry.Name())
	}

	return ids, nil
}

func isTrashRelPath(relPath string) bool {
	relPath = filepath.ToSlash(strings.TrimPrefix(relPath, "/"))

	return relPath == TrashDirName || strings.HasPrefix(relPath, TrashDirName+"/")
}

func validateTrashItemID(itemID string) error {
	if itemID == "" || strings.Contains(itemID, "/") || strings.Contains(itemID, "..") {
		return ErrInvalidPath
	}
	if strings.Contains(itemID, string(os.PathSeparator)) {
		return ErrInvalidPath
	}

	return nil
}
