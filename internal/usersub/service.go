// Package usersub stores per-user uploaded WebVTT captions (TTL 1d, replace per path).
// Files live under the cache root — never under /media.
package usersub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ttl            = 24 * time.Hour
	metaSuffix     = ".json"
	vttSuffix      = ".vtt"
	defaultLang    = "und"
	defaultLabel   = "Uploaded"
	dirPerm        = 0o750
	filePerm       = 0o600
	maxUploadBytes = 2 << 20 // 2 MiB
)

var (
	// ErrNotFound is returned when no non-expired subtitle exists for the user+path.
	ErrNotFound = errors.New("user subtitle not found")
	// ErrTooLarge is returned when the upload exceeds MaxUploadBytes.
	ErrTooLarge         = errors.New("user subtitle too large")
	errCacheDirRequired = errors.New("usersub cache dir required")
)

// MaxUploadBytes is the accepted upload size ceiling.
const MaxUploadBytes = maxUploadBytes

// Track is exposed on playback / list responses.
type Track struct {
	Lang      string    `json:"lang"`
	Label     string    `json:"label"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type metaFile struct {
	RelPath   string    `json:"relPath"`
	Lang      string    `json:"lang"`
	Label     string    `json:"label"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Service persists user subtitle VTT files beside JSON metadata.
type Service struct {
	root string
}

// NewService creates a usersub service rooted at cacheDir (e.g. cache/user-subs).
func NewService(cacheDir string) (*Service, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return nil, errCacheDirRequired
	}

	err := os.MkdirAll(cacheDir, dirPerm)
	if err != nil {
		return nil, fmt.Errorf("mkdir usersub cache: %w", err)
	}

	return &Service{root: cacheDir}, nil
}

// Put replaces any existing subtitle for user+path with VTT bytes (TTL 1d).
func (s *Service) Put( //nolint:cyclop // validate + write meta/vtt is one use case
	userID, relPath, lang, label string,
	vtt []byte,
) (Track, error) {
	if s == nil {
		return Track{}, ErrNotFound
	}
	if len(vtt) == 0 {
		return Track{}, fmt.Errorf("%w: empty body", ErrNotFound)
	}
	if len(vtt) > MaxUploadBytes {
		return Track{}, ErrTooLarge
	}

	relPath = normalizePath(relPath)
	userID = strings.TrimSpace(userID)
	if userID == "" || relPath == "" {
		return Track{}, fmt.Errorf("%w: missing user or path", ErrNotFound)
	}

	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		lang = defaultLang
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = defaultLabel
	}

	dir := s.userDir(userID)
	err := os.MkdirAll(dir, dirPerm)
	if err != nil {
		return Track{}, fmt.Errorf("mkdir user subtitle dir: %w", err)
	}

	base := s.baseName(relPath)
	vttPath := filepath.Join(dir, base+vttSuffix)
	metaPath := filepath.Join(dir, base+metaSuffix)
	expires := time.Now().UTC().Add(ttl)
	meta := metaFile{
		RelPath:   relPath,
		Lang:      lang,
		Label:     label,
		ExpiresAt: expires,
	}

	rawMeta, err := json.Marshal(meta)
	if err != nil {
		return Track{}, fmt.Errorf("marshal user subtitle meta: %w", err)
	}

	err = os.WriteFile(vttPath, vtt, filePerm)
	if err != nil {
		return Track{}, fmt.Errorf("write user subtitle vtt: %w", err)
	}

	err = os.WriteFile(metaPath, rawMeta, filePerm)
	if err != nil {
		_ = os.Remove(vttPath)

		return Track{}, fmt.Errorf("write user subtitle meta: %w", err)
	}

	return Track{Lang: lang, Label: label, ExpiresAt: expires}, nil
}

// Get returns track metadata and absolute VTT path when present and not expired.
func (s *Service) Get(userID, relPath string) (Track, string, error) {
	if s == nil {
		return Track{}, "", ErrNotFound
	}

	relPath = normalizePath(relPath)
	meta, vttPath, err := s.readMeta(userID, relPath)
	if err != nil {
		return Track{}, "", err
	}
	if time.Now().UTC().After(meta.ExpiresAt) {
		_ = s.Delete(userID, relPath)

		return Track{}, "", ErrNotFound
	}

	return Track{
		Lang:      meta.Lang,
		Label:     meta.Label,
		ExpiresAt: meta.ExpiresAt,
	}, vttPath, nil
}

// Delete removes the subtitle for user+path (idempotent).
func (s *Service) Delete(userID, relPath string) error {
	if s == nil {
		return nil
	}

	dir := s.userDir(userID)
	base := s.baseName(normalizePath(relPath))
	_ = os.Remove(filepath.Join(dir, base+vttSuffix))
	_ = os.Remove(filepath.Join(dir, base+metaSuffix))

	return nil
}

func (s *Service) readMeta(userID, relPath string) (metaFile, string, error) {
	dir := s.userDir(userID)
	base := s.baseName(relPath)
	metaPath := filepath.Join(dir, base+metaSuffix)
	vttPath := filepath.Join(dir, base+vttSuffix)

	raw, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return metaFile{}, "", ErrNotFound
		}

		return metaFile{}, "", fmt.Errorf("read user subtitle meta: %w", err)
	}

	var meta metaFile
	err = json.Unmarshal(raw, &meta)
	if err != nil {
		return metaFile{}, "", fmt.Errorf("parse user subtitle meta: %w", err)
	}

	_, statErr := os.Stat(vttPath)
	if statErr != nil {
		return metaFile{}, "", ErrNotFound
	}

	return meta, vttPath, nil
}

func (s *Service) userDir(userID string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}

		return '_'
	}, userID)

	return filepath.Join(s.root, safe)
}

func (s *Service) baseName(relPath string) string {
	sum := sha256.Sum256([]byte(relPath))

	return hex.EncodeToString(sum[:])
}

func normalizePath(raw string) string {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = filepath.ToSlash(cleaned)

	return cleaned
}
