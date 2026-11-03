package transcode

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// maxRunSegments bounds how far ahead of the request a single ffmpeg run may encode. It is the
// throttle: without it one seek transcodes the rest of the film for a client that may never
// watch it. Reaching the bound simply ends the run; the next request restarts one seek later.
const maxRunSegments = 12

const (
	// copySeekPreroll puts the demuxer seek safely before the target keyframe. Demuxers do not
	// agree on where a seek lands (Matroska plus B-frames undershoots by a GOP, MPEG-TS
	// overshoots to the next keyframe), so the input seek only has to land at or before it.
	copySeekPreroll = 1.5
	// copySeekTrim is how far before the target keyframe the output seek cuts. ffmpeg compares
	// DTS there, and a keyframe's DTS precedes its PTS by the B-frame reorder delay, so a cut
	// exactly on the keyframe would drop it.
	copySeekTrim = 0.5
	// keyframeMatchTolerance absorbs float drift between summed segment lengths and probed PTS.
	keyframeMatchTolerance = 0.002
	// audioRenditionKbps is the AAC rate of every audio rendition; variants advertise it too.
	audioRenditionKbps = 192
)

// segmentRun describes one on-demand ffmpeg invocation producing a contiguous run of segments.
type segmentRun struct {
	mediaPath  string
	variantDir string
	meta       SourceMeta
	// keyframes are the normalized source keyframe times; only copy runs need them.
	keyframes    []float64
	variant      Variant
	startIndex   int
	encoder      VideoEncoder
	render       string
	toneMap      ToneMapMode
	toneAlgo     ToneMappingAlgorithm
	downmix      DownmixAlgorithm
	downmixBoost float64
}

// startSeconds is the timeline offset of the first segment this run produces.
func (r segmentRun) startSeconds() float64 {
	return SegmentStart(r.meta.Segments, r.startIndex)
}

// endIndex is the first segment this run will not produce.
func (r segmentRun) endIndex() int {
	return min(r.startIndex+maxRunSegments, len(r.meta.Segments))
}

// cutTimes lists the segment boundaries after the run's start offset.
//
// ffmpeg rebases input timestamps to zero when seeking with -ss, and -output_ts_offset is
// applied by the inner muxer after the split decision, so both the segment muxer and the
// encoder see times relative to the run start.
func (r segmentRun) cutTimes() []float64 {
	elapsed := 0.0
	end := r.endIndex()
	cuts := make([]float64, 0, max(end-r.startIndex-1, 0))

	for index := r.startIndex; index < end-1; index++ {
		elapsed += r.meta.Segments[index]
		cuts = append(cuts, elapsed)
	}

	return cuts
}

// runSeconds is the throttled run length, or 0 when the run continues to end of file.
func (r segmentRun) runSeconds() float64 {
	end := r.endIndex()
	if end >= len(r.meta.Segments) {
		return 0
	}

	total := 0.0
	for index := r.startIndex; index < end; index++ {
		total += r.meta.Segments[index]
	}

	return total
}

// canCopyVideo reports whether the rung is the untouched source resolution of a copyable stream.
func (r segmentRun) canCopyVideo() bool {
	return r.variant.Kind == VariantVideo && r.meta.copiesVideoAt(r.variant.Value)
}

// copyTrim returns how far before the run start a copy run's output seek cuts, or 0 when the
// plain input seek must stay: transcoded runs (frame-accurate already), runs from zero, and
// starts that are not a probed keyframe (equal-length fallback) cannot use the trim.
func (r segmentRun) copyTrim() float64 {
	start := r.startSeconds()
	if !r.canCopyVideo() || start <= 0 {
		return 0
	}

	position := sort.SearchFloat64s(r.keyframes, start-keyframeMatchTolerance)
	if position >= len(r.keyframes) ||
		math.Abs(r.keyframes[position]-start) > keyframeMatchTolerance {
		return 0
	}

	trim := min(copySeekTrim, start)
	if position > 0 {
		// The previous keyframe must stay before the cut, or the run starts one GOP early.
		trim = min(trim, (start-r.keyframes[position-1])/2) //nolint:mnd // midpoint
	}

	return trim
}

