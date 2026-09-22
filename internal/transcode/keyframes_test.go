package transcode

import (
	"context"
	"os/exec"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseKeyframeCSV_KeepsOnlyMonotonicKeyPackets(t *testing.T) {
	t.Parallel()

	allure.Test(t, "only increasing keyframe timestamps survive parsing", func(a *allure.Context) {
		t := a.T()
		raw := "0.000000,K_\n" +
			"0.041667,__\n" +
			"2.000000,K_\n" +
			"1.500000,K_\n" + // out of order, e.g. B-frame reordering
			"4.000000,K_\n" +
			"bad,K_\n" +
			"\n"

		got := parseKeyframeCSV(raw)
		want := []float64{0, 2, 4}

		if len(got) != len(want) {
			t.Fatalf("got %v want %v", got, want)
		}

		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("got %v want %v", got, want)
			}
		}
	})
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
		_, err = ProbeKeyframes(ctx, mediaPath)
		if err == nil {
			t.Fatal("expected error from canceled context")
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("probe took too long after cancel: %v (%s)", err, time.Since(start))
		}
	})
}
