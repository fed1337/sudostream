package transcode

import (
	"strings"
)

// PackagingMode describes how the native-resolution rung is produced.
type PackagingMode string

const (
	// PackagingRemux indicates the source video is stream-copied into HLS segments.
	PackagingRemux PackagingMode = "remux"
	// PackagingTranscode indicates every rung is re-encoded.
	PackagingTranscode PackagingMode = "transcode"

	codecH264 = "h264"
	codecAVC1 = "avc1"
)

// RemuxEligible reports whether the source video can be copied into HLS without re-encode.
//
// Audio is always re-encoded to stereo AAC in its own rendition, so only the video stream
// matters here. HDR / wide-gamut / 10-bit sources must transcode so tone mapping can run.
func RemuxEligible(source SourceInfo) bool {
	return source.VideoStreamCount == 1 &&
		isRemuxVideoCodec(source.VideoCodec) &&
		!source.NeedsToneMap()
}

func isRemuxVideoCodec(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case codecH264, codecAVC1:
		return true
	default:
		return false
	}
}
