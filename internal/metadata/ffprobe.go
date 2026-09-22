package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const yearPrefixLength = 4

type ffprobeOutput struct {
	Format  ffprobeFormat   `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeFormat struct {
	Duration string            `json:"duration"`
	BitRate  string            `json:"bit_rate"` //nolint:tagliatelle // ffprobe JSON
	Tags     map[string]string `json:"tags"`
}

type ffprobeStream struct {
	CodecType  string            `json:"codec_type"` //nolint:tagliatelle // ffprobe JSON
	CodecName  string            `json:"codec_name"` //nolint:tagliatelle // ffprobe JSON
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	RFrameRate string            `json:"r_frame_rate"` //nolint:tagliatelle // ffprobe JSON
	Channels   int               `json:"channels"`
	Tags       map[string]string `json:"tags"`
}

// ProbePath runs ffprobe on absPath and returns normalized metadata.
func ProbePath(absPath string) (VideoFields, error) {
	return ProbePathContext(context.Background(), absPath)
}

// ProbePathContext runs ffprobe with cancellation support.
func ProbePathContext(ctx context.Context, absPath string) (VideoFields, error) {
	output, err := exec.CommandContext( //nolint:gosec // absPath resolved via mediafs
		ctx,
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		absPath,
	).Output()
	if err != nil {
		return VideoFields{}, fmt.Errorf("ffprobe %s: %w", filepath.Base(absPath), err)
	}

	var parsed ffprobeOutput
	err = json.Unmarshal(output, &parsed)
	if err != nil {
		return VideoFields{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	return mapFFProbe(parsed), nil
}

// ParseFFProbeJSON maps recorded ffprobe JSON into VideoFields (for tests).
func ParseFFProbeJSON(raw []byte) (VideoFields, error) {
	var parsed ffprobeOutput
	err := json.Unmarshal(raw, &parsed)
	if err != nil {
		return VideoFields{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	return mapFFProbe(parsed), nil
}

func mapFFProbe( //nolint:cyclop,gocognit // ffprobe tag mapping is intentionally explicit
	parsed ffprobeOutput,
) VideoFields {
	fields := VideoFields{} //nolint:exhaustruct // populated below

	// Editorial tags come from the container/format only. Stream titles are
	// track labels (e.g. "DD 2.0 @ 192 kbps") and must not become the file title.
	tags := cloneTagMap(parsed.Format.Tags)
	applyTagMappings(&fields, tags)

	if parsed.Format.Duration != "" {
		seconds, parseErr := strconv.ParseFloat(parsed.Format.Duration, 64)
		if parseErr == nil {
			fields.DurationSeconds = &seconds
		}
	}

	if parsed.Format.BitRate != "" {
		bitrate, parseErr := strconv.ParseInt(parsed.Format.BitRate, 10, 64)
		if parseErr == nil {
			fields.Bitrate = &bitrate
		}
	}

	var audioLanguages []string
	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			if fields.VideoCodec == nil && stream.CodecName != "" {
				codec := stream.CodecName
				fields.VideoCodec = &codec
			}
			if fields.Width == nil && stream.Width > 0 {
				width := stream.Width
				fields.Width = &width
			}
			if fields.Height == nil && stream.Height > 0 {
				height := stream.Height
				fields.Height = &height
			}
			if fields.FrameRate == nil && stream.RFrameRate != "" && stream.RFrameRate != "0/0" {
				frameRate := stream.RFrameRate
				fields.FrameRate = &frameRate
			}
		case "audio":
			if fields.AudioCodec == nil && stream.CodecName != "" {
				codec := stream.CodecName
				fields.AudioCodec = &codec
			}
			if fields.AudioChannels == nil && stream.Channels > 0 {
				channels := stream.Channels
				fields.AudioChannels = &channels
			}
			if lang := firstNonEmpty(stream.Tags["language"], stream.Tags["LANGUAGE"]); lang != "" {
				audioLanguages = appendUnique(audioLanguages, lang)
			}
		}
	}

	if len(audioLanguages) > 0 {
		fields.AudioLanguages = audioLanguages
	}

	return fields
}

func applyTagMappings(fields *VideoFields, tags map[string]string) {
	setString(fields, &fields.Title, tags, "title", "TITLE", "Title")
	setString(fields, &fields.SortTitle, tags, "sort_name", "SORT_NAME")
	setString(fields, &fields.OriginalTitle, tags, "original_title", "ORIGINAL_TITLE")
	setString(fields, &fields.EpisodeTitle, tags, "episode_title", "EPISODE_TITLE", "subtitle")
	setString(fields, &fields.Show, tags, "show", "SHOW", "series", "SERIES", "tvshow", "TVSHOW")
	setString(
		fields,
		&fields.Description,
		tags,
		"description",
		"DESCRIPTION",
		"synopsis",
		"SYNOPSIS",
		"comment",
		"COMMENT",
	)
	setString(fields, &fields.Studio, tags, "studio", "STUDIO", "publisher", "PUBLISHER")
	setString(fields, &fields.Composer, tags, "composer", "COMPOSER")
	setString(fields, &fields.Language, tags, "language", "LANGUAGE", "lang", "LANG")
	setString(fields, &fields.Country, tags, "country", "COUNTRY")
	setString(
		fields,
		&fields.ContentRating,
		tags,
		"content_rating",
		"CONTENT_RATING",
		"rating",
		"RATING",
	)
	setString(fields, &fields.IMDBID, tags, "imdb_id", "IMDB_ID", "IMDB")
	setString(fields, &fields.TMDBID, tags, "tmdb_id", "TMDB_ID", "TMDB")

	if season := firstIntTag(
		tags,
		"season_number",
		"SEASON_NUMBER",
		"season",
		"SEASON",
	); season != nil {
		fields.Season = season
	}

	if episode := firstIntTag(
		tags,
		"episode_id",
		"EPISODE_ID",
		"episode",
		"EPISODE",
		"episode_sort",
	); episode != nil {
		fields.Episode = episode
	}

	if year := firstIntTag(tags, "year", "YEAR", "date", "DATE"); year != nil {
		fields.Year = year
	}

	if release := firstNonEmpty(
		tags["release_date"],
		tags["RELEASE_DATE"],
		tags["date"],
		tags["DATE"],
	); release != "" {
		normalized := normalizeReleaseDate(release)
		if normalized != "" {
			fields.ReleaseDate = &normalized
		}
	}

	fields.Genres = splitList(firstNonEmpty(tags["genre"], tags["GENRE"]))
	fields.Directors = splitList(firstNonEmpty(tags["director"], tags["DIRECTOR"]))
	fields.Actors = splitList(
		firstNonEmpty(tags["actor"], tags["ACTOR"], tags["actors"], tags["ACTORS"]),
	)
	fields.Writers = splitList(firstNonEmpty(tags["writer"], tags["WRITER"]))
	fields.Producers = splitList(firstNonEmpty(tags["producer"], tags["PRODUCER"]))
	fields.Tags = splitList(
		firstNonEmpty(tags["keywords"], tags["KEYWORDS"], tags["tags"], tags["TAGS"]),
	)
}

func setString(fields *VideoFields, target **string, tags map[string]string, keys ...string) {
	if *target != nil {
		return
	}

	value := firstNonEmptyLookup(tags, keys...)
	if value == "" {
		return
	}

	*target = &value
	_ = fields
}

func firstNonEmptyLookup(tags map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(tags[key]); value != "" {
			return value
		}
	}

	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func firstIntTag(tags map[string]string, keys ...string) *int {
	raw := firstNonEmptyLookup(tags, keys...)
	if raw == "" {
		return nil
	}

	digits := strings.TrimLeft(raw, "0")
	if digits == "" {
		zero := 0

		return &zero
	}

	parsed, err := strconv.Atoi(digits)
	if err != nil {
		return nil
	}

	return &parsed
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '/'
	})

	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func normalizeReleaseDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if len(raw) >= yearPrefixLength && raw[0] >= '0' && raw[0] <= '9' {
		const isoDateLength = 10
		if len(raw) >= isoDateLength && raw[4] == '-' {
			return raw[:isoDateLength]
		}

		if len(raw) >= yearPrefixLength {
			return raw[:yearPrefixLength] + "-01-01"
		}
	}

	return raw
}

func cloneTagMap(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(tags))
	maps.Copy(cloned, tags)

	return cloned
}

func appendUnique(items []string, value string) []string {
	if slices.Contains(items, value) {
		return items
	}

	return append(items, value)
}

// IsVideoExtension reports whether ext is a supported video container.
func IsVideoExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mkv", ".mp4", ".m4v", ".avi", ".ts", ".m2ts", ".webm", ".mov", ".wmv", ".mpg", ".mpeg":
		return true
	default:
		return false
	}
}
