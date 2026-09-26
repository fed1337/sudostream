package transcode

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	// SegmentTargetSeconds is the nominal HLS segment length shared by every rendition.
	SegmentTargetSeconds = 6.0

	segmentPrefix    = "seg_"
	segmentExt       = ".ts"
	segmentDigits    = 5
	audioGroupID     = "aud"
	mediaPlaylist    = "playlist.m3u8"
	minSegmentLength = 0.1
)

// SegmentTable returns per-segment durations covering the whole media timeline.
//
// Keyframe-derived boundaries are required because stream-copied video can only be split at
// existing keyframes. Transcoded rungs are pinned to the same cut points with
// `-force_key_frames source`, so one table stays valid for every rendition and segment N of
// any rung covers the identical time range.
func SegmentTable(keyframes []float64, durationSeconds, targetSeconds float64) []float64 {
	if durationSeconds <= 0 || targetSeconds <= 0 {
		return nil
	}

	if len(keyframes) == 0 {
		return equalLengthSegments(durationSeconds, targetSeconds)
	}

	segments := keyframeSegments(keyframes, durationSeconds, targetSeconds)
	if len(segments) == 0 {
		return equalLengthSegments(durationSeconds, targetSeconds)
	}

	return segments
}

func keyframeSegments(keyframes []float64, durationSeconds, targetSeconds float64) []float64 {
	segments := make([]float64, 0, int(durationSeconds/targetSeconds)+2) //nolint:mnd // slack
	lastCut := 0.0
	desiredCut := targetSeconds

	for _, keyframe := range keyframes {
		if keyframe < desiredCut || keyframe >= durationSeconds {
			continue
		}

		length := keyframe - lastCut
		if length < minSegmentLength {
			continue
		}

		segments = append(segments, length)
		lastCut = keyframe
		desiredCut = lastCut + targetSeconds
	}

	if remaining := durationSeconds - lastCut; remaining >= minSegmentLength {
		segments = append(segments, remaining)
	}

	return segments
}

func equalLengthSegments(durationSeconds, targetSeconds float64) []float64 {
	count := int(math.Ceil(durationSeconds / targetSeconds))
	if count <= 0 {
		return nil
	}

	segments := make([]float64, count)
	for index := range count - 1 {
		segments[index] = targetSeconds
	}

	segments[count-1] = durationSeconds - targetSeconds*float64(count-1)
	if segments[count-1] < minSegmentLength {
		segments = segments[:count-1]
	}

	return segments
}

// SegmentStart returns the timeline offset of a segment index.
func SegmentStart(segments []float64, index int) float64 {
	start := 0.0
	for position := 0; position < index && position < len(segments); position++ {
		start += segments[position]
	}

	return start
}

// SegmentName formats the on-disk and playlist name for a segment index.
func SegmentName(index int) string {
	return fmt.Sprintf("%s%0*d%s", segmentPrefix, segmentDigits, index, segmentExt)
}

// SegmentIndexFromName parses a segment index out of `seg_00042.ts`.
func SegmentIndexFromName(name string) (int, bool) {
	if !strings.HasPrefix(name, segmentPrefix) || !strings.HasSuffix(name, segmentExt) {
		return 0, false
	}

	digits := strings.TrimSuffix(strings.TrimPrefix(name, segmentPrefix), segmentExt)

	index, err := strconv.Atoi(digits)
	if err != nil || index < 0 {
		return 0, false
	}

	return index, true
}

// BuildMediaPlaylist renders a complete VOD playlist from the segment table alone.
//
// The playlist never consults the filesystem: it is published in full before any ffmpeg run so
// the client gets a seekable timeline immediately and segments are produced on demand.
func BuildMediaPlaylist(segments []float64) []byte {
	target := 1
	for _, length := range segments {
		if ceil := int(math.Ceil(length)); ceil > target {
			target = ceil
		}
	}

	lines := make([]string, 0, len(segments)*2+7) //nolint:mnd // header + footer lines
	lines = append(lines,
		hlsPlaylistHeader,
		hlsPlaylistVersion6,
		"#EXT-X-TARGETDURATION:"+strconv.Itoa(target),
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXT-X-PLAYLIST-TYPE:VOD",
		"#EXT-X-INDEPENDENT-SEGMENTS",
	)

	for index, length := range segments {
		lines = append(lines,
			fmt.Sprintf("#EXTINF:%.6f,", length),
			SegmentName(index),
		)
	}

	lines = append(lines, "#EXT-X-ENDLIST", "")

	return []byte(strings.Join(lines, "\n"))
}

// VariantDir returns the on-demand HLS directory for a target height.
func VariantDir(height int) string {
	return fmt.Sprintf("v%d", height)
}

// AudioDir returns the HLS directory for one alternate audio rendition.
func AudioDir(trackIndex int) string {
	return fmt.Sprintf("a%d", trackIndex)
}

// LadderHeights lists ladder rungs for a source, highest first, including the native height.
func LadderHeights(sourceHeight int) []int {
	rungs := QualityRungs(sourceHeight)
	heights := make([]int, 0, len(rungs)+1)

	if sourceHeight > 0 && !qualityRungIncluded(rungs, sourceHeight) {
		heights = append(heights, sourceHeight)
	}

	for _, rung := range rungs {
		heights = append(heights, rung.Height)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(heights)))

	return heights
}

func qualityRungIncluded(rungs []QualityRung, height int) bool {
	for _, rung := range rungs {
		if rung.Height == height {
			return true
		}
	}

	return false
}

