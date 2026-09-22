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
