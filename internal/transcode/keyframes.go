package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	keyframeMarker = ".keyframes.json"
	// keyframeProbeTimeout bounds packet-index scans. Large HEVC/MKV files can take
	// minutes to dump every packet; equal-length segments are the documented fallback.
	keyframeProbeTimeout = 20 * time.Second
)

// ProbeKeyframes lists video keyframe presentation timestamps in seconds.
//
// Packet flags are read without decoding, so the cost is a sequential pass over the
// container. Sources without usable timestamps — or probes that exceed
// keyframeProbeTimeout — yield an empty/error result; callers fall back to equal-length
// segments.
func ProbeKeyframes(ctx context.Context, mediaPath string) ([]float64, error) {
	probeCtx, cancel := context.WithTimeout(ctx, keyframeProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(
		probeCtx,
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags",
		"-of", "csv=p=0",
		mediaPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe keyframes %s: %w", filepath.Base(mediaPath), err)
	}

	return parseKeyframeCSV(string(output)), nil
}

// parseKeyframeCSV extracts keyframe timestamps from `pts_time,flags` rows.
func parseKeyframeCSV(raw string) []float64 {
	lines := strings.Split(raw, "\n")
	keyframes := make([]float64, 0, len(lines))
	previous := -1.0

	for _, line := range lines {
		seconds, ok := parseKeyframeRow(line)
		if !ok || seconds <= previous {
			continue
		}

		keyframes = append(keyframes, seconds)
		previous = seconds
	}

	return keyframes
}

func parseKeyframeRow(line string) (float64, bool) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) < 2 { //nolint:mnd // pts_time,flags pair
		return 0, false
	}

	if !strings.Contains(fields[1], "K") {
		return 0, false
	}

	seconds, err := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
	if err != nil || seconds < 0 {
		return 0, false
	}

	return seconds, true
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
