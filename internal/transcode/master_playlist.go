package transcode

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const subtitleGroupID = "subs"

var sidecarSubtitleExts = map[string]struct{}{
	".vtt": {},
	".srt": {},
	".ass": {},
	".ssa": {},
}

var sidecarSearchSubdirs = []string{"", "Subs", "Subtitles", "subs", "subtitles"}

func sidecarSubtitleCandidates(mediaPath string) ([]string, error) {
	dir := filepath.Dir(mediaPath)
	videoStem := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))

	seen := make(map[string]struct{})
	matches := make([]string, 0)

	for _, sub := range sidecarSearchSubdirs {
		searchDir := dir
		if sub != "" {
			searchDir = filepath.Join(dir, sub)
		}

		entries, err := os.ReadDir(searchDir)
		if err != nil {
			if sub == "" {
				return nil, fmt.Errorf("read media dir: %w", err)
			}

			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if _, ok := sidecarSubtitleExts[ext]; !ok {
				continue
			}

			stem := strings.TrimSuffix(name, filepath.Ext(name))
			if !subtitleStemMatches(videoStem, stem) {
				continue
			}

			full := filepath.Join(searchDir, name)
			if _, ok := seen[full]; ok {
				continue
			}
			seen[full] = struct{}{}
			matches = append(matches, full)
		}
	}

	sort.Strings(matches)

	return matches, nil
}

func subtitleStemMatches(videoStem, sidecarStem string) bool {
	video := foldSubtitleStem(videoStem)
	sidecar := foldSubtitleStem(sidecarStem)
	if video == sidecar {
		return true
	}
	// Language / forced tags: movie.en.srt, movie.en.forced.srt
	if strings.HasPrefix(sidecar, video+".") {
		return true
	}
	// Series: same SxxExx / NxNN even when titles differ (or sidecar lives in Subs/).
	return episodeMarkerMatches(video, sidecar)
}

func episodeMarkerMatches(videoStem, sidecarStem string) bool {
	videoKey, videoLeft, ok := episodeMarkerParts(videoStem)
	if !ok {
		return false
	}
	sidecarKey, sidecarLeft, ok := episodeMarkerParts(sidecarStem)
	if !ok || videoKey != sidecarKey {
		return false
	}

	videoLeft = strings.Trim(videoLeft, " ._-")
	sidecarLeft = strings.Trim(sidecarLeft, " ._-")
	if videoLeft == "" || sidecarLeft == "" {
		// Subs/S01E02.ru.srt next to a single episode file.
		return true
	}

	return strings.HasPrefix(videoLeft, sidecarLeft) || strings.HasPrefix(sidecarLeft, videoLeft)
}

func episodeMarkerParts(stem string) (string, string, bool) {
	loc := sidecarEpisodeMarkerPattern.FindStringSubmatchIndex(stem)
	match := sidecarEpisodeMarkerPattern.FindStringSubmatch(stem)
	if match == nil || loc == nil {
		return "", "", false
	}

	var season, episode string
	switch {
	case match[1] != "" && match[2] != "":
		season, episode = match[1], match[2]
	case match[3] != "" && match[4] != "":
		season, episode = match[3], match[4]
	default:
		return "", "", false
	}

	seasonN, err := strconv.Atoi(season)
	if err != nil {
		return "", "", false
	}
	episodeN, err := strconv.Atoi(episode)
	if err != nil {
		return "", "", false
	}

	return fmt.Sprintf("s%02de%02d", seasonN, episodeN), stem[:loc[0]], true
}

func foldSubtitleStem(stem string) string {
	stem = strings.ToLower(strings.TrimSpace(stem))

	return strings.NewReplacer(
		"х", "x",
		"×", "x",
	).Replace(stem)
}

func sidecarSubtitleLabel(mediaPath, candidatePath string) string {
	videoStem := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	base := filepath.Base(candidatePath)
	foldedVideo := foldSubtitleStem(videoStem)
	foldedBase := foldSubtitleStem(base)

	prefix := foldedVideo + "."
	if !strings.HasPrefix(foldedBase, prefix) {
		return base
	}

	if len(base) >= len(videoStem)+1 &&
		strings.EqualFold(base[:len(videoStem)], videoStem) &&
		base[len(videoStem)] == '.' {
		trimmed := base[len(videoStem)+1:]
		if trimmed != "" {
			return trimmed
		}

		return base
	}

	trimmed := base[len(prefix):]
	if trimmed != "" {
		return trimmed
	}

	return base
}
