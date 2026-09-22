package transcode

import (
	"strings"
)

// DownmixAlgorithm selects how surround audio is folded to stereo.
//
// Values match Jellyfin EncodingOptions.DownMixStereoAlgorithm names used in admin UI.
type DownmixAlgorithm string

const (
	// DownmixNone uses ffmpeg's built-in channel matrix (-ac 2 only). No volume boost.
	DownmixNone DownmixAlgorithm = "none"
	// DownmixAC4 uses Jellyfin AC-4 pan filters (ETSI TS 103 190-1 defaults).
	DownmixAC4 DownmixAlgorithm = "ac4"

	layoutMono   = "mono"
	layoutStereo = "stereo"
	layout30     = "3.0"
	layout50     = "5.0"
	layout51     = "5.1"
	layout70     = "7.0"
	layout71     = "7.1"
)

// AC-4 pan filters copied from Jellyfin
// MediaBrowser.Controller/MediaEncoding/DownMixAlgorithmsHelper.cs (Ac4 entries).
//
//nolint:gochecknoglobals // layout→filter map mirrors Jellyfin constants
var ac4PanFilters = map[string]string{
	layout30: "pan=stereo|c0=c0+0.707*c2|c1=c1+0.707*c2",
	layout50: "pan=stereo|c0=c0+0.707*c2+0.707*c3|c1=c1+0.707*c2+0.707*c4",
	layout51: "pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5",
	layout70: "pan=5.0(side)|c0=c0|c1=c1|c2=c2|c3=0.707*c3+0.707*c5|c4=0.707*c4+0.707*c6," +
		"pan=stereo|c0=c0+0.707*c2+0.707*c3|c1=c1+0.707*c2+0.707*c4",
	layout71: "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7," +
		"pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5",
}

//nolint:gochecknoglobals // channel-count→layout table mirrors Jellyfin InferChannelLayout
var channelCountLayouts = map[int]string{
	1: layoutMono,
	2: layoutStereo,
	3: "2.1",
	4: "4.0",
	5: layout50,
	6: layout51,
	7: "6.1",
	8: layout71,
}

// InferChannelLayout mirrors Jellyfin DownMixAlgorithmsHelper.InferChannelLayout.
func InferChannelLayout(channels int, layout string) string {
	layout = strings.TrimSpace(layout)
	if layout != "" {
		return layout
	}

	return channelCountLayouts[channels]
}

// StereoDownmixFilter returns an ffmpeg -af pan string for AC-4, or "" for none / ≤2ch /
// unsupported layouts (caller still applies -ac 2).
func StereoDownmixFilter(algo DownmixAlgorithm, channels int, layout string) string {
	if algo != DownmixAC4 || channels <= 2 {
		return ""
	}

	inferred := InferChannelLayout(channels, layout)

	return ac4PanFilters[inferred]
}
