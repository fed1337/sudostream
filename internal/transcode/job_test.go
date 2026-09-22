package transcode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

var errTestFfmpegExit = errors.New("ffmpeg exited")

func TestJobFail_SetsErrorMessage(t *testing.T) {
	t.Parallel()

	allure.Test(t, "fail stores trimmed detail for status reporting", func(a *allure.Context) {
		t := a.T()
		job := &Job{mediaPath: "/media/a.mp4"}
		job.fail(errTestFfmpegExit, "  stderr details  ")

		if !job.Failed {
			t.Fatal("expected failed job")
		}
		if job.ErrorMsg != "stderr details" {
			t.Fatalf("error msg: got %q", job.ErrorMsg)
		}

		blank := &Job{mediaPath: "/media/b.mp4"}
		blank.fail(errTestFfmpegExit, "   ")
		if blank.ErrorMsg != errTestFfmpegExit.Error() {
			t.Fatalf("empty detail should fall back to err: got %q", blank.ErrorMsg)
		}
	})
}

func TestHasLocalSubtitles_Sidecar(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HasLocalSubtitles is true when a matching sidecar exists", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		mediaPath := filepath.Join(root, "movie.mp4")
		err := os.WriteFile(mediaPath, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write media: %v", err)
		}
		err = os.WriteFile(filepath.Join(root, "movie.en.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), 0o600)
		if err != nil {
			t.Fatalf("write sidecar: %v", err)
		}

		hasLocal, err := HasLocalSubtitles(context.Background(), mediaPath)
		if err != nil || !hasLocal {
			t.Fatalf("want local=true, got %v %v", hasLocal, err)
		}
	})
}

func TestHasLocalSubtitles_NoSidecarNoProbeSubs(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HasLocalSubtitles returns probe error without sidecar", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		mediaPath := filepath.Join(root, "movie.mp4")
		err := os.WriteFile(mediaPath, []byte("not a real media file"), 0o600)
		if err != nil {
			t.Fatalf("write media: %v", err)
		}

		hasLocal, err := HasLocalSubtitles(context.Background(), mediaPath)
		if err == nil || hasLocal {
			t.Fatalf("want probe error and local=false, got %v %v", hasLocal, err)
		}
	})
}

func TestAttachSidecarSubtitles_NoSidecarsIsNoOp(t *testing.T) {
	t.Parallel()

	allure.Test(t, "missing sidecar files returns nil", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		mediaPath := filepath.Join(root, "movie.mp4")
		err := os.WriteFile(mediaPath, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write media: %v", err)
		}

		outDir := filepath.Join(root, "cache")
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}

		err = EnsurePackagedSubtitles(context.Background(), mediaPath, outDir)
		if err != nil {
			t.Fatalf("ensure subtitles: %v", err)
		}

		if hasPackagedSubtitleTracks(outDir) {
			t.Fatal("expected no subtitle tracks")
		}
	})
}

func TestEnsurePackagedSubtitles_ConvertsSidecarSRT(t *testing.T) {
	t.Parallel()

	allure.Test(t, "sidecar srt is converted into cache vtt", func(a *allure.Context) {
		t := a.T()
		_, err := exec.LookPath("ffmpeg")
		if err != nil {
			t.Skip("ffmpeg not installed")
		}

		root := t.TempDir()
		mediaPath := filepath.Join(root, "movie.mp4")
		err = os.WriteFile(mediaPath, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write media: %v", err)
		}

		srt := "1\n00:00:01,000 --> 00:00:02,000\nHello\n"
		err = os.WriteFile(filepath.Join(root, "movie.srt"), []byte(srt), 0o600)
		if err != nil {
			t.Fatalf("write srt: %v", err)
		}

		outDir := filepath.Join(root, "cache")
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}

		err = EnsurePackagedSubtitles(context.Background(), mediaPath, outDir)
		if err != nil {
			t.Fatalf("ensure subtitles: %v", err)
		}

		tracks := listPackagedSubtitleTracks(outDir)
		if len(tracks) != 1 {
			t.Fatalf("expected one subtitle track, got %v", tracks)
		}

		playlistPath := filepath.Join(outDir, "subtitles", "srt", "playlist.m3u8")
		_, statErr := os.Stat(playlistPath)
		if statErr != nil {
			t.Fatalf("expected subtitle playlist: %v", statErr)
		}
	})
}

