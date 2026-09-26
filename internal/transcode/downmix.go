package transcode

import (
	"fmt"
	"strings"
)

// DownmixAlgorithm selects how surround audio is folded to stereo.
//
// Values match Jellyfin EncodingOptions.DownMixStereoAlgorithm names used in admin UI.
type DownmixAlgorithm string

const (
	// DownmixNone uses ffmpeg's built-in channel matrix (-ac 2 only).
	DownmixNone DownmixAlgorithm = "none"
	// DownmixAC4 uses Jellyfin AC-4 pan filters (ETSI TS 103 190-1 defaults).
	DownmixAC4 DownmixAlgorithm = "ac4"
	// DownmixDave750 uses Jellyfin Dave750 pan filters.
	DownmixDave750 DownmixAlgorithm = "dave750"
	// DownmixNightmodeDialogue uses Jellyfin NightmodeDialogue pan filters.
	DownmixNightmodeDialogue DownmixAlgorithm = "nightmodeDialogue"
	// DownmixRFC7845 uses Jellyfin RFC7845 pan filters.
	DownmixRFC7845 DownmixAlgorithm = "rfc7845"

	layoutMono   = "mono"
	layoutStereo = "stereo"
	layout21     = "2.1"
	layout30     = "3.0"
	layout40     = "4.0"
	layoutQuad   = "quad"
	layout50     = "5.0"
	layout51     = "5.1"
	layout61     = "6.1"
	layout70     = "7.0"
	layout71     = "7.1"

	minDownmixBoost       = 0.5
	maxDownmixBoost       = 3.0
	defaultNoneBoost      = 2.0
	defaultCustomPanBoost = 1.0
)

type downmixKey struct {
	algo   DownmixAlgorithm
	layout string
}

// Pan strings copied from Jellyfin MediaBrowser.Controller/MediaEncoding/DownMixAlgorithmsHelper.cs.
//
//nolint:gochecknoglobals,lll // mirrors upstream AlgorithmFilterStrings verbatim
var downmixPanFilters = map[downmixKey]string{
	{DownmixDave750, layout51}: "pan=stereo|c0=0.5*c2+0.707*c0+0.707*c4+0.5*c3|c1=0.5*c2+0.707*c1+0.707*c5+0.5*c3",
	{DownmixDave750, layout71}: "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7," +
		"pan=stereo|c0=0.5*c2+0.707*c0+0.707*c4+0.5*c3|c1=0.5*c2+0.707*c1+0.707*c5+0.5*c3",
	{DownmixNightmodeDialogue, layout51}: "pan=stereo|c0=c2+0.30*c0+0.30*c4|c1=c2+0.30*c1+0.30*c5",
	{DownmixNightmodeDialogue, layout71}: "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7," +
		"pan=stereo|c0=c2+0.30*c0+0.30*c4|c1=c2+0.30*c1+0.30*c5",
	{DownmixRFC7845, layout30}:   "pan=stereo|c0=0.414214*c2+0.585786*c0|c1=0.414214*c2+0.585786*c1",
	{DownmixRFC7845, layoutQuad}: "pan=stereo|c0=0.422650*c0+0.366025*c2+0.211325*c3|c1=0.422650*c1+0.366025*c3+0.211325*c2",
	{DownmixRFC7845, layout50}:   "pan=stereo|c0=0.460186*c2+0.650802*c0+0.563611*c3+0.325401*c4|c1=0.460186*c2+0.650802*c1+0.563611*c4+0.325401*c3",
	{DownmixRFC7845, layout51}:   "pan=stereo|c0=0.374107*c2+0.529067*c0+0.458186*c4+0.264534*c5+0.374107*c3|c1=0.374107*c2+0.529067*c1+0.458186*c5+0.264534*c4+0.374107*c3",
	{DownmixRFC7845, layout61}:   "pan=stereo|c0=0.321953*c2+0.455310*c0+0.394310*c5+0.227655*c6+0.278819*c4+0.321953*c3|c1=0.321953*c2+0.455310*c1+0.394310*c6+0.227655*c5+0.278819*c4+0.321953*c3",
	{DownmixRFC7845, layout71}:   "pan=stereo|c0=0.274804*c2+0.388631*c0+0.336565*c6+0.194316*c7+0.336565*c4+0.194316*c5+0.274804*c3|c1=0.274804*c2+0.388631*c1+0.336565*c7+0.194316*c6+0.336565*c5+0.194316*c4+0.274804*c3",
	{DownmixAC4, layout30}:       "pan=stereo|c0=c0+0.707*c2|c1=c1+0.707*c2",
	{DownmixAC4, layout50}:       "pan=stereo|c0=c0+0.707*c2+0.707*c3|c1=c1+0.707*c2+0.707*c4",
	{DownmixAC4, layout51}:       "pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5",
	{DownmixAC4, layout70}: "pan=5.0(side)|c0=c0|c1=c1|c2=c2|c3=0.707*c3+0.707*c5|c4=0.707*c4+0.707*c6," +
		"pan=stereo|c0=c0+0.707*c2+0.707*c3|c1=c1+0.707*c2+0.707*c4",
	{DownmixAC4, layout71}: "pan=5.1(side)|c0=c0|c1=c1|c2=c2|c3=c3|c4=0.707*c4+0.707*c6|c5=0.707*c5+0.707*c7," +
		"pan=stereo|c0=c0+0.707*c2+0.707*c4|c1=c1+0.707*c2+0.707*c5",
}

//nolint:gochecknoglobals // channel-count→layout table mirrors Jellyfin InferChannelLayout
var channelCountLayouts = map[int]string{
	1: layoutMono,
	2: layoutStereo,
	3: layout21,
	4: layout40,
	5: layout50,
	6: layout51,
	7: layout61,
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

// DefaultDownmixBoost returns the Jellyfin-aligned default boost for an algorithm.
func DefaultDownmixBoost(algo DownmixAlgorithm) float64 {
	if algo == DownmixNone {
		return defaultNoneBoost
	}

	return defaultCustomPanBoost
}

// StereoDownmixFilter returns an ffmpeg pan filter for the algorithm and layout, or "" when
// ffmpeg should use its built-in matrix (-ac 2 only).
func StereoDownmixFilter(algo DownmixAlgorithm, channels int, layout string) string {
	if algo == DownmixNone || channels <= 2 {
		return ""
	}

	inferred := InferChannelLayout(channels, layout)

	return downmixPanFilters[downmixKey{algo: algo, layout: inferred}]
}

// ComposeAudioFilter builds the -af chain (pan + volume) for encode/remux audio renditions.
func ComposeAudioFilter(algo DownmixAlgorithm, channels int, layout string, boost float64) string {
	if boost <= 0 {
		boost = DefaultDownmixBoost(algo)
	}

	pan := StereoDownmixFilter(algo, channels, layout)
	volume := volumeFilterValue(boost)
	if pan == "" && volume == "" {
		return ""
	}
	if pan == "" {
		return volume
	}
	if volume == "" {
		return pan
	}

	return pan + "," + volume
}

func volumeFilterValue(boost float64) string {
	if boost == 1.0 {
		return ""
	}

	return fmt.Sprintf("volume=%g", boost)
}
