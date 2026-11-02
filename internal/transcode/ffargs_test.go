package transcode

import (
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func segmentRunFixture(variant Variant, startIndex int) segmentRun {
	return segmentRun{
		mediaPath:  "/media/movie.mkv",
		variantDir: "/cache/key/" + variant.Dir(),
		meta: SourceMeta{
			Height:        720,
			PackagingMode: PackagingRemux,
			AudioStreams: []AudioStream{
				{Index: 1, Label: testAudioLabelEnglish},
				{Index: 2, Label: "Jp"},
			},
			Segments: []float64{6, 6, 4, 6},
		},
		variant:    variant,
		startIndex: startIndex,
		encoder:    EncoderSoftware,
	}
}

func TestSegmentRun_CutTimesAreRelativeToTheSeek(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cut times are relative to the run start after a seek", func(a *allure.Context) {
		t := a.T()
		run := segmentRunFixture(VideoVariant(720), 1)

		if start := run.startSeconds(); start != 6 {
			t.Fatalf("start: got %v want 6", start)
		}

		cuts := run.cutTimes()
		want := []float64{6, 10}

		if len(cuts) != len(want) {
			t.Fatalf("cuts: got %v want %v", cuts, want)
		}

		for index := range want {
			if cuts[index] != want[index] {
				t.Fatalf("cuts: got %v want %v", cuts, want)
			}
		}
	})
}

func TestSegmentRun_ThrottlesRunLength(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"a run stops after the throttle bound instead of encoding to EOF",
		func(a *allure.Context) {
			t := a.T()
			run := segmentRunFixture(VideoVariant(720), 0)
			run.meta.Segments = make([]float64, maxRunSegments*2)

			for index := range run.meta.Segments {
				run.meta.Segments[index] = SegmentTargetSeconds
			}

			if end := run.endIndex(); end != maxRunSegments {
				t.Fatalf("end index: got %d want %d", end, maxRunSegments)
			}

			want := SegmentTargetSeconds * maxRunSegments
			if length := run.runSeconds(); length != want {
				t.Fatalf("run seconds: got %v want %v", length, want)
			}

			if cuts := run.cutTimes(); len(cuts) != maxRunSegments-1 {
				t.Fatalf("cuts: got %d want %d", len(cuts), maxRunSegments-1)
			}

			tail := segmentRunFixture(VideoVariant(720), 0)
			if length := tail.runSeconds(); length != 0 {
				t.Fatalf("a run reaching EOF must not be time-limited, got %v", length)
			}
		},
	)
}

func TestBuildSegmentArgs_NativeRungCopiesVideo(t *testing.T) {
	t.Parallel()

	allure.Test(t, "the remuxable native rung is stream-copied", func(a *allure.Context) {
		t := a.T()
		joined := strings.Join(buildSegmentArgs(segmentRunFixture(VideoVariant(720), 2)), " ")

		if !strings.Contains(joined, "-c:v copy") {
			t.Fatalf("expected stream copy, got: %s", joined)
		}
		if strings.Contains(joined, "-force_key_frames") {
			t.Fatalf("copy must not force keyframes, got: %s", joined)
		}
		if !strings.Contains(joined, "-ss 12.000000") {
			t.Fatalf("expected seek to segment 2, got: %s", joined)
		}
		if !strings.Contains(joined, "-segment_start_number 2") {
			t.Fatalf("expected segment numbering to resume at 2, got: %s", joined)
		}
		if !strings.Contains(joined, "-output_ts_offset 12.000000") {
			t.Fatalf("expected timeline offset, got: %s", joined)
		}
		if !strings.Contains(joined, "-segment_times 4.000000") {
			t.Fatalf("expected explicit cut points, got: %s", joined)
		}
	})
}

