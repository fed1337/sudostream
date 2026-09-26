package transcode

import (
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestStereoDownmixFilter_JellyfinLayouts(t *testing.T) {
	t.Parallel()

	allure.Test(t, "pan filters match Jellyfin DownMixAlgorithmsHelper", func(a *allure.Context) {
		t := a.T()

		cases := []struct {
			algo     DownmixAlgorithm
			channels int
			layout   string
			want     string
		}{
			{algo: DownmixAC4, channels: 2, layout: layoutStereo, want: ""},
			{algo: DownmixAC4, channels: 6, layout: layout51, want: downmixPanFilters[downmixKey{DownmixAC4, layout51}]},
			{algo: DownmixDave750, channels: 6, layout: layout51, want: downmixPanFilters[downmixKey{DownmixDave750, layout51}]},
			{algo: DownmixRFC7845, channels: 4, layout: layoutQuad, want: downmixPanFilters[downmixKey{DownmixRFC7845, layoutQuad}]},
			{algo: DownmixNone, channels: 6, layout: layout51, want: ""},
		}

		for _, testCase := range cases {
			got := StereoDownmixFilter(testCase.algo, testCase.channels, testCase.layout)
			if got != testCase.want {
				t.Fatalf("algo=%s channels=%d layout=%q: got %q want %q",
					testCase.algo, testCase.channels, testCase.layout, got, testCase.want)
			}
		}
	})
}

func TestComposeAudioFilter_BoostAndPan(t *testing.T) {
	t.Parallel()

	allure.Test(t, "composeAudioFilter orders pan then volume", func(a *allure.Context) {
		t := a.T()

		filter := ComposeAudioFilter(DownmixAC4, 6, layout51, 1.5)
		if !strings.Contains(filter, "pan=stereo") {
			t.Fatalf("expected pan filter, got %q", filter)
		}
		if !strings.HasSuffix(filter, "volume=1.5") {
			t.Fatalf("expected volume suffix, got %q", filter)
		}

		noneOnly := ComposeAudioFilter(DownmixNone, 6, layout51, 2)
		if strings.Contains(noneOnly, "pan=") {
			t.Fatalf("none must not emit pan: %q", noneOnly)
		}
		if noneOnly != "volume=2" {
			t.Fatalf("none boost: got %q", noneOnly)
		}
	})
}
