package transcode

import (
	"path/filepath"
	"strings"
	"sudoStream/internal/mediafs"
)

const (
	masterPlaylistName = "master.m3u8"
	subtitlesDirName   = "subtitles"
)

// isHLSResourceStart reports whether a path segment begins an HLS resource suffix
// (master, variant dir, legacy stream_*, or packaged subtitles).
func isHLSResourceStart(name string) bool {
	switch {
	case name == masterPlaylistName, name == subtitlesDirName:
		return true
	case isHLSStreamDir(name):
		return true
	default:
		_, ok := ParseVariantDir(name)

		return ok
	}
}

func isHLSStreamDir(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), "stream_")
}

// SplitPlayPath separates a play URL path into the media rel path and HLS resource suffix.
func SplitPlayPath(raw string) (string, string, bool) {
	segments := strings.Split(strings.TrimPrefix(raw, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return "", "", false
	}

	if mediaPath, resource, ok := splitPlayPathWithVideoExt(segments); ok {
		return mediaPath, resource, true
	}

	if mediaPath, resource, ok := splitPlayPathExtensionless(segments); ok {
		return mediaPath, resource, true
	}

	return "", "", false
}

func splitPlayPathWithVideoExt(segments []string) (string, string, bool) {
	videoIndex := -1
	for index, segment := range segments {
		if mediafs.IsVideoExtension(filepath.Ext(segment)) {
			videoIndex = index
		}
	}

	if videoIndex < 0 {
		return "", "", false
	}

	mediaPath := strings.Join(segments[:videoIndex+1], "/")
	resource := masterPlaylistName
	if videoIndex+1 < len(segments) {
		resource = strings.Join(segments[videoIndex+1:], "/")
	}

	return mediaPath, resource, true
}

func splitPlayPathExtensionless(segments []string) (string, string, bool) {
	for index, segment := range segments {
		if !isHLSResourceStart(segment) {
			continue
		}
		if index == 0 {
			return "", "", false
		}

		mediaPath := strings.Join(segments[:index], "/")
		resource := strings.Join(segments[index:], "/")

		return mediaPath, resource, true
	}

	mediaPath := strings.Join(segments, "/")

	return mediaPath, masterPlaylistName, true
}