func TestBuildSegmentArgs_CopySeekPrerollAndTrim(t *testing.T) {
	t.Parallel()

	uniform := func(count int) []float64 {
		segments := make([]float64, count)
		for index := range segments {
			segments[index] = SegmentTargetSeconds
		}

		return segments
	}

	for _, tc := range []struct {
		name       string
		variant    Variant
		segments   []float64
		keyframes  []float64
		startIndex int
		want       []string
		reject     []string
	}{
		{
			name:       "keyframe start seeks early and trims just before the keyframe",
			variant:    VideoVariant(720),
			segments:   []float64{6, 6, 4, 6},
			keyframes:  []float64{0, 2, 6, 12, 16},
			startIndex: 2,
			want: []string{
				"-ss 10.500000 -i /media/movie.mkv -ss 1.000000 -map",
				"-output_ts_offset 11.500000",
				"-segment_times 4.000000",
			},
			reject: []string{"-t "},
		},
		{
			name:       "throttled run extends -t by the trim",
			variant:    VideoVariant(720),
			segments:   uniform(maxRunSegments * 2),
			keyframes:  []float64{0, 6, 12, 18},
			startIndex: 2,
			want:       []string{"-ss 1.000000 -t 72.500000 -map"},
		},
		{
			name:       "start inside the preroll skips the input seek",
			variant:    VideoVariant(720),
			segments:   []float64{1, 6, 6},
			keyframes:  []float64{0, 1, 7},
			startIndex: 1,
			want:       []string{"-nostdin -i /media/movie.mkv -ss 0.500000 -map", "-output_ts_offset 0.500000"},
		},
		{
			name:       "a close previous keyframe shrinks the trim to the midpoint",
			variant:    VideoVariant(720),
			segments:   []float64{6, 6, 4, 6},
			keyframes:  []float64{0, 6, 11.6, 12},
			startIndex: 2,
			want:       []string{"-ss 10.500000 -i /media/movie.mkv -ss 1.300000 -map", "-output_ts_offset 11.800000"},
		},
		{
			name:       "a start that is not a probed keyframe keeps the plain seek",
			variant:    VideoVariant(720),
			segments:   []float64{6, 6, 4, 6},
			keyframes:  []float64{0, 5, 13},
			startIndex: 2,
			want:       []string{"-ss 12.000000 -i /media/movie.mkv -map", "-output_ts_offset 12.000000"},
		},
		{
			name:       "transcoded rungs keep the frame-accurate input seek",
			variant:    VideoVariant(360),
			segments:   []float64{6, 6, 4, 6},
			keyframes:  []float64{0, 6, 12, 16},
			startIndex: 2,
			want:       []string{"-ss 12.000000 -i /media/movie.mkv -map", "-output_ts_offset 12.000000"},
		},
	} {
		allure.Test(t, "copy seek argv — "+tc.name, func(a *allure.Context) {
			t := a.T()
			run := segmentRunFixture(tc.variant, tc.startIndex)
			run.meta.Segments = tc.segments
			run.keyframes = tc.keyframes
			joined := strings.Join(buildSegmentArgs(run), " ")

			for _, fragment := range tc.want {
				if !strings.Contains(joined, fragment) {
					t.Fatalf("missing %q in: %s", fragment, joined)
				}
			}

			for _, fragment := range tc.reject {
				if strings.Contains(joined, fragment) {
					t.Fatalf("unexpected %q in: %s", fragment, joined)
				}
			}
		})
	}
}

func TestBuildSegmentArgs_LowerRungForcesKeyframesAtCuts(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"encoded rungs pin IDR frames to the shared cut points",
		func(a *allure.Context) {
			t := a.T()
			joined := strings.Join(buildSegmentArgs(segmentRunFixture(VideoVariant(360), 0)), " ")

			if !strings.Contains(joined, "-c:v libx264") {
				t.Fatalf("expected encode for a downscaled rung, got: %s", joined)
			}
			if !strings.Contains(joined, "-force_key_frames 6.000000,12.000000,16.000000") {
				t.Fatalf("expected forced keyframes at cut points, got: %s", joined)
			}
			if !strings.Contains(joined, "-segment_times 6.000000,12.000000,16.000000") {
				t.Fatalf("expected matching segment times, got: %s", joined)
			}
			if strings.Contains(joined, "-ss ") {
				t.Fatalf("a run from segment 0 must not seek, got: %s", joined)
			}
		},
	)
}

func TestBuildSegmentArgs_AudioRenditionIsVideolessAAC(t *testing.T) {
	t.Parallel()

	allure.Test(t, "audio renditions map one source track and drop video", func(a *allure.Context) {
		t := a.T()
		joined := strings.Join(buildSegmentArgs(segmentRunFixture(AudioVariant(1), 0)), " ")

		if !strings.Contains(joined, "-map 0:2") {
			t.Fatalf("expected second audio stream by global index, got: %s", joined)
		}
		if !strings.Contains(joined, "-vn") || !strings.Contains(joined, "-c:a aac") {
			t.Fatalf("expected video-free AAC rendition, got: %s", joined)
		}
	})
}
