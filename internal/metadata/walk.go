package metadata

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sudoStream/internal/mediafs"
)

// ListVideoPaths returns media-root-relative paths of every indexable video file under roots.
// Hidden directories are skipped. Shared by catalog views, provider enrichment, and anything
// else that needs "the video files of this library" without re-implementing the walk.
//
//nolint:cyclop // directory walk filters hidden dirs and non-video files
func ListVideoPaths(media *mediafs.Service, roots []string) ([]string, error) {
	if media == nil {
		return nil, nil
	}

	var paths []string

	for _, libraryRel := range roots {
		rootPath, err := media.DirPath(libraryRel)
		if err != nil {
			return nil, fmt.Errorf("resolve library dir: %w", err)
		}

		walkErr := filepath.WalkDir(
			rootPath,
			func(absPath string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					if strings.HasPrefix(entry.Name(), ".") && absPath != rootPath {
						return filepath.SkipDir
					}

					return nil
				}
				if !IsVideoExtension(filepath.Ext(entry.Name())) {
					return nil
				}
				rel, relErr := filepath.Rel(media.Root(), absPath)
				if relErr != nil {
					return fmt.Errorf("rel path: %w", relErr)
				}
				paths = append(paths, filepath.ToSlash(rel))

				return nil
			},
		)
		if walkErr != nil {
			return nil, fmt.Errorf("walk library: %w", walkErr)
		}
	}

	return paths, nil
}
