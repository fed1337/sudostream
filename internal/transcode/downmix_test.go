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

	for _, tc := range []struct {
		name     string
		algo     DownmixAlgorithm
		channels int
		layout   string
		boost    float64
		wantPan  bool
		want     string
		suffix   string
	}{
		{
			name: "ac4 pan then volume", algo: DownmixAC4, channels: 6, layout: layout51,
			boost: 1.5, wantPan: true, suffix: "volume=1.5",
		},
		{
			name: "none boost only", algo: DownmixNone, channels: 6, layout: layout51,
			boost: 2, want: "volume=2",
		},
	} {
		allure.Test(t, "compose audio filter — "+tc.name, func(a *allure.Context) {
			t := a.T()
			filter := ComposeAudioFilter(tc.algo, tc.channels, tc.layout, tc.boost)
			if tc.want != "" {
				if filter != tc.want {
					t.Fatalf("got %q want %q", filter, tc.want)
				}

				return
			}
			if tc.wantPan && !strings.Contains(filter, "pan=stereo") {
				t.Fatalf("expected pan filter, got %q", filter)
			}
			if !tc.wantPan && strings.Contains(filter, "pan=") {
				t.Fatalf("must not emit pan: %q", filter)
			}
			if tc.suffix != "" && !strings.HasSuffix(filter, tc.suffix) {
				t.Fatalf("expected suffix %q, got %q", tc.suffix, filter)
			}
		})
	}
}
