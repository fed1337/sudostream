package transcode

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseVariantDir_MapsRenditionDirectories(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"variant directories round-trip to rendition identifiers",
		func(a *allure.Context) {
			t := a.T()
			cases := []struct {
				dir  string
				want Variant
				ok   bool
			}{
				{dir: "v1080", want: VideoVariant(1080), ok: true},
				{dir: "a0", want: AudioVariant(0), ok: true},
				{dir: "a2", want: AudioVariant(2), ok: true},
				{dir: "v0", ok: false},
				{dir: "subtitles", ok: false},
				{dir: "x1", ok: false},
				{dir: "v", ok: false},
			}

			for _, tc := range cases {
				got, ok := ParseVariantDir(tc.dir)
				if ok != tc.ok {
					t.Fatalf("%q: ok got %v want %v", tc.dir, ok, tc.ok)
				}
				if ok && (got != tc.want || got.Dir() != tc.dir) {
					t.Fatalf("%q: got %+v want %+v", tc.dir, got, tc.want)
				}
			}
		},
	)
}

func TestSplitSegmentResource_ExtractsVariantAndIndex(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		resource   string
		wantParsed bool
		wantVar    Variant
		wantIndex  int
	}{
		{
			name:       "video segment",
			resource:   "v720/seg_00007.ts",
			wantParsed: true,
			wantVar:    VideoVariant(720),
			wantIndex:  7,
		},
		{
			name:       "audio segment",
			resource:   "a1/seg_00000.ts",
			wantParsed: true,
			wantVar:    AudioVariant(1),
			wantIndex:  0,
		},
		{name: "playlists are not segments", resource: "v720/playlist.m3u8"},
		{name: "subtitle resources are not segments", resource: "subtitles/eng/track.vtt"},
	} {
		allure.Test(t, "split segment resource — "+tc.name, func(a *allure.Context) {
			t := a.T()
			variant, index, parsed := SplitSegmentResource(tc.resource)
			if parsed != tc.wantParsed {
				t.Fatalf("parsed got %v want %v", parsed, tc.wantParsed)
			}
			if parsed && (variant != tc.wantVar || index != tc.wantIndex) {
				t.Fatalf("got %+v index=%d, want %+v index=%d", variant, index, tc.wantVar, tc.wantIndex)
			}
		})
	}
}

func TestValidVariant_ChecksLadderAndAudioRange(t *testing.T) {
	t.Parallel()

	meta := SourceMeta{
		Height:       480,
		AudioStreams: []AudioStream{{Index: 1}, {Index: 2}},
		Segments:     []float64{6},
	}

	for _, tc := range []struct {
		name    string
		variant Variant
		want    bool
	}{
		{name: "source-height rung is valid", variant: VideoVariant(480), want: true},
		{name: "lower ladder rung is valid", variant: VideoVariant(240), want: true},
		{name: "upscale rung is rejected", variant: VideoVariant(1080), want: false},
		{name: "in-range audio track is valid", variant: AudioVariant(1), want: true},
		{name: "out-of-range audio track is rejected", variant: AudioVariant(2), want: false},
	} {
		allure.Test(t, "valid variant — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if got := validVariant(meta, tc.variant); got != tc.want {
				t.Fatalf("validVariant(%+v) = %v, want %v", tc.variant, got, tc.want)
			}
		})
	}
}
