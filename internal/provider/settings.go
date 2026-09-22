// Package provider implements FI-1 library metadata/poster/subtitle providers:
// per-library settings, the in-process adapter registry, and category interfaces.
// See plans/FI-1-providers.md for the full product brief.
package provider

import (
	"errors"
	"regexp"
	"time"
)

// Metadata apply modes (FI-1 L10) — how provider fields merge into non-skipped rows.
const (
	// ApplyModeFillMissing writes only whitelist fields that are empty in the effective merge (default).
	ApplyModeFillMissing = "fill_missing"
	// ApplyModeFullRewrite writes all mapped provider fields, overwriting effective values.
	ApplyModeFullRewrite = "full_rewrite"
)

// Metadata write targets (FI-1 L9) — where matched metadata fields are written.
const (
	// WriteTargetDB writes matched fields to per-file DB overrides (default).
	WriteTargetDB = "db"
	// WriteTargetFile writes matched fields as embedded file tags.
	WriteTargetFile = "file"
)

var (
	// ErrNotFound is returned by Store when no settings row exists for a library.
	ErrNotFound = errors.New("provider settings not found")
	// ErrInvalidMetadataProvider is returned when metadataProvider is not a registered metadata key.
	ErrInvalidMetadataProvider = errors.New("invalid metadata provider")
	// ErrInvalidPosterProvider is returned when posterProvider is not a registered poster key.
	ErrInvalidPosterProvider = errors.New("invalid poster provider")
	// ErrInvalidSubtitleProvider is returned when subtitleProvider is not a registered subtitle key.
	ErrInvalidSubtitleProvider = errors.New("invalid subtitle provider")
	// ErrInvalidApplyMode is returned when metadataApplyMode is not a known value.
	ErrInvalidApplyMode = errors.New("invalid metadata apply mode")
	// ErrInvalidWriteTarget is returned when metadataWriteTarget is not a known value.
	ErrInvalidWriteTarget = errors.New("invalid metadata write target")
	// ErrSubtitleLanguagesRequired is returned when a subtitle provider is set with no languages.
	ErrSubtitleLanguagesRequired = errors.New("subtitle languages required when subtitle provider is set")
	// ErrInvalidSubtitleLanguage is returned when a language code is not ISO 639-1 alpha-2.
	ErrInvalidSubtitleLanguage = errors.New("subtitle languages must be ISO 639-1 alpha-2 codes")
)

// subtitleLangPattern enforces ISO 639-1 alpha-2 only (FI-1 L8), e.g. "en", "ru", "ja".
var subtitleLangPattern = regexp.MustCompile(`^[a-z]{2}$`)

// Settings is a library's provider configuration (FI-1 L2-L4: admin only, three optional slots).
type Settings struct {
	LibraryID                 string    `json:"libraryId"`
	MetadataProvider          *string   `json:"metadataProvider,omitempty"`
	PosterProvider            *string   `json:"posterProvider,omitempty"`
	SubtitleProvider          *string   `json:"subtitleProvider,omitempty"`
	SubtitleLanguages         []string  `json:"subtitleLanguages"`
	AllowOverrideUserMetadata bool      `json:"allowOverrideUserMetadata"`
	MetadataApplyMode         string    `json:"metadataApplyMode"`
	MetadataWriteTarget       string    `json:"metadataWriteTarget"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

// DefaultSettings returns the all-slots-empty configuration for a library that has never
// had providers configured (FI-1 L3: all slots nullable, never mandatory).
func DefaultSettings(libraryID string) Settings {
	return Settings{
		LibraryID:           libraryID,
		SubtitleLanguages:   []string{},
		MetadataApplyMode:   ApplyModeFillMissing,
		MetadataWriteTarget: WriteTargetDB,
	}
}

// IsSubtitleLanguage reports whether code is a valid ISO 639-1 alpha-2 subtitle language.
func IsSubtitleLanguage(code string) bool {
	return subtitleLangPattern.MatchString(code)
}
