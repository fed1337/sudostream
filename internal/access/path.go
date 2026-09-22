package access

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	trashDirName      = ".trash"
	maxUnescapeRounds = 2
)

// ErrInvalidPath is returned when a media-relative path cannot be canonicalized safely.
var ErrInvalidPath = errors.New("invalid path")

// NormalizeRelPath returns a media-relative slash path without a leading slash.
// It does not URL-unescape or collapse ".." — use CanonicalRelPath for request paths.
func NormalizeRelPath(rawPath string) string {
	cleaned := strings.TrimSpace(rawPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = filepath.ToSlash(cleaned)
	if cleaned == "" || cleaned == "." {
		return ""
	}

	return cleaned
}

// CanonicalRelPath returns a cleaned media-relative path for ACL checks.
// It PathUnescapes (at most twice), rejects null bytes and residual %2e/%2f encodings,
// then filepath.Cleans so "lib/..%2fother/file" becomes "other/file".
func CanonicalRelPath(rawPath string) (string, error) {
	trimmed := NormalizeRelPath(rawPath)
	if trimmed == "" {
		return "", nil
	}

	pathValue, err := unescapePathLimited(trimmed)
	if err != nil {
		return "", err
	}

	relPath := cleanMediaRelPath(pathValue)
	if relPath == ".." || strings.HasPrefix(relPath, "../") || strings.Contains(relPath, "/../") {
		return "", ErrInvalidPath
	}

	if relPath != trimmed {
		slog.Info(
			"access.path_canonicalized",
			"raw", trimmed,
			"canonical", relPath,
		)
	}

	return relPath, nil
}

func unescapePathLimited(pathValue string) (string, error) {
	current := pathValue
	for range maxUnescapeRounds {
		unescaped, err := url.PathUnescape(current)
		if err != nil {
			return "", ErrInvalidPath
		}
		if unescaped == current {
			break
		}
		current = unescaped
	}
	if hasResidualPathEncoding(current) || strings.Contains(current, "\x00") {
		return "", ErrInvalidPath
	}

	return current, nil
}

func cleanMediaRelPath(pathValue string) string {
	pathValue = filepath.FromSlash(pathValue)
	cleanPath := filepath.Clean(string(os.PathSeparator) + pathValue)
	if cleanPath == "." || cleanPath == string(os.PathSeparator) {
		return ""
	}

	return filepath.ToSlash(strings.TrimPrefix(cleanPath, string(os.PathSeparator)))
}

func hasResidualPathEncoding(value string) bool {
	lower := strings.ToLower(value)

	return strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f")
}

// RootPaths returns registered folder roots, falling back to RelPath for fixtures.
func (l Library) RootPaths() []string {
	if len(l.Roots) > 0 {
		return l.Roots
	}
	if strings.TrimSpace(l.RelPath) != "" {
		return []string{l.RelPath}
	}

	return nil
}

// PathWithinRoot reports whether relPath is root or a descendant of root.
func PathWithinRoot(relPath, root string) bool {
	relPath = NormalizeRelPath(relPath)
	root = NormalizeRelPath(root)
	if relPath == "" || root == "" {
		return false
	}
	if relPath == root {
		return true
	}

	return strings.HasPrefix(relPath, root+"/")
}

// RootsOverlap reports whether two roots are equal or nested.
func RootsOverlap(left, right string) bool {
	return PathWithinRoot(left, right) || PathWithinRoot(right, left)
}

// IsHiddenRelPath reports whether a media path is under a dot-directory (including .trash).
func IsHiddenRelPath(relPath string) bool {
	relPath = NormalizeRelPath(relPath)
	if relPath == "" {
		return false
	}
	for part := range strings.SplitSeq(relPath, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}

	return false
}

// IsTrashRelPath reports whether the path is the trash directory or inside it.
func IsTrashRelPath(relPath string) bool {
	relPath = NormalizeRelPath(relPath)

	return relPath == trashDirName || strings.HasPrefix(relPath, trashDirName+"/")
}

// MatchLibrary returns the library with the longest matching root prefix.
// Request paths are canonicalized so encoded ".." cannot spoof a granted root.
func MatchLibrary(libraries []Library, rawPath string) (Library, bool) {
	relPath, err := CanonicalRelPath(rawPath)
	if err != nil || relPath == "" {
		return Library{}, false
	}

	bestLen := -1
	var best Library
	for _, library := range libraries {
		for _, root := range library.RootPaths() {
			if PathWithinRoot(relPath, root) && len(root) > bestLen {
				best = library
				bestLen = len(root)
			}
		}
	}

	return best, bestLen >= 0
}
