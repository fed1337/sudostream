// Package mediafs provides safe media filesystem browsing and file access.
package mediafs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	contentTypeSniffBytes = 512
	directoryMimeType     = "inode/directory"
	octetStreamMimeType   = "application/octet-stream"
	webmExtension         = ".webm"
	webmMimeType          = "video/webm"
	mediaRootName         = "media"
)

var (
	// ErrMediaRootNotDirectory is returned when the configured root is not a directory.
	ErrMediaRootNotDirectory = errors.New("media root is not a directory")
	// ErrInvalidPath is returned when a path cannot be parsed safely.
	ErrInvalidPath = errors.New("invalid path")
	// ErrPathOutsideRoot is returned when a path escapes the configured media root.
	ErrPathOutsideRoot = errors.New("path outside media root")
)

// Item describes a media filesystem entry.
type Item struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"isDir"`
	Size      int64  `json:"size"`
	Extension string `json:"extension"`
	MimeType  string `json:"mimeType"`
	Watched   *bool  `json:"watched,omitempty"`
	Favorited *bool  `json:"favorited,omitempty"`
	Actions   Action `json:"actions"`
	Children  []Item `json:"children,omitempty"`
}

// Action contains API links available for a media item.
type Action struct {
	Play      string `json:"play,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Download  string `json:"download"`
	CanDelete bool   `json:"canDelete,omitempty"`
}