func TestAppendHWGlobalArgs_EncoderBranches(t *testing.T) {
	t.Parallel()

	allure.Test(t, "appendHWGlobalArgs covers hardware encoder argv", func(a *allure.Context) {
		t := a.T()
		nvenc := strings.Join(appendHWGlobalArgs(nil, EncoderNVENC, ""), " ")
		if !strings.Contains(nvenc, "-hwaccel cuda") {
			t.Fatalf("expected cuda hwaccel, got: %s", nvenc)
		}

		qsv := strings.Join(appendHWGlobalArgs(nil, EncoderQSV, ""), " ")
		if !strings.Contains(qsv, "-hwaccel qsv") {
			t.Fatalf("expected qsv hwaccel, got: %s", qsv)
		}

		vaapi := strings.Join(appendHWGlobalArgs(nil, EncoderVAAPI, "/dev/dri/renderD128"), " ")
		if !strings.Contains(vaapi, "-init_hw_device vaapi=va:/dev/dri/renderD128") {
			t.Fatalf("expected vaapi device init, got: %s", vaapi)
		}

		noRender := appendHWGlobalArgs(nil, EncoderVAAPI, "")
		if len(noRender) != 0 {
			t.Fatalf("expected no vaapi args without render node, got: %v", noRender)
		}

		if len(appendHWGlobalArgs(nil, EncoderSoftware, "")) != 0 {
			t.Fatal("expected no software encoder global args")
		}

		if len(appendHWGlobalArgs(nil, VideoEncoder("unknown"), "")) != 0 {
			t.Fatal("expected unknown encoder to add no global args")
		}
	})
}

func TestAppendVideoEncodeArgs_EncoderBranches(t *testing.T) {
	t.Parallel()

	allure.Test(t, "appendVideoEncodeArgs covers encoder-specific flags", func(a *allure.Context) {
		t := a.T()
		rung := FastStartRung(720)

		nvenc := strings.Join(appendVideoEncodeArgs(nil, EncoderNVENC, rung, ToneMapNone, ToneMappingBT2390), " ")
		if !strings.Contains(nvenc, "scale_cuda") ||
			!strings.Contains(nvenc, string(EncoderNVENC)) {
			t.Fatalf("expected nvenc scale/codec, got: %s", nvenc)
		}

		qsv := strings.Join(appendVideoEncodeArgs(nil, EncoderQSV, rung, ToneMapNone, ToneMappingBT2390), " ")
		if !strings.Contains(qsv, "scale_qsv") || !strings.Contains(qsv, string(EncoderQSV)) {
			t.Fatalf("expected qsv scale/codec, got: %s", qsv)
		}

		vaapi := strings.Join(appendVideoEncodeArgs(nil, EncoderVAAPI, rung, ToneMapNone, ToneMappingBT2390), " ")
		if !strings.Contains(vaapi, "scale_vaapi") ||
			!strings.Contains(vaapi, string(EncoderVAAPI)) {
			t.Fatalf("expected vaapi scale/codec, got: %s", vaapi)
		}

		software := strings.Join(
			appendVideoEncodeArgs(nil, EncoderSoftware, rung, ToneMapNone, ToneMappingBT2390),
			" ",
		)
		if !strings.Contains(software, "-crf 23") {
			t.Fatalf("expected software crf, got: %s", software)
		}

		unknown := strings.Join(
			appendVideoEncodeArgs(nil, VideoEncoder("unknown"), rung, ToneMapNone, ToneMappingBT2390),
			" ",
		)
		if !strings.Contains(unknown, string(EncoderSoftware)) {
			t.Fatalf("expected software fallback, got: %s", unknown)
		}
	})
}

func TestSubtitleVTTFilename(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"subtitleVTTFilename sanitizes labels and falls back to index",
		func(a *allure.Context) {
			t := a.T()
			cases := []struct {
				label string
				index int
				want  string
			}{
				{label: "", index: 3, want: "3"},
				{label: "   ", index: 2, want: "2"},
				{label: "English", index: 0, want: "English"},
				{label: `a/b\c:d*e?f"g<h>i|j`, index: 1, want: "a_b_c_d_e_f_g_h_i_j"},
			}

			for _, testCase := range cases {
				got := subtitleVTTFilename(testCase.label, testCase.index)
				if got != testCase.want {
					t.Fatalf(
						"subtitleVTTFilename(%q, %d)=%q want %q",
						testCase.label,
						testCase.index,
						got,
						testCase.want,
					)
				}
			}
		},
	)
}

