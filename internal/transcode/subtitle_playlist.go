package transcode

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const subtitleSegmentName = "track.vtt"

type packagedSubtitleTrack struct {
	ID          string
	DisplayName string
	RelPlaylist string
}

func hasPackagedSubtitleTracks(outDir string) bool {
	return len(listPackagedSubtitleTracks(outDir)) > 0
}

func subtitleVTTHasCues(vttPath string) bool {
	raw, err := os.ReadFile(vttPath)
	if err != nil || len(raw) < 12 {
		return false
	}

	return strings.Contains(string(raw), "-->")
}

func subtitleTrackHasCues(outDir, trackID string) bool {
	if subtitleVTTHasCues(filepath.Join(outDir, "subtitles", trackID, subtitleSegmentName)) {
		return true
	}

	return subtitleVTTHasCues(filepath.Join(outDir, "subtitles", trackID+".vtt"))
}

func purgeInvalidSubtitleTracks(outDir string) {
	subtitlesDir := filepath.Join(outDir, "subtitles")
	entries, err := os.ReadDir(subtitlesDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			if !subtitleTrackHasCues(outDir, entry.Name()) {
				_ = os.RemoveAll(filepath.Join(subtitlesDir, entry.Name()))
			}

			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".vtt") {
			continue
		}

		trackID := strings.TrimSuffix(name, filepath.Ext(name))
		if subtitleTrackHasCues(outDir, trackID) {
			continue
		}

		_ = os.Remove(filepath.Join(subtitlesDir, name))
		_ = os.RemoveAll(filepath.Join(subtitlesDir, trackID))
	}
}

func listPackagedSubtitleTracks(outDir string) []packagedSubtitleTrack {
	subtitlesDir := filepath.Join(outDir, "subtitles")
	entries, err := os.ReadDir(subtitlesDir)
	if err != nil {
		return nil
	}

	tracks := make([]packagedSubtitleTrack, 0, len(entries))
	seen := map[string]struct{}{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		track, ok := packagedSubtitleTrackFromDir(outDir, entry.Name())
		if !ok {
			continue
		}

		tracks = append(tracks, track)
		seen[track.ID] = struct{}{}
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		track, ok := packagedSubtitleTrackFromLegacyVTT(outDir, entry.Name(), seen)
		if !ok {
			continue
		}

		tracks = append(tracks, track)
	}

	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].ID < tracks[j].ID
	})

	return tracks
}

func packagedSubtitleTrackFromDir(outDir, trackID string) (packagedSubtitleTrack, bool) {
	playlistPath := filepath.Join(outDir, "subtitles", trackID, "playlist.m3u8")
	_, statErr := os.Stat(playlistPath)
	if statErr != nil || !subtitleTrackHasCues(outDir, trackID) {
		return packagedSubtitleTrack{}, false
	}

	return packagedSubtitleTrack{
		ID:          trackID,
		DisplayName: subtitleDisplayName(trackID),
		RelPlaylist: filepath.ToSlash(filepath.Join("subtitles", trackID, "playlist.m3u8")),
	}, true
}

func packagedSubtitleTrackFromLegacyVTT(
	outDir, name string,
	seen map[string]struct{},
) (packagedSubtitleTrack, bool) {
	if !strings.HasSuffix(strings.ToLower(name), ".vtt") {
		return packagedSubtitleTrack{}, false
	}

	trackID := strings.TrimSuffix(name, filepath.Ext(name))
	if _, ok := seen[trackID]; ok {
		return packagedSubtitleTrack{}, false
	}

	playlistPath := filepath.Join(outDir, "subtitles", trackID, "playlist.m3u8")
	_, statErr := os.Stat(playlistPath)
	if statErr != nil || !subtitleTrackHasCues(outDir, trackID) {
		return packagedSubtitleTrack{}, false
	}

	return packagedSubtitleTrack{
		ID:          trackID,
		DisplayName: subtitleDisplayName(trackID),
		RelPlaylist: filepath.ToSlash(filepath.Join("subtitles", trackID, "playlist.m3u8")),
	}, true
}

