package metadata

import (
	"path/filepath"
	"strings"
	"unicode"
)

// BuildSearchDocument concatenates filename tokens, heuristics, and JSON string fields.
func BuildSearchDocument(relPath string, original, override VideoFields) string {
	parts := make([]string, 0, 32) //nolint:mnd // typical token count for path + fields
	relPath = strings.TrimSpace(relPath)
	basename := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))

	parts = append(
		parts,
		relPath,
		basename,
		normalizeSearchText(relPath),
		normalizeSearchText(basename),
	)

	film := ParseFilmIdentity(relPath)
	if film.Title != "" {
		parts = append(parts, film.Title)
	}
	series := ParseSeriesIdentity(relPath)
	if series.Show != "" {
		parts = append(parts, series.Show, series.ShowKey)
	}

	appendVideoFieldStrings(&parts, original)
	appendVideoFieldStrings(&parts, override)

	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	return strings.Join(out, " ")
}

// NormalizeSearchQuery splits a user query into AND tokens.
func NormalizeSearchQuery(raw string) []string {
	normalized := normalizeSearchText(raw)
	fields := strings.Fields(normalized)
	tokens := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, exists := seen[field]; exists {
			continue
		}
		seen[field] = struct{}{}
		tokens = append(tokens, field)
	}

	return tokens
}

func normalizeSearchText(value string) string { //nolint:cyclop // punctuation folding is explicit
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	prevSpace := false
	for _, runeValue := range value {
		switch {
		case unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue):
			builder.WriteRune(runeValue)
			prevSpace = false
		case runeValue == '.' || runeValue == '_' || runeValue == '-' ||
			runeValue == '[' || runeValue == ']' || runeValue == '{' || runeValue == '}' ||
			unicode.IsSpace(runeValue):
			if !prevSpace && builder.Len() > 0 {
				builder.WriteByte(' ')
				prevSpace = true
			}
		}
	}

	return strings.TrimSpace(builder.String())
}

func appendVideoFieldStrings(parts *[]string, fields VideoFields) {
	add := func(value *string) {
		if value == nil {
			return
		}
		trimmed := strings.TrimSpace(*value)
		if trimmed != "" {
			*parts = append(*parts, trimmed)
		}
	}
	add(fields.Title)
	add(fields.SortTitle)
	add(fields.OriginalTitle)
	add(fields.EpisodeTitle)
	add(fields.Show)
	add(fields.Description)
	add(fields.Studio)
	add(fields.IMDBID)
	add(fields.TMDBID)
	*parts = append(*parts, fields.Genres...)
	*parts = append(*parts, fields.Tags...)
	*parts = append(*parts, fields.Directors...)
	*parts = append(*parts, fields.Actors...)
	*parts = append(*parts, fields.Writers...)
	*parts = append(*parts, fields.Producers...)
}

func escapeILIKE(token string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

	return replacer.Replace(token)
}

// EscapeILIKEToken escapes LIKE metacharacters for a search token.
func EscapeILIKEToken(token string) string {
	return escapeILIKE(token)
}