func TestConvertSubtitleToWebVTT_SRTAndFailures(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"convertSubtitleToWebVTT covers go srt path and empty input failure",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			srtPath := filepath.Join(root, "movie.srt")
			outPath := filepath.Join(root, "movie.vtt")

			err := os.WriteFile(srtPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n"), 0o600)
			if err != nil {
				t.Fatalf("write srt: %v", err)
			}

			err = convertSubtitleToWebVTT(context.Background(), srtPath, outPath)
			if err != nil {
				t.Fatalf("convert srt: %v", err)
			}
			if !subtitleVTTHasCues(outPath) {
				t.Fatal("expected cues in converted vtt")
			}

			emptySRT := filepath.Join(root, "empty.srt")
			emptyOut := filepath.Join(root, "empty.vtt")
			err = os.WriteFile(emptySRT, []byte(""), 0o600)
			if err != nil {
				t.Fatalf("write empty srt: %v", err)
			}

			err = convertSubtitleToWebVTT(context.Background(), emptySRT, emptyOut)
			if !errors.Is(err, errSubtitleNoCues) {
				t.Fatalf("expected errSubtitleNoCues, got %v", err)
			}
		},
	)
}

func TestAttachEmbeddedSubtitles_NoStreamsOrProbeFailure(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"attachEmbeddedSubtitles is a no-op without subtitle streams",
		func(a *allure.Context) {
			t := a.T()
			_, lookErr := exec.LookPath("ffprobe")
			if lookErr != nil {
				t.Skip("ffprobe not installed")
			}

			mediaPath := synthesizeFixture(t, 4)
			outDir := filepath.Join(t.TempDir(), "cache")
			err := os.MkdirAll(outDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir cache: %v", err)
			}

			err = attachEmbeddedSubtitles(context.Background(), mediaPath, outDir, 4)
			if err != nil {
				t.Fatalf("attach embedded: %v", err)
			}

			if hasPackagedSubtitleTracks(outDir) {
				t.Fatal("fixture has no embedded subs; expected no packaged tracks")
			}

			// Probe failure should warn and return nil (non-media file).
			bogus := filepath.Join(t.TempDir(), "bogus.mp4")
			err = os.WriteFile(bogus, []byte("not a media file"), 0o600)
			if err != nil {
				t.Fatalf("write bogus: %v", err)
			}

			err = attachEmbeddedSubtitles(context.Background(), bogus, outDir, 1)
			if err != nil {
				t.Fatalf("probe failure should be soft: %v", err)
			}
		},
	)
}

func TestConvertEmbeddedSubtitleToWebVTT_FailsWithoutStream(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"convertEmbeddedSubtitleToWebVTT errors when stream is missing",
		func(a *allure.Context) {
			t := a.T()
			_, lookErr := exec.LookPath("ffmpeg")
			if lookErr != nil {
				t.Skip("ffmpeg not installed")
			}

			mediaPath := synthesizeFixture(t, 3)
			outPath := filepath.Join(t.TempDir(), "missing.vtt")

			err := convertEmbeddedSubtitleToWebVTT(context.Background(), mediaPath, 99, outPath)
			if err == nil {
				t.Fatal("expected failure for missing embedded stream")
			}
		},
	)
}