// Breadcrumb describes one path segment in a browse response.
type Breadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// BrowseResponse contains metadata for a browsed media folder.
type BrowseResponse struct {
	Path        string       `json:"path"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
	Folder      Item         `json:"folder"`
	Total       int          `json:"total"`
	Limit       int          `json:"limit"`
	Offset      int          `json:"offset"`
}

// Service provides safe access to media files under a configured root.
type Service struct {
	root string
}

// New creates a media filesystem service rooted at root.
func New(root string) (*Service, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs media root: %w", err)
	}

	rootInfo, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat media root: %w", err)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrMediaRootNotDirectory, absRoot)
	}

	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve media root symlinks: %w", err)
	}

	return &Service{root: resolvedRoot}, nil
}

// Root returns the absolute media root path.
func (s *Service) Root() string {
	return s.root
}

// Browse returns folder metadata and immediate children (shallow) for a path under the media root.
// Callers apply ACL filtering then ApplyPage for limit/offset.
func (s *Service) Browse(rawPath string) (BrowseResponse, error) {
	fullPath, relPath, err := s.resolve(rawPath)
	if err != nil {
		return BrowseResponse{}, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return BrowseResponse{}, fmt.Errorf("stat browse path: %w", err)
	}
	if !info.IsDir() {
		return BrowseResponse{}, fs.ErrInvalid
	}

	node, err := s.buildTree(fullPath, relPath)
	if err != nil {
		return BrowseResponse{}, err
	}

	return BrowseResponse{
		Path:        relPath,
		Breadcrumbs: buildBreadcrumbs(relPath),
		Folder:      node,
		Total:       len(node.Children),
		Limit:       len(node.Children),
		Offset:      0,
	}, nil
}

// OpenFile opens a file under the media root and returns its metadata.
func (s *Service) OpenFile(rawPath string) (*os.File, os.FileInfo, error) {
	fullPath, _, err := s.resolve(rawPath)
	if err != nil {
		return nil, nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, nil, fmt.Errorf("stat media file: %w", err)
	}
	if info.IsDir() {
		return nil, nil, fs.ErrInvalid
	}

	// #nosec G304 -- fullPath is resolved and constrained to Service.root.
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open media file: %w", err)
	}

	return file, info, nil
}

// Health verifies the configured media root is still accessible.
func (s *Service) Health() error {
	_, err := os.Stat(s.root)
	if err != nil {
		return fmt.Errorf("stat media root: %w", err)
	}

	return nil
}

// FilePath returns the resolved filesystem path for a file under the media root.
func (s *Service) FilePath(rawPath string) (string, error) {
	fullPath, _, err := s.resolve(rawPath)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("stat media file path: %w", err)
	}
	if info.IsDir() {
		return "", fs.ErrInvalid
	}

	return fullPath, nil
}

// DirPath returns the resolved filesystem path for a directory under the media root.
func (s *Service) DirPath(rawPath string) (string, error) {
	fullPath, _, err := s.resolve(rawPath)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("stat media directory path: %w", err)
	}
	if !info.IsDir() {
		return "", fs.ErrInvalid
	}

	return fullPath, nil
}

func (s *Service) buildTree(dirPath, relPath string) (Item, error) {
	node := Item{
		Name:      folderName(relPath),
		Path:      relPath,
		IsDir:     true,
		Size:      0,
		Extension: "",
		MimeType:  directoryMimeType,
		Actions:   Action{Play: "", Thumbnail: "", Download: ""},
		Children:  []Item{},
	}

	children, err := s.buildChildren(dirPath, relPath)
	if err != nil {
		return Item{}, err
	}
	node.Children = children

	return node, nil
}

func (s *Service) buildChildren(dirPath, relPath string) ([]Item, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("read media directory: %w", err)
	}

	children := make([]Item, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		childRel := path.Join(relPath, name)
		childFull, _, err := s.resolve(childRel)
		if err != nil {
			continue
		}

		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}

		item := Item{
			Name:      name,
			Path:      childRel,
			IsDir:     entry.IsDir(),
			Size:      sizeOf(entryInfo),
			Extension: strings.ToLower(filepath.Ext(name)),
			MimeType:  "",
			Actions:   Action{Play: "", Thumbnail: "", Download: ""},
			Children:  nil,
		}
		item.MimeType = DetectMimeType(childFull, item.Extension, item.IsDir)
		item.Actions = buildActions(item.Path, item.MimeType, item.Extension, item.IsDir)

		children = append(children, item)
	}

	sort.SliceStable(children, func(i, j int) bool {
		if children[i].IsDir != children[j].IsDir {
			return children[i].IsDir
		}

		return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
	})

	return children, nil
}

func (s *Service) resolve(rawPath string) (string, string, error) {
	pathValue, err := url.PathUnescape(strings.TrimPrefix(rawPath, "/"))
	if err != nil {
		return "", "", ErrInvalidPath
	}
	if strings.Contains(pathValue, "\x00") {
		return "", "", ErrInvalidPath
	}

	pathValue = filepath.FromSlash(pathValue)
	cleanPath := filepath.Clean(string(os.PathSeparator) + pathValue)
	var relPath string
	if cleanPath == "." || cleanPath == string(os.PathSeparator) {
		relPath = ""
	} else {
		relPath = strings.TrimPrefix(cleanPath, string(os.PathSeparator))
	}

	candidate := filepath.Join(s.root, relPath)
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", fmt.Errorf("abs media candidate: %w", err)
	}
	if !isWithinRoot(s.root, absCandidate) {
		return "", "", ErrPathOutsideRoot
	}

	resolvedCandidate := absCandidate
	target, evalErr := filepath.EvalSymlinks(absCandidate)
	if evalErr == nil {
		resolvedCandidate = target
	} else if !errors.Is(evalErr, os.ErrNotExist) {
		return "", "", fmt.Errorf("resolve media candidate symlinks: %w", evalErr)
	}

	if !isWithinRoot(s.root, resolvedCandidate) {
		return "", "", ErrPathOutsideRoot
	}

	return resolvedCandidate, filepath.ToSlash(relPath), nil
}

func isWithinRoot(root, candidate string) bool {
	if candidate == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(os.PathSeparator)) {
		prefix += string(os.PathSeparator)
	}

	return strings.HasPrefix(candidate, prefix)
}

// DetectMimeType determines the MIME type of a file based on its extension or directory status.
func DetectMimeType(filePath, extension string, isDir bool) string {
	if isDir {
		return directoryMimeType
	}
	if extension != "" {
		if fromExt := mimeTypeFromExtension(extension); fromExt != "" {
			return fromExt
		}
	}

	// #nosec G304 -- filePath is produced by Service.resolve before this call path.
	f, err := os.Open(filePath)
	if err != nil {
		return octetStreamMimeType
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, contentTypeSniffBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return octetStreamMimeType
	}

	return http.DetectContentType(buf[:n])
}

func mimeTypeFromExtension(extension string) string {
	switch strings.ToLower(extension) {
	case webmExtension:
		// Go's mime.TypeByExtension maps .webm to audio/webm; treat library webm as video.
		return webmMimeType
	default:
		return mime.TypeByExtension(extension)
	}
}

// IsVideoExtension checks if a file extension corresponds to a known video format.
func IsVideoExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".mp4", ".mkv", webmExtension, ".avi", ".mov", ".m4v", ".wmv", ".mpg", ".mpeg":
		return true
	default:
		return false
	}
}

func buildActions(relPath, mimeType, extension string, isDir bool) Action {
	if isDir {
		return Action{Play: "", Thumbnail: "", Download: ""}
	}

	escaped := EscapePathSegments(relPath)
	action := Action{
		Thumbnail: "",
		Download:  "/api/download/" + escaped,
	}
	if strings.HasPrefix(mimeType, "video/") || IsVideoExtension(extension) {
		action.Play = "/api/play/" + escaped + "/master.m3u8"
		action.Thumbnail = "/api/thumbnail/" + escaped
	}
	if strings.HasPrefix(mimeType, "image/") {
		action.Thumbnail = "/api/thumbnail/" + escaped
	}

	return action
}

// EscapePathSegments URL-escapes each segment of a media relative path.
func EscapePathSegments(relPath string) string {
	return escapePathSegments(relPath)
}

func escapePathSegments(relPath string) string {
	if relPath == "" {
		return ""
	}
	parts := strings.Split(relPath, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}

	return strings.Join(parts, "/")
}

func buildBreadcrumbs(relPath string) []Breadcrumb {
	breadcrumbs := []Breadcrumb{{Name: mediaRootName, Path: ""}}
	if relPath == "" {
		return breadcrumbs
	}

	parts := strings.Split(relPath, "/")
	current := ""
	for _, part := range parts {
		if current == "" {
			current = part
		} else {
			current = path.Join(current, part)
		}
		breadcrumbs = append(breadcrumbs, Breadcrumb{Name: part, Path: current})
	}

	return breadcrumbs
}

func folderName(relPath string) string {
	if relPath == "" {
		return mediaRootName
	}
	parts := strings.Split(relPath, "/")

	return parts[len(parts)-1]
}

func sizeOf(info os.FileInfo) int64 {
	if info.IsDir() {
		return 0
	}

	return info.Size()
}
