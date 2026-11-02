package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	keyframeMarker = ".keyframes.json"
	// keyframeProbeTimeout bounds packet-index scans. Large HEVC/MKV files can take
	// minutes to dump every packet; equal-length segments are the documented fallback.
	keyframeProbeTimeout = 20 * time.Second
	bitsPerByte          = 8
)

// PacketIndex is one packet-level pass over the first video stream: keyframe times build the
// segment table and packet sizes give the stream-copied rung its real bandwidth.
type PacketIndex struct {
	Keyframes []float64
	packets   []packetSample
}

type packetSample struct {
	pts  float64
	size int64
}

// ProbeVideoPackets indexes video packet timestamps, sizes, and keyframe flags.
//
// Packet flags are read without decoding, so the cost is a sequential pass over the
// container. Sources without usable timestamps — or probes that exceed
// keyframeProbeTimeout — yield an empty/error result; callers fall back to equal-length
// segments.
func ProbeVideoPackets(ctx context.Context, mediaPath string) (PacketIndex, error) {
	probeCtx, cancel := context.WithTimeout(ctx, keyframeProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		probeCtx,
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,size,flags",
		"-of", "csv=p=0",
		mediaPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return PacketIndex{}, fmt.Errorf("ffprobe packets %s: %w", filepath.Base(mediaPath), err)
	}

	return parsePacketCSV(string(output)), nil
}

// parsePacketCSV reads `pts_time,size,flags` rows (ffprobe's fixed packet field order).
// Keyframes keep only strictly increasing timestamps, dropping B-frame reorder noise.
func parsePacketCSV(raw string) PacketIndex {
	lines := strings.Split(raw, "\n")
	index := PacketIndex{
		Keyframes: make([]float64, 0, len(lines)/24), //nolint:mnd // ~1 keyframe per second
		packets:   make([]packetSample, 0, len(lines)),
	}
	previous := -1.0

	for _, line := range lines {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 3 { //nolint:mnd // pts_time,size,flags
			continue
		}

		seconds, err := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
		if err != nil {
			continue
		}

		size, err := strconv.ParseInt(strings.TrimSpace(fields[1]), 10, 64)
		if err == nil && size > 0 {
			index.packets = append(index.packets, packetSample{pts: seconds, size: size})
		}

		if !strings.Contains(fields[2], "K") || seconds < 0 || seconds <= previous {
			continue
		}

		index.Keyframes = append(index.Keyframes, seconds)
		previous = seconds
	}

	return index
}

// normalized shifts packet times onto the timeline ffmpeg writes: without -copyts it rebases
// output by the container start_time, and -ss is relative to it too. A table built from raw
// PTS lands every copy seek on the wrong keyframe whenever start_time is non-zero.
func (p PacketIndex) normalized(startSeconds float64) PacketIndex {
	if startSeconds == 0 {
		return p
	}

	shifted := PacketIndex{
		Keyframes: make([]float64, 0, len(p.Keyframes)),
		packets:   make([]packetSample, len(p.packets)),
	}

	for _, keyframe := range p.Keyframes {
		if value := keyframe - startSeconds; value >= 0 {
			shifted.Keyframes = append(shifted.Keyframes, value)
		}
	}

	for position, packet := range p.packets {
		shifted.packets[position] = packetSample{pts: packet.pts - startSeconds, size: packet.size}
	}

	return shifted
}

// segmentBitrates returns the peak and mean video bitrate (bits/s) over the segment table.
// Segments shorter than half the target are skipped for the peak so a short tail cannot
// dominate it.
func (p PacketIndex) segmentBitrates(segments []float64) (int64, int64) {
	if len(p.packets) == 0 || len(segments) == 0 {
		return 0, 0
	}

	starts := make([]float64, len(segments))
	total := 0.0

	for position, length := range segments {
		starts[position] = total
		total += length
	}

	bytes := make([]int64, len(segments))
	sum := int64(0)

	for _, packet := range p.packets {
		position := max(sort.SearchFloat64s(starts, packet.pts+1e-9)-1, 0) //nolint:mnd // epsilon
		bytes[position] += packet.size
		sum += packet.size
	}

	if total <= 0 {
		return 0, 0
	}

	average := int64(float64(sum*bitsPerByte) / total)
	peak := int64(0)

	for position, length := range segments {
		if length < SegmentTargetSeconds/2 {
			continue
		}

		peak = max(peak, int64(float64(bytes[position]*bitsPerByte)/length))
	}

	return max(peak, average), average
}

// WriteKeyframes caches keyframe timestamps beside the transcode output.
func WriteKeyframes(outDir string, keyframes []float64) error {
	raw, err := json.Marshal(keyframes)
	if err != nil {
		return fmt.Errorf("marshal keyframes: %w", err)
	}

	//nolint:mnd // permissions
	err = os.WriteFile(filepath.Join(outDir, keyframeMarker), raw, 0o600)
	if err != nil {
		return fmt.Errorf("write keyframes: %w", err)
	}

	return nil
}

// ReadKeyframes loads cached keyframe timestamps.
func ReadKeyframes(outDir string) ([]float64, bool) {
	raw, err := os.ReadFile(filepath.Join(outDir, keyframeMarker))
	if err != nil {
		return nil, false
	}

	var keyframes []float64

	err = json.Unmarshal(raw, &keyframes)
	if err != nil {
		return nil, false
	}

	return keyframes, true
}