func TestProbeAndPublish_WritesTimelineWithoutEncoding(t *testing.T) {
	t.Parallel()

	allure.Test(t, "probe publishes every playlist and no media segments", func(a *allure.Context) {
		t := a.T()
		mediaPath := synthesizeFixture(t, 12)
		outDir := filepath.Join(t.TempDir(), "cache")

		err := os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}

		meta, err := ProbeAndPublish(context.Background(), outDir, mediaPath)
		if err != nil {
			t.Fatalf("probe and publish: %v", err)
		}

		if meta.SegmentCount() < 2 {
			t.Fatalf("expected a multi-segment timeline, got %d", meta.SegmentCount())
		}

		master, err := os.ReadFile(filepath.Join(outDir, masterPlaylistName)) //nolint:gosec // temp
		if err != nil {
			t.Fatalf("read master: %v", err)
		}
		if !strings.Contains(string(master), "#EXT-X-STREAM-INF") {
			t.Fatalf("expected variants in master, got:\n%s", master)
		}

		nativePlaylist := filepath.Join(outDir, VariantDir(meta.Height), mediaPlaylist)
		body, err := os.ReadFile(nativePlaylist) //nolint:gosec // temp dir
		if err != nil {
			t.Fatalf("read native playlist: %v", err)
		}
		if !strings.Contains(string(body), "#EXT-X-ENDLIST") {
			t.Fatalf("expected complete VOD playlist, got:\n%s", body)
		}

		_, statErr := os.Stat(filepath.Join(outDir, VariantDir(meta.Height), SegmentName(0)))
		if statErr == nil {
			t.Fatal("expected no segments to be produced during publish")
		}
	})
}

func TestJobRun_PublishesValidSource(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Job.Run publishes a valid source timeline", func(a *allure.Context) {
		t := a.T()
		mediaPath := synthesizeFixture(t, 8)
		root := t.TempDir()

		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

		outDir := filepath.Join(root, "ok")
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		service.wg.Add(1)
		job := &Job{cacheKey: "ok", mediaPath: mediaPath, outDir: outDir}
		job.Run(service)
		if job.Failed {
			t.Fatalf("expected success, got %q", job.ErrorMsg)
		}
		if !isTranscodeComplete(outDir) {
			t.Fatal("expected complete marker after Job.Run")
		}
		if _, ok := ReadSourceMeta(outDir); !ok {
			t.Fatal("expected source meta after Job.Run")
		}
	})
}

func TestJobRun_FailsOnBadMedia(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"Job.Run fails a non-media input without ffmpeg fixtures",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()

			service, err := NewService(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

			badDir := filepath.Join(root, "bad")
			err = os.MkdirAll(badDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir bad: %v", err)
			}
			badMedia := filepath.Join(root, "bad.mp4")
			err = os.WriteFile(badMedia, []byte("not media"), 0o600)
			if err != nil {
				t.Fatalf("write bad media: %v", err)
			}

			service.wg.Add(1)
			failJob := &Job{cacheKey: "bad", mediaPath: badMedia, outDir: badDir}
			failJob.Run(service)
			if !failJob.Failed {
				t.Fatal("expected Job.Run to fail for non-media input")
			}
		},
	)
}

func TestEnsurePackagedSubtitles_SkipsWhenTracksExist(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"EnsurePackagedSubtitles is a no-op when cues already exist",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			mediaPath := filepath.Join(root, "movie.mp4")
			err := os.WriteFile(mediaPath, []byte("fake"), 0o600)
			if err != nil {
				t.Fatalf("write media: %v", err)
			}

			outDir := filepath.Join(root, "cache")
			trackDir := filepath.Join(outDir, "subtitles", "English")
			err = os.MkdirAll(trackDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir track: %v", err)
			}

			vtt := "WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nHi\n"
			err = os.WriteFile(filepath.Join(trackDir, subtitleSegmentName), []byte(vtt), 0o600)
			if err != nil {
				t.Fatalf("write vtt: %v", err)
			}
			err = writeSubtitleMediaPlaylist(
				filepath.Join(trackDir, "playlist.m3u8"),
				subtitleSegmentName,
				12,
			)
			if err != nil {
				t.Fatalf("write playlist: %v", err)
			}

			err = EnsurePackagedSubtitles(context.Background(), mediaPath, outDir)
			if err != nil {
				t.Fatalf("ensure: %v", err)
			}
			if !hasPackagedSubtitleTracks(outDir) {
				t.Fatal("expected existing track to remain")
			}
		},
	)
}

