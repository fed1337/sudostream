package transcode

import (
	"context"
	"math"
	"os/exec"
	"slices"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParsePacketCSV_KeepsOnlyMonotonicKeyPackets(t *testing.T) {
	t.Parallel()

	allure.Test(t, "only increasing keyframe timestamps survive parsing", func(a *allure.Context) {
		t := a.T()
		raw := "0.000000,1000,K__\n" +
			"0.041667,200,___\n" +
			"2.000000,1000,K__,\n" + // MPEG-TS rows carry a trailing comma
			"1.500000,900,K__\n" + // out of order, e.g. B-frame reordering
			"4.000000,1000,K__\n" +
			"bad,1000,K__\n" +
			"5.000000,bad,___\n" +
			"\n"

		got := parsePacketCSV(raw)
		want := []float64{0, 2, 4}

		if !slices.Equal(got.Keyframes, want) {
			t.Fatalf("keyframes: got %v want %v", got.Keyframes, want)
		}

		if len(got.packets) != 5 {
			t.Fatalf("packets: got %d want 5 (%v)", len(got.packets), got.packets)
		}
	})
}

func TestPacketIndex_NormalizedToStartTime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		start float64
		want  []float64
	}{
		{name: "zero start is unchanged", start: 0, want: []float64{0, 2, 4}},
		{name: "negative start (MKV AAC priming) shifts later", start: -0.021, want: []float64{0.021, 2.021, 4.021}},
		{name: "positive start (MPEG-TS) shifts earlier and drops pre-zero", start: 2.5, want: []float64{1.5}},
	} {
		allure.Test(t, "packet index normalization — "+tc.name, func(a *allure.Context) {
			t := a.T()
			index := PacketIndex{
				Keyframes: []float64{0, 2, 4},
				packets:   []packetSample{{pts: 0, size: 10}, {pts: 4, size: 10}},
			}

			got := index.normalized(tc.start)
			if len(got.Keyframes) != len(tc.want) {
				t.Fatalf("keyframes: got %v want %v", got.Keyframes, tc.want)
			}

			for position := range tc.want {
				if math.Abs(got.Keyframes[position]-tc.want[position]) > 1e-9 {
					t.Fatalf("keyframes: got %v want %v", got.Keyframes, tc.want)
				}
			}

			if got.packets[1].pts != 4-tc.start {
				t.Fatalf("packet pts: got %v want %v", got.packets[1].pts, 4-tc.start)
			}
		})
	}
}

func TestPacketIndex_SegmentBitrates(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		packets     []packetSample
		segments    []float64
		wantPeak    int64
		wantAverage int64
	}{
		{
			name:     "peak is the busiest full segment",
			packets:  []packetSample{{pts: 0, size: 6000}, {pts: 6, size: 12000}, {pts: 12.5, size: 6000}},
			segments: []float64{6, 6, 6},
			// 6000B/6s = 8 kbit/s, 12000B/6s = 16 kbit/s; mean 24000B over 18s.
			wantPeak:    16000,
			wantAverage: 10666,
		},
		{
			name:        "a short tail segment cannot set the peak",
			packets:     []packetSample{{pts: 0, size: 6000}, {pts: 6.2, size: 6000}},
			segments:    []float64{6, 0.5},
			wantPeak:    14769, // falls back to the mean, above the full segment's 8 kbit/s
			wantAverage: 14769,
		},
		{name: "no packets yields zero", segments: []float64{6}},
	} {
		allure.Test(t, "segment bitrates — "+tc.name, func(a *allure.Context) {
			t := a.T()

			peak, average := PacketIndex{packets: tc.packets}.segmentBitrates(tc.segments)
			if peak != tc.wantPeak || average != tc.wantAverage {
				t.Fatalf("got peak=%d average=%d want peak=%d average=%d",
					peak, average, tc.wantPeak, tc.wantAverage)
			}
		})
	}
}

func TestKeyframes_RoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cached keyframes survive a write and read cycle", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()

		err := WriteKeyframes(root, []float64{0, 6.25, 12.5})
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		got, ok := ReadKeyframes(root)
		if !ok || len(got) != 3 || got[1] != 6.25 {
			t.Fatalf("read: got %v ok=%v", got, ok)
		}

		if _, ok = ReadKeyframes(t.TempDir()); ok {
			t.Fatal("expected miss on an empty cache dir")
		}
	})
}

func TestProbeKeyframes_RespectsCanceledContext(t *testing.T) {
	t.Parallel()

	allure.Test(t, "canceled context fails keyframe probe quickly", func(a *allure.Context) {
		t := a.T()
		_, err := exec.LookPath("ffprobe")
		if err != nil {
			t.Skip("ffprobe not installed")
		}

		mediaPath := synthesizeFixture(t, 4)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		_, err = ProbeVideoPackets(ctx, mediaPath)
		if err == nil {
			t.Fatal("expected error from canceled context")
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("probe took too long after cancel: %v (%s)", err, time.Since(start))
		}
	})
}