func subtitleDisplayName(trackID string) string {
	switch strings.ToLower(trackID) {
	case "srt", "vtt", "ass", "ssa":
		return "Subtitles"
	default:
		return trackID
	}
}

func writeSubtitleMediaPlaylist(playlistPath, segmentURI string, durationSeconds float64) error {
	if durationSeconds <= 0 {
		durationSeconds = 1
	}

	targetDuration := max(int(math.Ceil(durationSeconds)), 1)

	content := strings.Join([]string{
		hlsPlaylistHeader,
		"#EXT-X-VERSION:3",
		fmt.Sprintf("#EXT-X-TARGETDURATION:%d", targetDuration),
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXT-X-PLAYLIST-TYPE:VOD",
		fmt.Sprintf("#EXTINF:%.3f,", durationSeconds),
		segmentURI,
		"#EXT-X-ENDLIST",
		"",
	}, "\n")

	//nolint:mnd // permissions
	err := os.MkdirAll(filepath.Dir(playlistPath), 0o750)
	if err != nil {
		return fmt.Errorf("mkdir subtitle playlist dir: %w", err)
	}

	//nolint:mnd // permissions
	err = os.WriteFile(playlistPath, []byte(content), 0o600)
	if err != nil {
		return fmt.Errorf("write subtitle playlist: %w", err)
	}

	return nil
}

func ensureSubtitleMediaPlaylists(outDir string, durationSeconds float64) error {
	subtitlesDir := filepath.Join(outDir, "subtitles")
	entries, err := os.ReadDir(subtitlesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("read subtitles dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			err = ensureTrackDirSubtitlePlaylist(subtitlesDir, entry.Name(), durationSeconds)
			if err != nil {
				return err
			}

			continue
		}

		err = ensureLegacyFlatSubtitlePlaylist(subtitlesDir, entry.Name(), durationSeconds)
		if err != nil {
			return err
		}
	}

	return nil
}

func ensureTrackDirSubtitlePlaylist(subtitlesDir, trackID string, durationSeconds float64) error {
	trackDir := filepath.Join(subtitlesDir, trackID)
	playlistPath := filepath.Join(trackDir, "playlist.m3u8")
	segmentPath := filepath.Join(trackDir, subtitleSegmentName)

	_, playlistErr := os.Stat(playlistPath)
	_, segmentErr := os.Stat(segmentPath)
	if playlistErr != nil && segmentErr == nil && subtitleVTTHasCues(segmentPath) {
		return writeSubtitleMediaPlaylist(playlistPath, subtitleSegmentName, durationSeconds)
	}

	return nil
}

func ensureLegacyFlatSubtitlePlaylist(subtitlesDir, name string, durationSeconds float64) error {
	if !strings.HasSuffix(strings.ToLower(name), ".vtt") {
		return nil
	}

	trackID := strings.TrimSuffix(name, filepath.Ext(name))
	playlistPath := filepath.Join(subtitlesDir, trackID, "playlist.m3u8")
	_, statErr := os.Stat(playlistPath)
	if statErr == nil {
		return nil
	}

	legacyPath := filepath.Join(subtitlesDir, name)
	if !subtitleVTTHasCues(legacyPath) {
		return nil
	}

	return writeSubtitleMediaPlaylist(
		playlistPath,
		filepath.ToSlash("../"+name),
		durationSeconds,
	)
}

func sourceDurationForSubtitles(outDir string, source SourceInfo) float64 {
	if duration, ok := ReadSourceDuration(outDir); ok && duration > 0 {
		return duration
	}

	if meta, ok := ReadSourceMeta(outDir); ok && meta.DurationSeconds > 0 {
		return meta.DurationSeconds
	}

	if source.DurationSeconds > 0 {
		return source.DurationSeconds
	}

	return 1
}
