package mediafs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrDeleteNotEmpty is returned when deleting a non-empty directory without recursive=true.
var ErrDeleteNotEmpty = errors.New("directory is not empty")

// ErrDeleteReadOnly is returned when the media root or parent directory is not writable.
var ErrDeleteReadOnly = errors.New("media path is not writable")

// Delete removes a file or directory under the media root.
// Non-empty directories require recursive=true.
func (s *Service) Delete(rawPath string, recursive bool) error {
	fullPath, relPath, err := s.resolve(rawPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("stat delete path: %w", err)
	}

	if info.IsDir() {
		entries, readErr := os.ReadDir(fullPath)
		if readErr != nil {
			return fmt.Errorf("read delete directory: %w", readErr)
		}

		if len(entries) > 0 && !recursive {
			return ErrDeleteNotEmpty
		}

		err = os.RemoveAll(fullPath)
		if err != nil {
			return deleteError(relPath, err)
		}

		return nil
	}

	err = os.Remove(fullPath)
	if err != nil {
		return deleteError(relPath, err)
	}

	return nil
}

func deleteError(relPath string, err error) error {
	if isReadOnlyFSError(err) {
		return fmt.Errorf("%w: %s", ErrDeleteReadOnly, relPath)
	}

	return fmt.Errorf("remove %s: %w", relPath, err)
}

func isReadOnlyFSError(err error) bool {
	if pathErr, ok := errors.AsType[*os.PathError](err); ok {
		err = pathErr.Err
	}

	return errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EROFS) ||
		errors.Is(err, syscall.EACCES)
}

// DeletePrefix removes all cached artifacts under relPath prefix (best effort).
func DeletePrefix(root, relPath string) {
	if root == "" || relPath == "" {
		return
	}

	prefix := filepath.Join(root, relPath)

	_ = os.RemoveAll(prefix)
}
