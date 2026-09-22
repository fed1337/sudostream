package provider

import "context"

// FileHashHint identifies one media file by stored content hashes (scan-time identity).
type FileHashHint struct {
	RelPath      string
	AbsPath      string
	Ed2kHash     string
	HashFileSize int64
}

// FileHashFields is metadata resolved from a provider file-hash lookup (e.g. AniDB ed2k).
type FileHashFields struct {
	Title        *string
	Show         *string
	Description  *string
	Genres       []string
	Studio       *string
	Year         *int
	EpisodeTitle *string
	Season       *int
	Episode      *int
	IDs          ExternalIDs
	ExternalID   *string
}

// FileHashProvider optionally resolves identity via content hash before title search.
// MatchNone / soft errors mean the enricher should fall back to MatchFilm / MatchShow.
type FileHashProvider interface {
	MatchFileHash(ctx context.Context, hint FileHashHint) (MatchStatus, FileHashFields, error)
}
