// Package metadata indexes and serves cached video metadata.
package metadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"sudoStream/internal/access"
	"time"
)

var (
	// ErrNotFound is returned when no metadata row exists for a path.
	ErrNotFound = errors.New("metadata not found")
	// ErrNotVideo is returned when the path is not a supported video file.
	ErrNotVideo = errors.New("path is not a supported video file")
	// ErrFileTargetUnsupported is returned when PATCH target=file is requested without a media service.
	ErrFileTargetUnsupported = errors.New("file metadata editing is not supported yet")
	// ErrFileWriteFailed is returned when the ffmpeg metadata remux fails.
	ErrFileWriteFailed = errors.New("file metadata write failed")
	// ErrInvalidTarget is returned when PATCH target is not recognized.
	ErrInvalidTarget = errors.New("invalid metadata patch target")
	// ErrInvalidPatchValue is returned when a patch field has an unsupported value.
	ErrInvalidPatchValue = errors.New("invalid metadata patch value")
	// ErrServiceUnavailable is returned when the metadata service is not configured.
	ErrServiceUnavailable = errors.New("metadata service unavailable")
	// ErrUnknownLibrary is returned when a path does not belong to a registered library.
	ErrUnknownLibrary = errors.New("unknown library for path")
)

// PatchTarget selects where metadata writes are applied.
type PatchTarget string

const (
	// PatchTargetOverride writes to per-file DB overrides.
	PatchTargetOverride PatchTarget = "override"
	// PatchTargetFile writes embedded tags via ffmpeg.
	PatchTargetFile PatchTarget = "file"
)

// VideoFields holds normalized video metadata keys shared by original, override, and effective.
//
//nolint:tagliatelle // Metadata API and DB schema use snake_case keys per I3 plan.
type VideoFields struct {
	Title           *string  `json:"title,omitempty"`
	SortTitle       *string  `json:"sort_title,omitempty"`
	OriginalTitle   *string  `json:"original_title,omitempty"`
	EpisodeTitle    *string  `json:"episode_title,omitempty"`
	Show            *string  `json:"show,omitempty"`
	Season          *int     `json:"season,omitempty"`
	Episode         *int     `json:"episode,omitempty"`
	Year            *int     `json:"year,omitempty"`
	ReleaseDate     *string  `json:"release_date,omitempty"`
	Description     *string  `json:"description,omitempty"`
	Genres          []string `json:"genres,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Directors       []string `json:"directors,omitempty"`
	Actors          []string `json:"actors,omitempty"`
	Writers         []string `json:"writers,omitempty"`
	Producers       []string `json:"producers,omitempty"`
	AudioLanguages  []string `json:"audio_languages,omitempty"`
	Studio          *string  `json:"studio,omitempty"`
	Composer        *string  `json:"composer,omitempty"`
	Language        *string  `json:"language,omitempty"`
	Country         *string  `json:"country,omitempty"`
	ContentRating   *string  `json:"content_rating,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	VideoCodec      *string  `json:"video_codec,omitempty"`
	AudioCodec      *string  `json:"audio_codec,omitempty"`
	Width           *int     `json:"width,omitempty"`
	Height          *int     `json:"height,omitempty"`
	FrameRate       *string  `json:"frame_rate,omitempty"`
	Bitrate         *int64   `json:"bitrate,omitempty"`
	AudioChannels   *int     `json:"audio_channels,omitempty"`
	IMDBID          *string  `json:"imdb_id,omitempty"`
	TMDBID          *string  `json:"tmdb_id,omitempty"`
	TVDBID          *string  `json:"tvdb_id,omitempty"`
	TvmazeID        *string  `json:"tvmaze_id,omitempty"`
	// System-owned identity hashes (scan-time; never overwritten; not PATCH-editable).
	MovieHash    *string `json:"movie_hash,omitempty"`
	Ed2kHash     *string `json:"ed2k_hash,omitempty"`
	HashFileSize *int64  `json:"hash_file_size,omitempty"` // size when hashes were computed
}

// SourceFields separates cached original tags from DB overrides.
type SourceFields struct {
	Original VideoFields `json:"original"`
	Override VideoFields `json:"override"`
}

// StoredOverride is the override_fields JSON document.
// Absent/nil keys mean "leave original"; JSON null on PATCH removes the key
// so effective falls back to the file (original) value.
type StoredOverride struct {
	VideoFields
}

// FilenameHint holds season/episode parsed from the basename (never persisted).
type FilenameHint struct {
	Season  *int `json:"season,omitempty"`
	Episode *int `json:"episode,omitempty"`
}

// MetadataResponse is returned by GET /api/metadata/*path.
//
//nolint:revive,tagliatelle // MetadataResponse is the public API shape referenced by OpenAPI.
type MetadataResponse struct {
	Path                   string             `json:"path"`
	LibraryType            access.LibraryType `json:"library_type"`
	UsesMetadataForDisplay bool               `json:"uses_metadata_for_display"`
	Source                 SourceFields       `json:"source"`
	Effective              VideoFields        `json:"effective"`
	DisplayName            string             `json:"display_name"`
	OverriddenBy           *string            `json:"overridden_by,omitempty"`
	FilenameHint           *FilenameHint      `json:"filename_hint,omitempty"`
	ProbedAt               *time.Time         `json:"probed_at,omitempty"`
}

// PatchRequest updates metadata for a file.
type PatchRequest struct {
	Target PatchTarget           `json:"target"`
	Fields map[string]*jsonValue `json:"fields"`
}

// jsonValue accepts a scalar, list, or null in PATCH bodies.
type jsonValue struct {
	raw any
}

// UnmarshalJSON decodes patch field values including explicit null.
func (v *jsonValue) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		v.raw = nil

		return nil
	}

	var decoded any
	err := json.Unmarshal(data, &decoded)
	if err != nil {
		return fmt.Errorf("decode patch value: %w", err)
	}

	v.raw = decoded

	return nil
}

// IsNull reports whether the patch value clears the field.
func (v *jsonValue) IsNull() bool {
	return v == nil || v.raw == nil
}

// Value returns the decoded patch value.
func (v *jsonValue) Value() any {
	if v == nil {
		return nil
	}

	return v.raw
}

// Row is a persisted metadata record.
type Row struct {
	LibraryID    string
	RelPath      string
	Original     VideoFields
	Override     StoredOverride
	FileMtime    *time.Time
	FileSize     *int64
	ProbedAt     *time.Time
	OverrideAt   *time.Time
	OverriddenBy *string
}

// SearchRow is a metadata path that matched a search query.
type SearchRow struct {
	LibraryID string
	RelPath   string
}

// SearchHit is a hydrated search result for the API.
type SearchHit struct {
	Path        string             `json:"path"`
	Title       string             `json:"title"`
	LibrarySlug string             `json:"librarySlug"`
	LibraryType access.LibraryType `json:"libraryType"`
	ShowKey     string             `json:"showKey,omitempty"`
}

// ShelfRow identifies an indexed media file for home shelves.
type ShelfRow struct {
	LibraryID string
	RelPath   string
	Title     string
}
