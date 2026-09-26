package transcode

import (
	"strings"
)

const maxUserAudioLanguagePrefs = 3

// SelectDefaultAudioStreamIndex picks the default encode/HLS audio track index using user
// language priority, skipping commentary/descriptive tracks when alternatives exist.
func SelectDefaultAudioStreamIndex(streams []AudioStream, userLanguages []string) int {
	if len(streams) == 0 {
		return -1
	}

	prefs := normalizeLanguagePrefs(userLanguages)
	if len(prefs) > 0 {
		for _, pref := range prefs {
			for index, stream := range streams {
				if stream.Commentary {
					continue
				}
				if languageMatches(pref, stream.Language) {
					return index
				}
			}
		}
	}

	for index, stream := range streams {
		if !stream.Commentary {
			return index
		}
	}

	return 0
}

func normalizeLanguagePrefs(langs []string) []string {
	out := make([]string, 0, len(langs))
	for _, lang := range langs {
		lang = strings.ToLower(strings.TrimSpace(lang))
		if lang == "" {
			continue
		}
		out = append(out, lang)
		if len(out) >= maxUserAudioLanguagePrefs {
			break
		}
	}

	return out
}

func languageMatches(pref, trackLang string) bool {
	trackLang = strings.ToLower(strings.TrimSpace(trackLang))
	if trackLang == "" {
		return false
	}
	if trackLang == pref {
		return true
	}

	// Match ISO 639-2 (eng) with 639-1 (en) prefixes.
	return strings.HasPrefix(trackLang, pref) || strings.HasPrefix(pref, trackLang)
}
