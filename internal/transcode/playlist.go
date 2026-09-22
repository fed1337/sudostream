package transcode

import (
	"net/url"
	"strings"
)

// RewritePlaylistTokens appends access_token to relative URIs in an HLS playlist.
func RewritePlaylistTokens(body []byte, accessToken string) []byte {
	if accessToken == "" {
		return body
	}

	lines := strings.Split(string(body), "\n")
	for index, line := range lines {
		lines[index] = rewritePlaylistLine(line, accessToken)
	}

	return []byte(strings.Join(lines, "\n"))
}

func rewritePlaylistLine(line, accessToken string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return line
	}

	if strings.HasPrefix(trimmed, "#") {
		if strings.Contains(trimmed, `URI="`) {
			return rewriteTagURI(line, accessToken)
		}

		return line
	}

	return appendAccessToken(trimmed, accessToken)
}

func rewriteTagURI(line, accessToken string) string {
	const prefix = `URI="`
	start := strings.Index(line, prefix)
	if start < 0 {
		return line
	}

	start += len(prefix)
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return line
	}

	uri := line[start : start+end]
	updated := appendAccessToken(uri, accessToken)

	return line[:start] + updated + line[start+end:]
}

func appendAccessToken(uri, accessToken string) string {
	if accessToken == "" || strings.Contains(uri, "access_token=") {
		return uri
	}

	separator := "?"
	if strings.Contains(uri, "?") {
		separator = "&"
	}

	return uri + separator + "access_token=" + url.QueryEscape(accessToken)
}

// PlaylistHasEndList reports whether an HLS media playlist is complete.
func PlaylistHasEndList(body []byte) bool {
	return strings.Contains(string(body), "#EXT-X-ENDLIST")
}

// PlaylistCacheControl returns Cache-Control for an HLS playlist response.
func PlaylistCacheControl(body []byte) string {
	if PlaylistHasEndList(body) {
		return "private, max-age=3600"
	}

	return "no-cache, no-store, must-revalidate"
}

// IsMasterPlaylist reports whether the body is an HLS master (multivariant) playlist.
func IsMasterPlaylist(body []byte) bool {
	return strings.Contains(string(body), "#EXT-X-STREAM-INF:")
}

// NormalizePlaylistForClient adjusts ffmpeg EVENT playlists for browser VOD playback.
func NormalizePlaylistForClient(body []byte) []byte {
	text := string(body)
	text = stripLeadingDiscontinuity(text)

	if strings.Contains(text, "#EXT-X-ENDLIST") {
		text = strings.Replace(text, "#EXT-X-PLAYLIST-TYPE:EVENT", "#EXT-X-PLAYLIST-TYPE:VOD", 1)
	}

	return []byte(text)
}

func stripLeadingDiscontinuity(text string) string {
	lines := strings.Split(text, "\n")
	firstSegment := -1

	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#EXTINF:") {
			firstSegment = index

			break
		}
	}

	if firstSegment <= 0 {
		return text
	}

	for index := firstSegment - 1; index >= 0; index-- {
		if strings.TrimSpace(lines[index]) == "#EXT-X-DISCONTINUITY" {
			lines = append(lines[:index], lines[index+1:]...)

			break
		}
	}

	return strings.Join(lines, "\n")
}
