package transcode

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

var (
	errUnsupportedSubtitleEncoding = errors.New("unsupported subtitle encoding")
	errInvalidSRTTimestamp         = errors.New("invalid srt timestamp")
)

var srtBlockPattern = regexp.MustCompile(
	`(?s)\d+\s*\n(\d{2}:\d{2}:\d{2},\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2},\d{3})\s*\n(.*?)(?:\n\n|\z)`,
)

// convertSRTFileToWebVTT parses a SubRip sidecar and writes WebVTT without ffmpeg.
func convertSRTFileToWebVTT(inputPath, outputPath string) error {
	raw, err := readSidecarSubtitleFile(inputPath)
	if err != nil {
		return fmt.Errorf("read srt: %w", err)
	}

	text, err := decodeSubtitleText(raw)
	if err != nil {
		return err
	}

	vtt, err := srtTextToWebVTT(text)
	if err != nil {
		return err
	}

	//nolint:mnd // permissions
	err = os.WriteFile(outputPath, []byte(vtt), 0o600)
	if err != nil {
		return fmt.Errorf("write vtt: %w", err)
	}

	return nil
}

func decodeSubtitleText(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("decode subtitle text: %w", errSubtitleNoCues)
	}

	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})

	candidates := []string{}
	if utf8.Valid(raw) {
		candidates = append(candidates, string(raw))
	}

	for _, label := range []string{"cp1251", "cp866", "iso-8859-1"} {
		text, decodeErr := decodeWithEncoding(raw, label)
		if decodeErr != nil {
			continue
		}

		candidates = append(candidates, text)
	}

	for _, text := range candidates {
		vtt, err := srtTextToWebVTT(text)
		if err == nil && strings.Contains(vtt, "-->") {
			return text, nil
		}
	}

	return "", fmt.Errorf("decode subtitle text: %w", errSubtitleNoCues)
}

func decodeWithEncoding(raw []byte, encoding string) (string, error) {
	var decoder transform.Transformer

	switch encoding {
	case "cp1251":
		decoder = charmap.Windows1251.NewDecoder()
	case "cp866":
		decoder = charmap.CodePage866.NewDecoder()
	case "iso-8859-1":
		decoder = charmap.ISO8859_1.NewDecoder()
	default:
		return "", fmt.Errorf("%w: %s", errUnsupportedSubtitleEncoding, encoding)
	}

	decoded, _, err := transform.Bytes(decoder, raw)
	if err != nil {
		return "", fmt.Errorf("decode %s: %w", encoding, err)
	}

	return string(decoded), nil
}

func srtTextToWebVTT(text string) (string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	matches := srtBlockPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("parse srt: %w", errSubtitleNoCues)
	}

	lines := []string{"WEBVTT", ""}
	for _, match := range matches {
		if len(match) != 4 { //nolint:mnd // regexp capture groups
			continue
		}

		start, startErr := srtTimestampToWebVTT(match[1])
		end, endErr := srtTimestampToWebVTT(match[2])
		if startErr != nil || endErr != nil {
			continue
		}

		body := strings.TrimSpace(match[3])
		if body == "" {
			continue
		}

		lines = append(lines, fmt.Sprintf("%s --> %s", start, end), body, "")
	}

	if len(lines) <= 2 { //nolint:mnd // WEBVTT header only
		return "", fmt.Errorf("parse srt: %w", errSubtitleNoCues)
	}

	return strings.Join(lines, "\n"), nil
}

func srtTimestampToWebVTT(value string) (string, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 { //nolint:mnd // timestamp pair
		return "", fmt.Errorf("%w: %q", errInvalidSRTTimestamp, value)
	}

	millis, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid srt millis %q: %w", parts[1], err)
	}

	return fmt.Sprintf("%s.%03d", parts[0], millis), nil
}

// NormalizeSubtitleUpload accepts client-uploaded VTT or SRT bytes and returns WebVTT.
func NormalizeSubtitleUpload(raw []byte, filename string) ([]byte, error) {
	raw = bytes.TrimSpace(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
	if len(raw) == 0 {
		return nil, fmt.Errorf("normalize subtitle upload: %w", errSubtitleNoCues)
	}

	lower := strings.ToLower(filename)
	text := string(raw)
	if strings.HasPrefix(strings.TrimSpace(text), "WEBVTT") || strings.HasSuffix(lower, ".vtt") {
		if !strings.HasPrefix(strings.TrimSpace(text), "WEBVTT") {
			text = "WEBVTT\n\n" + strings.TrimSpace(text)
		}
		if !strings.Contains(text, "-->") {
			return nil, fmt.Errorf("normalize subtitle upload: %w", errSubtitleNoCues)
		}

		return []byte(text), nil
	}

	decoded, err := decodeSubtitleText(raw)
	if err != nil {
		return nil, fmt.Errorf("normalize subtitle upload: %w", err)
	}

	vtt, err := srtTextToWebVTT(decoded)
	if err != nil {
		return nil, fmt.Errorf("normalize subtitle upload: %w", err)
	}

	return []byte(vtt), nil
}

func readSidecarSubtitleFile(inputPath string) ([]byte, error) {
	// inputPath is a resolved sidecar beside media.

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read sidecar subtitle: %w", err)
	}

	return raw, nil
}
