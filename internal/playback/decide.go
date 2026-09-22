package playback

import (
	"path/filepath"
	"strings"
)

// Decision is the play-method outcome for one negotiate request.
type Decision struct {
	Method PlayMethod
}

// Decide picks Direct Play, remux, or transcode from the locked FI-8 tree:
//
//	if profile can Direct Play (and quality is original) → directPlay
//	else if original quality → remux when eligible, else transcode
//	else → transcode
//
// selectedHeight 0 means "original / source" quality.
func Decide(profile DeviceProfile, source SourceCaps, selectedHeight int) Decision {
	originalQuality := selectedHeight <= 0 || (source.Height > 0 && selectedHeight >= source.Height)

	if originalQuality && !profile.PreferHLS && canDirectPlay(profile, source) {
		return Decision{Method: MethodDirectPlay}
	}

	if originalQuality && source.RemuxOK {
		return Decision{Method: MethodRemux}
	}

	return Decision{Method: MethodTranscode}
}

func canDirectPlay(profile DeviceProfile, source SourceCaps) bool {
	if source.Container == "" || source.VideoCodec == "" {
		return false
	}

	if profile.MaxStaticBitrate > 0 && source.Bitrate > 0 && source.Bitrate > profile.MaxStaticBitrate {
		return false
	}

	for _, entry := range profile.DirectPlay {
		if matchesDirectPlay(entry, source) {
			return true
		}
	}

	return false
}

func matchesDirectPlay(entry DirectPlayProfile, source SourceCaps) bool {
	if !csvContains(entry.Container, source.Container) {
		return false
	}

	if entry.VideoCodec != "" && !csvContains(entry.VideoCodec, source.VideoCodec) {
		return false
	}

	if entry.AudioCodec != "" && source.AudioCodec != "" &&
		!csvContains(entry.AudioCodec, source.AudioCodec) {
		return false
	}

	return true
}

// csvContains reports whether needle is one of the comma-separated tokens in list.
func csvContains(list, needle string) bool {
	needle = normalizeToken(needle)
	if needle == "" {
		return false
	}

	for part := range strings.SplitSeq(list, ",") {
		if normalizeToken(part) == needle {
			return true
		}
	}

	return false
}

func normalizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "avc1", "avc":
		return "h264"
	case "hev1", "hvc1", "h265":
		return "hevc"
	case "vorbis":
		return "vorbis"
	default:
		return value
	}
}

// ContainerFromPath maps a media file extension to a Direct Play container token.
func ContainerFromPath(mediaPath string) string {
	switch strings.ToLower(filepath.Ext(mediaPath)) {
	case ".mp4", ".m4v":
		return "mp4"
	case ".webm":
		return "webm"
	case ".mkv":
		return "mkv"
	case ".mov":
		return "mov"
	case ".avi":
		return "avi"
	case ".mpg", ".mpeg":
		return "mpeg"
	case ".wmv":
		return "wmv"
	default:
		return strings.TrimPrefix(strings.ToLower(filepath.Ext(mediaPath)), ".")
	}
}