// buildSegmentArgs renders the ffmpeg argv for one on-demand run.
//
// The segment muxer is driven by the shared cut table rather than ffmpeg's own pacing, so
// segment N of every rendition covers the identical time range and the client can switch
// quality or audio mid-stream without a timeline rebase.
func buildSegmentArgs(run segmentRun) []string {
	cuts := run.cutTimes()
	start := run.startSeconds()

	//nolint:mnd // ffmpeg argv capacity estimate
	args := make([]string, 0, 48)
	args = append(args, "-v", "warning", "-nostdin")

	if run.variant.Kind == VariantVideo && !run.canCopyVideo() {
		args = appendHWGlobalArgs(args, run.encoder, run.render)
	}

	trim := run.copyTrim()
	tsOffset := start

	if trim > 0 {
		// Stream copy can only start on a keyframe, and the demuxer seek alone lands on the
		// wrong one. Seek early, then drop everything before the target keyframe on output;
		// copy skips leading non-keyframes, so the first packet is exactly the target. Output
		// times are rebased to the output seek point, hence the offset is start-trim.
		inputSeek := max(start-copySeekPreroll, 0)
		if inputSeek > 0 {
			args = append(args, "-ss", formatSeconds(inputSeek))
		}

		args = append(args, "-i", run.mediaPath, "-ss", formatSeconds(start-inputSeek-trim))
		tsOffset = start - trim
	} else {
		if start > 0 {
			args = append(args, "-ss", formatSeconds(start))
		}

		args = append(args, "-i", run.mediaPath)
	}

	if length := run.runSeconds(); length > 0 {
		// -t counts from the output seek point, which sits trim seconds before the start.
		args = append(args, "-t", formatSeconds(length+trim))
	}

	if run.variant.Kind == VariantAudio {
		args = appendAudioRenditionArgs(args, run)
	} else {
		args = appendVideoRenditionArgs(args, run, cuts)
	}

	// output_ts_offset restores the seek position on the shared timeline, so a run started
	// mid-file produces the same timestamps as one started at zero. The mpegts muxer would
	// otherwise add its default 1.4s startup delay on top of that offset.
	args = append(args,
		"-output_ts_offset", formatSeconds(tsOffset),
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-avoid_negative_ts", "disabled",
	)

	return append(args, appendSegmentMuxerArgs(run, cuts)...)
}

func appendVideoRenditionArgs(args []string, run segmentRun, cuts []float64) []string {
	args = append(args, "-map", "0:v:0", "-an", "-sn", "-dn")

	if run.canCopyVideo() {
		return append(args, "-c:v", "copy")
	}

	args = appendVideoEncodeArgs(
		args,
		run.encoder,
		ladderRung(run.meta.Height, run.variant.Value),
		run.toneMap,
		run.toneAlgo,
	)

	if len(cuts) > 0 {
		// Forced IDR frames at the shared cut points keep every encoded rung splittable at the
		// exact boundaries the playlist advertises.
		args = append(args, "-force_key_frames", joinSeconds(cuts))
	}

	return append(args, "-sc_threshold", "0")
}

func appendAudioRenditionArgs(args []string, run segmentRun) []string {
	stream := run.meta.AudioStreams[run.variant.Value]
	args = append(args,
		"-map", mapInputStream(stream.Index),
		"-vn", "-sn", "-dn",
	)

	if filter := ComposeAudioFilter(
		run.downmix,
		stream.Channels,
		stream.ChannelLayout,
		run.downmixBoost,
	); filter != "" {
		args = append(args, "-af", filter)
	}

	return append(args,
		"-c:a", "aac",
		"-ac", "2",
		"-b:a", strconv.Itoa(audioRenditionKbps)+"k",
	)
}

func appendSegmentMuxerArgs(run segmentRun, cuts []float64) []string {
	args := []string{
		"-f", "segment",
		"-segment_format", "mpegts",
		"-segment_time_delta", "0.1",
		"-segment_start_number", strconv.Itoa(run.startIndex),
		"-segment_list_type", "flat",
		"-segment_list", filepath.Join(run.variantDir, "segments.txt"),
	}

	if len(cuts) > 0 {
		args = append(args, "-segment_times", joinSeconds(cuts))
	}

	return append(args, filepath.Join(run.variantDir, segmentPrefix+"%05d"+segmentExt))
}

func formatSeconds(seconds float64) string {
	return strconv.FormatFloat(seconds, 'f', 6, 64)
}

func joinSeconds(values []float64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, formatSeconds(value))
	}

	return strings.Join(parts, ",")
}

// mapInputStream selects a source stream by ffprobe global index (not 0:a:N ordinal).
func mapInputStream(streamIndex int) string {
	return fmt.Sprintf("0:%d", streamIndex)
}
