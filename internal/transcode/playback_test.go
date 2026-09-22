package transcode_test

import (
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestBuildPlaybackInfo_ReadyExposesFullLadderAndTracks(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ready playback lists every rung and audio track", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()

		meta := transcode.SourceMeta{
			Height:          720,
			DurationSeconds: 120,
			PackagingMode:   transcode.PackagingRemux,
			AudioStreams: []transcode.AudioStream{
				{Index: 1, Label: "English"},
				{Index: 2, Label: "Japanese"},
			},
			Segments: []float64{6, 6},
		}

		err := transcode.WriteSourceMeta(root, meta)
		if err != nil {
			t.Fatalf("write meta: %v", err)
		}

		info := transcode.BuildPlaybackInfo(
			root,
			"/api/play/movies/foo.mp4/master.m3u8",
			transcode.JobStatus{Status: transcode.StatusReady},
		)

		if info.MasterURL == "" {
			t.Fatal("expected master URL when ready")
		}
		if info.DurationSeconds != 120 {
			t.Fatalf("duration: got %v", info.DurationSeconds)
		}
		if len(info.Qualities) < 2 || info.Qualities[0].Height != 720 {
			t.Fatalf("qualities: %+v", info.Qualities)
		}
		if len(info.AudioTracks) != 2 || info.AudioTracks[1].Label != "Japanese" {
			t.Fatalf("audio tracks: %+v", info.AudioTracks)
		}
	})
}

func TestBuildPlaybackInfo_ProcessingOmitsMasterURL(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"master URL is withheld until the timeline is published",
		func(a *allure.Context) {
			t := a.T()
			info := transcode.BuildPlaybackInfo(
				t.TempDir(),
				"/api/play/movies/foo.mp4/master.m3u8",
				transcode.JobStatus{Status: transcode.StatusProcessing},
			)

			if info.MasterURL != "" {
				t.Fatalf("expected no master URL while processing, got %q", info.MasterURL)
			}
		},
	)
}

func TestBuildPlaybackInfo_ToneMappedRespectsAdminToggle(t *testing.T) {
	t.Parallel()

	allure.Test(t, "toneMapped is false when admin tonemap is disabled", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		meta := transcode.SourceMeta{
			Height:        1080,
			ColorTransfer: "smpte2084",
			PackagingMode: transcode.PackagingTranscode,
			Segments:      []float64{6},
			AudioStreams:  []transcode.AudioStream{{Index: 1, Label: "Default"}},
		}
		err := transcode.WriteSourceMeta(root, meta)
		if err != nil {
			t.Fatalf("write meta: %v", err)
		}

		enabled := transcode.DefaultTranscodeSettings()
		info := transcode.BuildPlaybackInfoWithSettings(
			root, "", transcode.JobStatus{Status: transcode.StatusReady}, enabled,
		)
		if !info.ToneMapped {
			t.Fatal("expected toneMapped when enabled")
		}

		disabled := enabled
		disabled.ToneMappingEnabled = false
		info = transcode.BuildPlaybackInfoWithSettings(
			root, "", transcode.JobStatus{Status: transcode.StatusReady}, disabled,
		)
		if info.ToneMapped {
			t.Fatal("expected toneMapped false when admin disabled")
		}
	})
}

func TestContentTypeForResource_MapsHLSExtensions(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HLS resource paths map to expected MIME types", func(a *allure.Context) {
		t := a.T()
		cases := map[string]string{
			"master.m3u8":  "application/vnd.apple.mpegurl",
			"seg_00000.ts": "video/mp2t",
			"chunk.m4s":    "video/iso.segment",
			"subs/eng.vtt": "text/vtt; charset=utf-8",
			"unknown.bin":  "application/octet-stream",
		}
		for resource, want := range cases {
			if got := transcode.ContentTypeForResource(resource); got != want {
				t.Fatalf("%q: got %q want %q", resource, got, want)
			}
		}
	})
}
