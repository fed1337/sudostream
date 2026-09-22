package transcode

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseProbeDuration(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parseProbeDuration accepts ffprobe duration strings", func(a *allure.Context) {
		t := a.T()
		cases := []struct {
			raw  string
			want float64
		}{
			{raw: "3723.456", want: 3723.456},
			{raw: " 60.0 ", want: 60},
			{raw: "", want: 0},
			{raw: "nope", want: 0},
			{raw: "-1", want: 0},
		}

		for _, tc := range cases {
			if got := parseProbeDuration(tc.raw); got != tc.want {
				t.Fatalf("parseProbeDuration(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		}
	})
}

func TestSourceDuration_RoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"WriteSourceDuration and ReadSourceDuration round-trip",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()

			err := WriteSourceDuration(root, 2712.5)
			if err != nil {
				t.Fatalf("write: %v", err)
			}

			got, ok := ReadSourceDuration(root)
			if !ok {
				t.Fatal("expected duration file to exist")
			}
			if got != 2712.5 {
				t.Fatalf("got %v want 2712.5", got)
			}
		},
	)
}

func TestBuildPlaybackInfo_FallsBackToSourceDuration(t *testing.T) {
	t.Parallel()

	allure.Test(t, "duration is reported before probe metadata lands", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()

		err := WriteSourceDuration(root, 1800)
		if err != nil {
			t.Fatalf("write duration: %v", err)
		}

		info := BuildPlaybackInfo(
			root,
			"/api/play/movies/foo.mp4/master.m3u8",
			JobStatus{Status: StatusProcessing},
		)

		if info.DurationSeconds != 1800 {
			t.Fatalf("duration: got %v want 1800", info.DurationSeconds)
		}
	})
}
