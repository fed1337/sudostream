package transcode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSourceMetaFromProbe_PackagingAndTracks(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"sourceMetaFromProbe sets remux/transcode and segment table",
		func(a *allure.Context) {
			t := a.T()
			remux := sourceMetaFromProbe(SourceInfo{
				Height:           1080,
				Width:            1920,
				DurationSeconds:  12,
				FrameRate:        24,
				VideoCodec:       codecH264,
				VideoStreamCount: 1,
				AudioStreams:     []AudioStream{{Index: 0, Label: testAudioLabelEnglish}},
				SubtitleStreams:  []SubtitleStream{{Index: 2, Label: "Subs"}},
			}, []float64{0, 6})

			if remux.PackagingMode != PackagingRemux {
				t.Fatalf("packaging: got %q", remux.PackagingMode)
			}
			if remux.SegmentCount() == 0 {
				t.Fatal("expected segments")
			}
			if len(remux.AudioTracks()) != 1 ||
				remux.AudioTracks()[0].Label != testAudioLabelEnglish {
				t.Fatalf("audio tracks: %+v", remux.AudioTracks())
			}
			if len(remux.Qualities()) == 0 {
				t.Fatal("expected qualities")
			}

			transcodeMeta := sourceMetaFromProbe(SourceInfo{
				Height:           720,
				DurationSeconds:  6,
				VideoCodec:       "hevc",
				VideoStreamCount: 1,
			}, nil)
			if transcodeMeta.PackagingMode != PackagingTranscode {
				t.Fatalf("expected transcode packaging, got %q", transcodeMeta.PackagingMode)
			}
		},
	)
}

func TestWriteReadSourceMeta_RoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "WriteSourceMeta and ReadSourceMeta round-trip", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		want := SourceMeta{
			Height:          720,
			Width:           1280,
			DurationSeconds: 18,
			PackagingMode:   PackagingTranscode,
			AudioStreams:    []AudioStream{{Index: 1, Label: testAudioLabelCommentary}},
			Segments:        []float64{6, 6, 6},
		}

		err := WriteSourceMeta(root, want)
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		got, ok := ReadSourceMeta(root)
		if !ok {
			t.Fatal("expected readable meta")
		}
		if got.Height != want.Height || got.SegmentCount() != 3 ||
			got.AudioTracks()[0].Label != testAudioLabelCommentary {
			t.Fatalf("round-trip mismatch: %+v", got)
		}
	})
}

func TestPublishPlaylists_WritesMasterVariantsAndAudio(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"PublishPlaylists writes master, ladder, and audio playlists",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			meta := SourceMeta{
				Height: 720,
				AudioStreams: []AudioStream{
					{Index: 0, Label: "Default"},
					{Index: 1, Label: testAudioLabelCommentary},
				},
				Segments: []float64{6, 6},
			}

			err := PublishPlaylists(root, meta)
			if err != nil {
				t.Fatalf("publish: %v", err)
			}

			//nolint:gosec // path is under t.TempDir()
			master, err := os.ReadFile(filepath.Join(root, masterPlaylistName))
			if err != nil {
				t.Fatalf("read master: %v", err)
			}
			if !strings.Contains(string(master), "#EXT-X-STREAM-INF") {
				t.Fatalf("expected stream inf in master:\n%s", master)
			}

			for _, height := range LadderHeights(meta.Height) {
				path := filepath.Join(root, VariantDir(height), mediaPlaylist)
				_, statErr := os.Stat(path)
				if statErr != nil {
					t.Fatalf("missing variant playlist %s: %v", path, statErr)
				}
			}

			for index := range meta.AudioStreams {
				path := filepath.Join(root, AudioDir(index), mediaPlaylist)
				_, statErr := os.Stat(path)
				if statErr != nil {
					t.Fatalf("missing audio playlist %s: %v", path, statErr)
				}
			}
		},
	)
}

func TestProbeAndPublish_FailsWithoutMedia(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ProbeAndPublish returns error for missing media", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		_, err := ProbeAndPublish(context.Background(), root, filepath.Join(root, "missing.mp4"))
		if err == nil {
			t.Fatal("expected probe failure")
		}
	})
}
