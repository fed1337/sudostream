package transcode

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const sourceDurationMarker = ".source_duration"

// WriteSourceDuration stores the probed media duration for playback UI during progressive transcode.
func WriteSourceDuration(outDir string, seconds float64) error {
	if outDir == "" || seconds <= 0 {
		return nil
	}

	path := filepath.Join(outDir, sourceDurationMarker)
	//nolint:mnd // permissions for transcode cache file
	err := os.WriteFile(path, []byte(strconv.FormatFloat(seconds, 'f', 3, 64)), 0o600)
	if err != nil {
		return fmt.Errorf("write source duration: %w", err)
	}

	return nil
}

// ReadSourceDuration returns the probed duration written at transcode start.
func ReadSourceDuration(outDir string) (float64, bool) {
	if outDir == "" {
		return 0, false
	}

	raw, err := os.ReadFile(filepath.Join(outDir, sourceDurationMarker))
	if err != nil {
		return 0, false
	}

	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil || seconds <= 0 {
		return 0, false
	}

	return seconds, true
}

func parseProbeDuration(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return 0
	}

	return seconds
}

func parseProbeBitrate(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	bitrate, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || bitrate <= 0 {
		return 0
	}

	return bitrate
}
