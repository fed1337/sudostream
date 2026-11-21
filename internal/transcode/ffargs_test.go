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

func TestSegmentRun_EncodesToEOF(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"a run covers every remaining segment; pacing is pause throttle not -t",
		func(a *allure.Context) {
			t := a.T()
			run := segmentRunFixture(VideoVariant(720), 2)
			run.meta.Segments = []float64{6, 6, 6, 6, 6, 6}

			if end := run.endIndex(); end != 6 {
				t.Fatalf("end index: got %d want 6", end)
			}

			if cuts := run.cutTimes(); len(cuts) != 3 {
				t.Fatalf("cuts: got %d want 3 (%v)", len(cuts), cuts)
			}

			joined := strings.Join(buildSegmentArgs(run), " ")
			if strings.Contains(joined, "-t ") {
				t.Fatalf("EOF runs must not set -t, got: %s", joined)
			}
		},
	)
}

func TestBuildSegmentArgs_VariantArgv(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		variant    Variant
		startIndex int
		want       []string
		reject     []string
	}{
		{
			name:       "the remuxable native rung is stream-copied",
			variant:    VideoVariant(720),
			startIndex: 2,
			want: []string{
				"-c:v copy",
				"-ss 12.000000",
				"-segment_start_number 2",
				"-output_ts_offset 12.000000",
				"-segment_times 4.000000",
			},
			reject: []string{"-force_key_frames"},
		},
		{
			name:       "encoded rungs pin IDR frames to the shared cut points",
			variant:    VideoVariant(360),
			startIndex: 0,
			want: []string{
				"-c:v libx264",
				"-force_key_frames 6.000000,12.000000,16.000000",
				"-segment_times 6.000000,12.000000,16.000000",
			},
			reject: []string{"-ss "},
		},
		{
			name:       "audio renditions map one source track and drop video",
			variant:    AudioVariant(1),
			startIndex: 0,
			want:       []string{"-map 0:2", "-vn", "-c:a aac"},
		},
	} {
		allure.Test(t, "segment argv — "+tc.name, func(a *allure.Context) {
			t := a.T()
			joined := strings.Join(buildSegmentArgs(segmentRunFixture(tc.variant, tc.startIndex)), " ")

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
			name:       "long copy run seeks with trim and has no -t",
			variant:    VideoVariant(720),
			segments:   uniform(40),
			keyframes:  []float64{0, 6, 12, 18},
			startIndex: 2,
			want:       []string{"-ss 10.500000 -i /media/movie.mkv -ss 1.000000 -map"},
			reject:     []string{"-t "},
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
