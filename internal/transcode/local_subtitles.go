package transcode

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
)

//nolint:gochecknoglobals // compiled once
var (
	sidecarEpisodeMarkerPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])(?:s(\d{1,2})[\s._-]*e(\d{1,2})|(\d{1,2})[xх](\d{1,2}))(?:[\s._-]|$)`,
	)
	sidecarLangTokenPattern = regexp.MustCompile(`(?i)^[a-z]{2,3}$`)
)

// Common ISO 639-2/B/T → ISO 639-1 for subtitle filenames and ffprobe tags.
var subtitleLangAliases = map[string]string{ //nolint:gochecknoglobals // static map
	"eng": "en", "rus": "ru", "spa": "es", "jpn": "ja", "jap": "ja",
	"fre": "fr", "fra": "fr", "ger": "de", "deu": "de", "ita": "it",
	"por": "pt", "chi": "zh", "zho": "zh", "kor": "ko", "ukr": "uk",
	"pol": "pl", "dut": "nl", "nld": "nl", "swe": "sv", "nor": "no",
	"fin": "fi", "dan": "da", "tur": "tr", "ara": "ar", "heb": "he",
	"hin": "hi", "tha": "th", "vie": "vi", "cze": "cs", "ces": "cs",
	"hun": "hu", "rum": "ro", "ron": "ro", "gre": "el", "ell": "el",
}

// LocalSubtitleLanguages returns ISO 639-1 codes already available locally for mediaPath
// (language-tagged sidecars and embedded streams). Untagged tracks do not appear in the set
// and therefore do not block provider searches for specific languages.
func LocalSubtitleLanguages(ctx context.Context, mediaPath string) (map[string]struct{}, error) {
	langs := make(map[string]struct{})

	sidecars, err := sidecarSubtitleCandidates(mediaPath)
	if err != nil {
		return nil, err
	}
	for _, candidate := range sidecars {
		if lang, ok := sidecarLanguageCode(mediaPath, candidate); ok {
			langs[lang] = struct{}{}
		}
	}

	info, err := ProbeSource(ctx, mediaPath)
	if err != nil {
		if len(langs) > 0 {
			return langs, nil
		}

		return nil, err
	}
	for _, stream := range info.SubtitleStreams {
		if lang, ok := normalizeSubtitleLanguage(stream.Language); ok {
			langs[lang] = struct{}{}
		}
	}

	return langs, nil
}

// HasLocalSubtitles reports whether mediaPath has any matching sidecar file or embedded
// subtitle stream (language-agnostic). Prefer LocalSubtitleLanguages for provider skip logic.
func HasLocalSubtitles(ctx context.Context, mediaPath string) (bool, error) {
	sidecars, err := sidecarSubtitleCandidates(mediaPath)
	if err != nil {
		return false, err
	}
	if len(sidecars) > 0 {
		return true, nil
	}

	info, err := ProbeSource(ctx, mediaPath)
	if err != nil {
		return false, err
	}

	return len(info.SubtitleStreams) > 0, nil
}

func sidecarLanguageCode(mediaPath, candidatePath string) (string, bool) {
	label := sidecarSubtitleLabel(mediaPath, candidatePath)
	label = strings.TrimSuffix(label, filepath.Ext(label))
	if label == "" {
		return "", false
	}

	for token := range strings.SplitSeq(strings.ToLower(label), ".") {
		token = strings.TrimSpace(token)
		if lang, ok := normalizeSubtitleLanguage(token); ok {
			return lang, true
		}
	}

	return "", false
}

func normalizeSubtitleLanguage(raw string) (string, bool) {
	code := strings.ToLower(strings.TrimSpace(raw))
	if code == "" || code == "und" || code == "null" {
		return "", false
	}
	if mapped, ok := subtitleLangAliases[code]; ok {
		return mapped, true
	}
	if len(code) == 2 && sidecarLangTokenPattern.MatchString(code) {
		return code, true
	}

	return "", false
}
