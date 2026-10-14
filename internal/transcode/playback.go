package transcode

import (
	"path/filepath"
	"strconv"
	"strings"
)

// TrackInfo describes an audio or subtitle rendition exposed to clients.
type TrackInfo struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Default bool   `json:"default,omitempty"`
}

// QualityInfo describes one HLS video variant.
type QualityInfo struct {
	Height int    `json:"height"`
	Label  string `json:"label"`
}

// PlaybackInfo is returned by GET /api/playback.
//
// Every listed quality and track is immediately playable: the master playlist advertises them
// all and segments are generated on demand, so there is no per-rung readiness to report.
type PlaybackInfo struct {
	Status          Status        `json:"status"`
	Error           string        `json:"error,omitempty"`
	DurationSeconds float64       `json:"durationSeconds,omitempty"`
	PackagingMode   PackagingMode `json:"packagingMode,omitempty"`
	Encoder         string        `json:"encoder,omitempty"`
	ToneMapped      bool          `json:"toneMapped,omitempty"`
	Qualities       []QualityInfo `json:"qualities,omitempty"`
	AudioTracks     []TrackInfo   `json:"audioTracks,omitempty"`
	SubtitleTracks  []TrackInfo   `json:"subtitleTracks,omitempty"`
	// Chapters come from the media container (any video), not from series catalog identity.
	Chapters  []ChapterInfo `json:"chapters,omitempty"`
	MasterURL string        `json:"masterUrl,omitempty"`
}

// BuildPlaybackInfo assembles the client playback contract from cached probe metadata.
func BuildPlaybackInfo(outDir, masterURL string, jobStatus JobStatus) PlaybackInfo {
	return BuildPlaybackInfoWithSettings(outDir, masterURL, jobStatus, DefaultTranscodeSettings())
}

// BuildPlaybackInfoWithSettings includes resolved encoder / tone-map visibility fields.
func BuildPlaybackInfoWithSettings(
	outDir, masterURL string,
	jobStatus JobStatus,
	settings TranscodeSettings,
) PlaybackInfo {
	return BuildPlaybackInfoWithUserPrefs(outDir, masterURL, jobStatus, settings, nil)
}

// BuildPlaybackInfoWithUserPrefs applies optional user audio language priority to track defaults.
func BuildPlaybackInfoWithUserPrefs(
	outDir, masterURL string,
	jobStatus JobStatus,
	settings TranscodeSettings,
	userLanguages []string,
) PlaybackInfo {
	info := PlaybackInfo{Status: jobStatus.Status, Error: jobStatus.Error}
	if jobStatus.Status == StatusReady {
		info.MasterURL = masterURL
	}

	meta, ok := ReadSourceMeta(outDir)
	if !ok {
		if seconds, hasDuration := ReadSourceDuration(outDir); hasDuration {
			info.DurationSeconds = seconds
		}

		return info
	}

	info.DurationSeconds = meta.DurationSeconds
	info.PackagingMode = meta.PackagingMode
	info.Qualities = meta.Qualities()
	defaultAudio := SelectDefaultAudioStreamIndex(meta.AudioStreams, userLanguages)
	info.AudioTracks = meta.AudioTracksWithDefault(defaultAudio)
	info.SubtitleTracks = packagedSubtitleTrackInfos(outDir)
	info.ToneMapped = meta.NeedsToneMap() && settings.ToneMappingEnabled
	if meta.Chapters != nil {
		info.Chapters = meta.Chapters
	}

	encoder, _ := ResolveVideoEncoder(settings.HwAccel)
	info.Encoder = string(encoder)

	return info
}

func packagedSubtitleTrackInfos(outDir string) []TrackInfo {
	packaged := listPackagedSubtitleTracks(outDir)
	tracks := make([]TrackInfo, 0, len(packaged))

	for index, track := range packaged {
		tracks = append(tracks, TrackInfo{
			ID:    strconv.Itoa(index),
			Label: track.DisplayName,
		})
	}

	return tracks
}

func formatQualityLabel(height int) string {
	if label, ok := qualityLabels[height]; ok {
		return label
	}

	return strconv.Itoa(height) + "p"
}

//nolint:gochecknoglobals // quality label map mirrors ladder constants
var qualityLabels = map[int]string{
	2160: "4K",
	1440: "2K",
	1080: "FHD",
	720:  "HD",
	480:  "480p",
	360:  "360p",
	240:  "240p",
}

// ContentTypeForResource returns the HTTP content type for an HLS resource path.
func ContentTypeForResource(resource string) string {
	base := filepath.Base(resource)
	switch {
	case strings.HasSuffix(base, ".m3u8"):
		return "application/vnd.apple.mpegurl"
	case strings.HasSuffix(base, ".ts"):
		return "video/mp2t"
	case strings.HasSuffix(base, ".m4s"):
		return "video/iso.segment"
	case strings.HasSuffix(base, ".vtt"):
		return "text/vtt; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
