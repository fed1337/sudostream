package transcode

import (
	"math"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSegmentTable_CutsOnKeyframes(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"segment boundaries land on keyframes at or after the target",
		func(a *allure.Context) {
			t := a.T()
			keyframes := []float64{0, 2, 4, 6.5, 8, 10, 13, 16, 18}

			segments := SegmentTable(keyframes, 20, 6)
			if len(segments) == 0 {
				t.Fatal("expected segments")
			}

			total := 0.0
			for _, length := range segments {
				if length <= 0 {
					t.Fatalf("non-positive segment in %v", segments)
				}

				total += length
			}

			if math.Abs(total-20) > 0.001 {
				t.Fatalf(
					"segments must cover the timeline exactly, got %v (total %v)",
					segments,
					total,
				)
			}

			if math.Abs(segments[0]-6.5) > 0.001 {
				t.Fatalf("first cut should be the keyframe at 6.5, got %v", segments)
			}
		},
	)
}

func TestSegmentTable_FallsBackToEqualLength(t *testing.T) {
	t.Parallel()

	allure.Test(t, "sources without keyframe data get uniform segments", func(a *allure.Context) {
		t := a.T()
		segments := SegmentTable(nil, 25, 6)

		want := []float64{6, 6, 6, 6, 1}
		if len(segments) != len(want) {
			t.Fatalf("got %v want %v", segments, want)
		}

		for index := range want {
			if math.Abs(segments[index]-want[index]) > 0.001 {
				t.Fatalf("got %v want %v", segments, want)
			}
		}
	})
}

func TestSegmentTable_RejectsEmptyTimeline(t *testing.T) {
	t.Parallel()

	allure.Test(t, "zero duration yields no timeline", func(a *allure.Context) {
		t := a.T()
		if segments := SegmentTable([]float64{0, 6}, 0, 6); segments != nil {
			t.Fatalf("expected nil, got %v", segments)
		}
		if segments := SegmentTable(nil, 10, 0); segments != nil {
			t.Fatalf("expected nil, got %v", segments)
		}
	})
}

func TestSegmentStart_AccumulatesTable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "segment start is the sum of preceding durations", func(a *allure.Context) {
		t := a.T()
		segments := []float64{6, 4, 8}

		if got := SegmentStart(segments, 0); got != 0 {
			t.Fatalf("start 0: got %v", got)
		}
		if got := SegmentStart(segments, 2); got != 10 {
			t.Fatalf("start 2: got %v", got)
		}
		if got := SegmentStart(segments, 99); got != 18 {
			t.Fatalf("clamped start: got %v", got)
		}
	})
}

func TestSegmentName_RoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "segment names encode a parseable zero-padded index", func(a *allure.Context) {
		t := a.T()
		if name := SegmentName(42); name != "seg_00042.ts" {
			t.Fatalf("name: got %q", name)
		}

		index, parsed := SegmentIndexFromName("seg_00042.ts")
		if !parsed || index != 42 {
			t.Fatalf("parse: got %d parsed=%v", index, parsed)
		}

		if _, parsed = SegmentIndexFromName("playlist.m3u8"); parsed {
			t.Fatal("expected playlists to be rejected")
		}
		if _, parsed = SegmentIndexFromName("seg_abc.ts"); parsed {
			t.Fatal("expected non-numeric index to be rejected")
		}
	})
}

func TestBuildMediaPlaylist_IsCompleteVOD(t *testing.T) {
	t.Parallel()

	allure.Test(t, "media playlist is fully published up front", func(a *allure.Context) {
		t := a.T()
		body := string(BuildMediaPlaylist([]float64{6, 6, 3.5}))

		for _, want := range []string{
			"#EXT-X-PLAYLIST-TYPE:VOD",
			"#EXT-X-TARGETDURATION:6",
			"#EXTINF:6.000000,",
			"#EXTINF:3.500000,",
			"seg_00002.ts",
			"#EXT-X-ENDLIST",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("missing %q in:\n%s", want, body)
			}
		}
	})
}

func TestLadderHeights_IncludesNativeAndNeverUpscales(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"ladder spans the native height down to the lowest rung",
		func(a *allure.Context) {
			t := a.T()
			heights := LadderHeights(576)
			if len(heights) == 0 || heights[0] != 576 {
				t.Fatalf("expected native height first, got %v", heights)
			}

			for index := 1; index < len(heights); index++ {
				if heights[index] >= heights[index-1] {
					t.Fatalf("expected descending ladder, got %v", heights)
				}
				if heights[index] > 576 {
					t.Fatalf("ladder must not upscale, got %v", heights)
				}
			}
		},
	)
}