// BuildMasterPlaylist renders the multivariant playlist: video rungs plus alternate audio and
// subtitle rendition groups. Audio is a separate group so a track switch is a client-side
// rendition change instead of a video repackage.
func BuildMasterPlaylist(meta SourceMeta, subtitles []packagedSubtitleTrack) []byte {
	audioTags := audioMediaTags(meta.AudioStreams)
	subtitleTags := subtitleMediaTags(subtitles)
	heights := LadderHeights(meta.Height)
	// header + independent-segments + tags + (STREAM-INF + URI) per rung + trailing blank
	lines := make([]string, 0, 3+len(audioTags)+len(subtitleTags)+2*len(heights)+1)
	lines = append(lines, hlsPlaylistHeader, hlsPlaylistVersion6, "#EXT-X-INDEPENDENT-SEGMENTS")
	lines = append(lines, audioTags...)
	lines = append(lines, subtitleTags...)

	attrs := streamInfSuffix(len(meta.AudioStreams) > 0, len(subtitles) > 0)
	for _, height := range heights {
		lines = append(
			lines,
			streamInfLine(meta, height, attrs),
			VariantDir(height)+"/"+mediaPlaylist,
		)
	}

	lines = append(lines, "")

	return []byte(strings.Join(lines, "\n"))
}

func streamInfSuffix(hasAudio, hasSubtitles bool) string {
	attrs := ""
	if hasAudio {
		attrs += `,AUDIO="` + audioGroupID + `"`
	}

	if hasSubtitles {
		attrs += `,SUBTITLES="` + subtitleGroupID + `"`
	}

	return attrs
}

func streamInfLine(meta SourceMeta, height int, attrs string) string {
	rung := ladderRung(meta.Height, height)

	bandwidth := bitrateToInt(rung.Bitrate)
	if bandwidth <= 0 {
		bandwidth = defaultStubBandwidth
	}

	width := scaledWidth(meta, height)

	return fmt.Sprintf(
		`#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS="%s"%s`,
		bandwidth,
		width,
		height,
		hlsCodecsAttr,
		attrs,
	)
}

// scaledWidth keeps the source aspect ratio so the client reports sane rendition dimensions.
func scaledWidth(meta SourceMeta, height int) int {
	if meta.Width > 0 && meta.Height > 0 {
		width := meta.Width * height / meta.Height
		if width%2 != 0 {
			width++
		}

		if width > 0 {
			return width
		}
	}

	return height * 16 / 9 //nolint:mnd // 16:9 fallback
}

func ladderRung(sourceHeight, height int) QualityRung {
	for _, rung := range QualityRungs(sourceHeight) {
		if rung.Height == height {
			return rung
		}
	}

	return FastStartRung(sourceHeight)
}

func audioMediaTags(streams []AudioStream) []string {
	tags := make([]string, 0, len(streams))
	for index, stream := range streams {
		defaultFlag := "NO"
		if index == 0 {
			defaultFlag = "YES"
		}

		language := stream.Language
		if language == "" {
			language = "und"
		}

		tags = append(tags, fmt.Sprintf(
			`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="%s",NAME="%s",LANGUAGE="%s",`+
				`DEFAULT=%s,AUTOSELECT=YES,CHANNELS="2",URI="%s"`,
			audioGroupID,
			escapeAttr(stream.Label),
			escapeAttr(language),
			defaultFlag,
			path.Join(AudioDir(index), mediaPlaylist),
		))
	}

	return tags
}

func subtitleMediaTags(tracks []packagedSubtitleTrack) []string {
	tags := make([]string, 0, len(tracks))
	for index, track := range tracks {
		defaultFlag := "NO"
		if index == 0 {
			defaultFlag = "YES"
		}

		tags = append(tags, fmt.Sprintf(
			`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="%s",NAME="%s",`+
				`DEFAULT=%s,AUTOSELECT=YES,URI="%s"`,
			subtitleGroupID,
			escapeAttr(track.DisplayName),
			defaultFlag,
			track.RelPlaylist,
		))
	}

	return tags
}

func escapeAttr(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), `"`, "'")
}

// ApplyMasterPlaylistDefaultAudio rewrites DEFAULT= on TYPE=AUDIO lines for user language policy.
func ApplyMasterPlaylistDefaultAudio(playlist []byte, defaultIndex int) []byte {
	if defaultIndex <= 0 {
		return playlist
	}

	lines := strings.Split(string(playlist), "\n")
	audioLine := 0
	for index, line := range lines {
		if !strings.Contains(line, "TYPE=AUDIO") {
			continue
		}

		if audioLine == defaultIndex {
			lines[index] = replacePlaylistDefaultFlag(line, "YES")
		} else {
			lines[index] = replacePlaylistDefaultFlag(line, "NO")
		}

		audioLine++
	}

	return []byte(strings.Join(lines, "\n"))
}

func replacePlaylistDefaultFlag(line, value string) string {
	if strings.Contains(line, "DEFAULT=") {
		return defaultFlagPattern.ReplaceAllString(line, "DEFAULT="+value)
	}

	return line
}

//nolint:gochecknoglobals // compiled once for master playlist rewrite
var defaultFlagPattern = regexp.MustCompile(`DEFAULT=(YES|NO)`)

func bitrateToInt(bitrate string) int {
	value := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(bitrate)), "k")
	if value == "" {
		return 0
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}

	return parsed * 1000 //nolint:mnd // kilobits to bits
}
