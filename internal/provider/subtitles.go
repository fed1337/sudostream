package provider

import "context"

// SubtitleHint identifies a single video file for a subtitle lookup. Subtitles always match
// per file, never per show (FI-1: match granularity), since tracks are episode-specific.
type SubtitleHint struct {
	RelPath string
	// AbsPath is the resolved media file path for moviehash fallback; empty skips live hashing.
	AbsPath string
	// MovieHash / MovieHashSize are scan-time identity hashes (preferred over AbsPath hashing).
	MovieHash     string
	MovieHashSize int64
	Title         string
	Show          *string
	Season        *int
	Episode       *int
	ImdbID        *string
	TmdbID        *string
}

// SubtitleFile is a fetched subtitle track ready to write under
// /var/lib/sudostream/cache/providers/subtitles/ (FI-1 L7).
type SubtitleFile struct {
	Bytes      []byte
	Lang       string
	ExternalID *string
}

// SubtitleProvider fetches subtitle tracks for one adapter (e.g. OpenSubtitles, E3).
type SubtitleProvider interface {
	// Key returns the registry key, e.g. "opensubtitles".
	Key() string
	// FetchSubtitle matches and fetches one subtitle track for hint in lang (ISO 639-1 alpha-2).
	FetchSubtitle(ctx context.Context, hint SubtitleHint, lang string) (MatchStatus, SubtitleFile, error)
}
