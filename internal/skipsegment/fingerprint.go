package skipsegment

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	uint32Bytes           = 4
	maxFFmpegErrLogChars  = 240
	chromaprintSampleRate = "11025"
)

// ErrNoAudio is returned when ffmpeg cannot produce a chromaprint (no audio track).
var ErrNoAudio = errors.New("no audio stream for chromaprint")

var (
	errScanWindowTooShort = errors.New("scan window too short")
	errChromaprintRawLen  = errors.New("chromaprint raw length invalid")
)

// ExtractFingerprint runs jellyfin-ffmpeg chromaprint over the intro scan window.
// Uses raw little-endian uint32 output (no compressed decode). Absolute path must
// already be resolved via mediafs.
func ExtractFingerprint(
	ctx context.Context,
	absPath string,
	durationSeconds float64,
) ([]uint32, float64, error) {
	scanWindowSec := IntroScanWindowSec(durationSeconds)
	if scanWindowSec < float64(minIntroSec) {
		return nil, scanWindowSec, fmt.Errorf("%w: %.1fs", errScanWindowTooShort, scanWindowSec)
	}

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-nostats",
		"-i", absPath,
		"-t", formatSeconds(scanWindowSec),
		"-vn",
		"-map", "0:a:0?",
		"-ac", "1",
		"-ar", chromaprintSampleRate,
		"-f", "chromaprint",
		"-fp_format", "raw",
		"-algorithm", "1",
		"pipe:1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr != nil {
		message := strings.TrimSpace(stderr.String())
		if strings.Contains(message, "Output file does not contain any stream") ||
			strings.Contains(message, "matches no streams") {
			return nil, scanWindowSec, ErrNoAudio
		}

		return nil, scanWindowSec, fmt.Errorf("ffmpeg chromaprint: %w (%s)", runErr, truncateErr(message))
	}

	raw := stdout.Bytes()
	if len(raw) < uint32Bytes || len(raw)%uint32Bytes != 0 {
		return nil, scanWindowSec, fmt.Errorf("%w: %d", errChromaprintRawLen, len(raw))
	}

	items := make([]uint32, len(raw)/uint32Bytes)
	for index := range items {
		items[index] = binary.LittleEndian.Uint32(raw[index*uint32Bytes : (index+1)*uint32Bytes])
	}

	return items, scanWindowSec, nil
}

func formatSeconds(seconds float64) string {
	return strconv.FormatFloat(seconds, 'f', 3, 64)
}

func truncateErr(message string) string {
	message = strings.ReplaceAll(message, "\n", " ")
	if len(message) > maxFFmpegErrLogChars {
		return message[:maxFFmpegErrLogChars]
	}

	return message
}
