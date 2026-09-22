package transcode

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testTagTitle             = "title"
	testCodecAAC             = "aac"
	testAudioLabelEnglish    = "English"
	testAudioLabelJapanese   = "Japanese"
	testAudioLabelCommentary = "Commentary"
	testLangEng              = "eng"
	testEncoderBoomStderr    = "encoder boom"
)

func TestAudioLabel_PrefersTitleLanguageAndChannels(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"audio label prefers title then language then channels",
		func(a *allure.Context) {
			t := a.T()
			withTitle := probeStream{
				Index:    2,
				Channels: 6,
				Tags:     map[string]string{testTagTitle: " Commentary "},
			}
			if got := audioLabel(withTitle); got != testAudioLabelCommentary {
				t.Fatalf("title: got %q", got)
			}

			withLang := probeStream{
				Index:    1,
				Channels: 2,
				Tags:     map[string]string{"language": testLangEng},
			}
			if got := audioLabel(withLang); got != testLangEng {
				t.Fatalf("language: got %q", got)
			}

			withChannels := probeStream{Index: 3, Channels: 2}
			if got := audioLabel(withChannels); got != "Audio 3 (2 ch)" {
				t.Fatalf("channels: got %q", got)
			}

			fallback := probeStream{Index: 0}
			if got := audioLabel(fallback); got != "Audio 0" {
				t.Fatalf("fallback: got %q", got)
			}
		},
	)
}

func TestSubtitleLabel_PrefersTitleLanguageAndIndex(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"subtitle label prefers title then language then stream index",
		func(a *allure.Context) {
			t := a.T()
			withTitle := probeStream{
				Index: 1,
				Tags:  map[string]string{testTagTitle: " English "},
			}
			if got := subtitleLabel(withTitle); got != testAudioLabelEnglish {
				t.Fatalf("title: got %q", got)
			}

			withLang := probeStream{
				Index: 2,
				Tags:  map[string]string{"language": "spa"},
			}
			if got := subtitleLabel(withLang); got != "spa" {
				t.Fatalf("language: got %q", got)
			}

			fallback := probeStream{Index: 4}
			if got := subtitleLabel(fallback); got != "Subtitles 4" {
				t.Fatalf("fallback: got %q", got)
			}
		},
	)
}

func TestProbeSource_ParsesSyntheticMedia(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ProbeSource reads ffprobe output for synthetic media", func(a *allure.Context) {
		t := a.T()
		_, err := exec.LookPath("ffprobe")
		if err != nil {
			t.Skip("ffprobe not installed")
		}
		_, err = exec.LookPath("ffmpeg")
		if err != nil {
			t.Skip("ffmpeg not installed")
		}

		mediaPath := filepath.Join(t.TempDir(), "probe-sample.mp4")
		cmd := exec.CommandContext( //nolint:gosec // test fixture path is under temp dir
			context.Background(),
			"ffmpeg",
			"-hide_banner",
			"-loglevel", "error",
			"-f", "lavfi",
			"-i", "testsrc=size=640x360:rate=1",
			"-f", "lavfi",
			"-i", "sine=frequency=440:duration=1",
			"-t", "1",
			"-y",
			mediaPath,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg fixture: %v (%s)", err, output)
		}

		info, err := ProbeSource(context.Background(), mediaPath)
		if err != nil {
			t.Fatalf("probe source: %v", err)
		}
		if info.Height < 360 {
			t.Fatalf("expected at least 360p height, got %d", info.Height)
		}
		if len(info.AudioStreams) == 0 {
			t.Fatal("expected at least one audio stream")
		}
		if info.AudioStreams[0].Label == "" {
			t.Fatal("expected non-empty audio label")
		}
	})
}

func TestProbeSource_InvalidPathReturnsError(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ProbeSource rejects missing media files", func(a *allure.Context) {
		t := a.T()
		_, err := exec.LookPath("ffprobe")
		if err != nil {
			t.Skip("ffprobe not installed")
		}

		missing := filepath.Join(t.TempDir(), "missing.mp4")
		_, err = ProbeSource(context.Background(), missing)
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})
}

func TestSourceInfoFromProbeOutput_SkipsAttachedPictureStreams(t *testing.T) {
	t.Parallel()

	allure.Test(t, "attached_pic video streams are ignored for height", func(a *allure.Context) {
		t := a.T()
		info := sourceInfoFromProbeOutput(probeOutput{
			Streams: []probeStream{
				{
					CodecType:   codecTypeVideo,
					Height:      720,
					Disposition: map[string]int{"attached_pic": 1},
				},
				{
					CodecType: codecTypeVideo,
					Height:    480,
				},
			},
		})

		if info.Height != 480 {
			t.Fatalf("expected 480p from non-attached stream, got %d", info.Height)
		}
	})
}

func TestSourceInfoFromProbeOutput_PromotesDefaultAudioDisposition(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"disposition-flagged default audio track becomes AudioStreams[0]",
		func(a *allure.Context) {
			t := a.T()
			info := sourceInfoFromProbeOutput(probeOutput{
				Streams: []probeStream{
					{CodecType: codecTypeVideo, Height: 720},
					{
						CodecType: codecTypeAudio,
						Index:     1,
						CodecName: "dts",
						Tags:      map[string]string{testTagTitle: testAudioLabelCommentary},
					},
					{
						CodecType:   codecTypeAudio,
						Index:       2,
						CodecName:   testCodecAAC,
						Tags:        map[string]string{testTagTitle: testAudioLabelJapanese},
						Disposition: map[string]int{"default": 1},
					},
				},
			})

			if len(info.AudioStreams) != 2 || info.AudioStreams[0].Label != testAudioLabelJapanese {
				t.Fatalf("expected default-flagged track first, got %+v", info.AudioStreams)
			}
			if info.AudioCodec != testCodecAAC {
				t.Fatalf("expected AudioCodec to follow promoted default, got %q", info.AudioCodec)
			}
		},
	)

	allure.Test(
		t,
		"no disposition default falls back to first probed audio stream",
		func(a *allure.Context) {
			t := a.T()
			info := sourceInfoFromProbeOutput(probeOutput{
				Streams: []probeStream{
					{CodecType: codecTypeVideo, Height: 720},
					{
						CodecType: codecTypeAudio,
						Index:     1,
						CodecName: testCodecAAC,
						Tags:      map[string]string{testTagTitle: testAudioLabelEnglish},
					},
					{
						CodecType: codecTypeAudio,
						Index:     2,
						CodecName: testCodecAAC,
						Tags:      map[string]string{testTagTitle: testAudioLabelJapanese},
					},
				},
			})

			if len(info.AudioStreams) != 2 || info.AudioStreams[0].Label != testAudioLabelEnglish {
				t.Fatalf("expected fallback to first probed stream, got %+v", info.AudioStreams)
			}
		},
	)
}

func TestParseProbeJSON_RejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	allure.Test(t, "invalid ffprobe json is rejected", func(a *allure.Context) {
		t := a.T()
		var parsed probeOutput
		err := json.Unmarshal([]byte("{"), &parsed)
		if err == nil {
			t.Fatal("expected parse error")
		}
	})
}
