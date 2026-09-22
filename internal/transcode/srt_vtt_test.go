package transcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"golang.org/x/text/encoding/charmap"
)

func TestConvertSRTFileToWebVTT_UTF8(t *testing.T) {
	t.Parallel()

	allure.Test(t, "utf-8 srt converts to webvtt cues", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		inputPath := filepath.Join(root, "movie.srt")
		outputPath := filepath.Join(root, "movie.vtt")

		err := os.WriteFile(
			inputPath,
			[]byte("1\n00:00:01,000 --> 00:00:04,000\nHello world\n\n"),
			0o600,
		)
		if err != nil {
			t.Fatalf("write srt: %v", err)
		}

		err = convertSRTFileToWebVTT(inputPath, outputPath)
		if err != nil {
			t.Fatalf("convert srt: %v", err)
		}

		raw, err := os.ReadFile(outputPath) //nolint:gosec // test temp dir
		if err != nil {
			t.Fatalf("read vtt: %v", err)
		}

		body := string(raw)
		if !strings.Contains(body, "WEBVTT") || !strings.Contains(body, "Hello world") {
			t.Fatalf("unexpected vtt body:\n%s", body)
		}
	})
}

func TestConvertSRTFileToWebVTT_CP1251(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cp1251 srt converts without ffmpeg", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		inputPath := filepath.Join(root, "movie.srt")
		outputPath := filepath.Join(root, "movie.vtt")

		encoded, err := charmap.Windows1251.NewEncoder().Bytes([]byte(
			"1\r\n00:00:01,000 --> 00:00:04,000\r\nПривет мир\r\n\r\n",
		))
		if err != nil {
			t.Fatalf("encode cp1251: %v", err)
		}

		err = os.WriteFile(inputPath, encoded, 0o600)
		if err != nil {
			t.Fatalf("write srt: %v", err)
		}

		err = convertSRTFileToWebVTT(inputPath, outputPath)
		if err != nil {
			t.Fatalf("convert srt: %v", err)
		}

		raw, err := os.ReadFile(outputPath) //nolint:gosec // test temp dir
		if err != nil {
			t.Fatalf("read vtt: %v", err)
		}

		body := string(raw)
		if !strings.Contains(body, "Привет мир") {
			t.Fatalf("expected cyrillic cues, got:\n%s", body)
		}
	})
}