func TestAttachSidecarSubtitles_SkipsBrokenSidecar(t *testing.T) {
	t.Parallel()

	allure.Test(t, "broken sidecar conversion leaves no packaged track", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		mediaPath := filepath.Join(root, "movie.mp4")
		err := os.WriteFile(mediaPath, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write media: %v", err)
		}

		// Empty ASS/SSA forces the ffmpeg loop; Go SRT path is skipped for non-.srt.
		err = os.WriteFile(filepath.Join(root, "movie.ass"), []byte(""), 0o600)
		if err != nil {
			t.Fatalf("write ass: %v", err)
		}

		outDir := filepath.Join(root, "cache")
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}

		err = attachSidecarSubtitles(context.Background(), mediaPath, outDir, 12)
		if err != nil {
			t.Fatalf("attach sidecar: %v", err)
		}
		if hasPackagedSubtitleTracks(outDir) {
			t.Fatal("broken sidecar must not leave a packaged track")
		}
	})
}

func TestAttachEmbeddedSubtitles_WithMovTextTrack(t *testing.T) {
	t.Parallel()

	allure.Test(t, "embedded mov_text is packaged into subtitle tracks", func(a *allure.Context) {
		t := a.T()
		mediaPath := synthesizeFixtureWithSubtitles(t)
		outDir := filepath.Join(t.TempDir(), "cache")
		err := os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		err = attachEmbeddedSubtitles(context.Background(), mediaPath, outDir, 4)
		if err != nil {
			t.Fatalf("attach embedded: %v", err)
		}

		if !hasPackagedSubtitleTracks(outDir) {
			t.Fatal("expected packaged embedded subtitle track")
		}
	})
}

func TestSourceMeta_HelpersAndInvalidCache(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"AudioTracks default and invalid meta files are ignored",
		func(a *allure.Context) {
			t := a.T()
			meta := SourceMeta{}
			tracks := meta.AudioTracks()
			if len(tracks) != 1 || tracks[0].Label != defaultAudioTrackLabel {
				t.Fatalf("default audio tracks: %+v", tracks)
			}

			root := t.TempDir()
			err := os.WriteFile(filepath.Join(root, sourceMetaMarker), []byte(`{`), 0o600)
			if err != nil {
				t.Fatalf("write bad meta: %v", err)
			}
			if _, ok := ReadSourceMeta(root); ok {
				t.Fatal("expected invalid meta to be ignored")
			}

			err = os.WriteFile(
				filepath.Join(root, sourceMetaMarker),
				[]byte(`{"height":480,"segments":[]}`),
				0o600,
			)
			if err != nil {
				t.Fatalf("write empty segments: %v", err)
			}
			if _, ok := ReadSourceMeta(root); ok {
				t.Fatal("expected empty segments to be ignored")
			}

			missingDir := filepath.Join(root, "missing", "nested")
			err = WriteSourceMeta(missingDir, SourceMeta{Height: 480, Segments: []float64{6}})
			if err == nil {
				t.Fatal("expected write into missing dir to fail")
			}
		},
	)
}

// synthesizeFixture renders a short H.264 clip with a 2-second GOP for segment tests.
func synthesizeFixture(t *testing.T, seconds int) string {
	t.Helper()

	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	mediaPath := filepath.Join(t.TempDir(), "fixture.mp4")

	cmd := exec.CommandContext( //nolint:gosec // fixture path is under temp dir
		context.Background(),
		"ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", strconv.Itoa(seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "20", "-keyint_min", "20",
		"-c:a", "aac", "-shortest", "-y",
		mediaPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("ffmpeg fixture unavailable: %v (%s)", err, output)
	}

	return mediaPath
}

func synthesizeFixtureWithSubtitles(t *testing.T) string {
	t.Helper()

	_, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	root := t.TempDir()
	srtPath := filepath.Join(root, "subs.srt")
	err = os.WriteFile(srtPath, []byte("1\n00:00:00,000 --> 00:00:01,000\nHello\n"), 0o600)
	if err != nil {
		t.Fatalf("write srt: %v", err)
	}

	mediaPath := filepath.Join(root, "with-subs.mp4")
	cmd := exec.CommandContext( //nolint:gosec // fixture path is under temp dir
		context.Background(),
		"ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-i", srtPath,
		"-t", "3",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac",
		"-c:s", "mov_text",
		"-shortest", "-y",
		mediaPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("ffmpeg subtitle fixture unavailable: %v (%s)", err, output)
	}

	return mediaPath
}
