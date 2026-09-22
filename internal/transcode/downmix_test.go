package transcode

import (
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestStereoDownmixFilter_AC4Layouts(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"AC-4 pan filters match Jellyfin DownMixAlgorithmsHelper",
		func(a *allure.Context) {
			t := a.T()
			cases := []struct {
				channels int
				layout   string
				want     string
			}{
				{channels: 2, layout: layoutStereo, want: ""},
				{channels: 6, layout: "", want: ac4PanFilters[layout51]},
				{channels: 8, layout: layout71, want: ac4PanFilters[layout71]},
				{channels: 6, layout: layout51, want: ac4PanFilters[layout51]},
				{channels: 3, layout: layout30, want: ac4PanFilters[layout30]},
			}

			for _, testCase := range cases {
				got := StereoDownmixFilter(DownmixAC4, testCase.channels, testCase.layout)
				if got != testCase.want {
					t.Fatalf("channels=%d layout=%q: got %q want %q",
						testCase.channels, testCase.layout, got, testCase.want)
				}
			}

			if StereoDownmixFilter(DownmixNone, 6, layout51) != "" {
				t.Fatal("none must not emit a pan filter")
			}
			if strings.Contains(StereoDownmixFilter(DownmixNone, 6, layout51), "volume=") {
				t.Fatal("none must not boost volume")
			}
		},
	)
}
