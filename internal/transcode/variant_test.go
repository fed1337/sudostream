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

	allure.Test(t, "segment resources resolve to a rendition and index", func(a *allure.Context) {
		t := a.T()
		variant, index, parsed := SplitSegmentResource("v720/seg_00007.ts")
		if !parsed || variant != VideoVariant(720) || index != 7 {
			t.Fatalf("got %+v index=%d parsed=%v", variant, index, parsed)
		}

		variant, index, parsed = SplitSegmentResource("a1/seg_00000.ts")
		if !parsed || variant != AudioVariant(1) || index != 0 {
			t.Fatalf("got %+v index=%d parsed=%v", variant, index, parsed)
		}

		if _, _, parsed = SplitSegmentResource("v720/playlist.m3u8"); parsed {
			t.Fatal("playlists are not segments")
		}
		if _, _, parsed = SplitSegmentResource("subtitles/eng/track.vtt"); parsed {
			t.Fatal("subtitle resources are not segments")
		}
	})
}

func TestValidVariant_ChecksLadderAndAudioRange(t *testing.T) {
	t.Parallel()

	allure.Test(t, "only published renditions are accepted", func(a *allure.Context) {
		t := a.T()
		meta := SourceMeta{
			Height:       480,
			AudioStreams: []AudioStream{{Index: 1}, {Index: 2}},
			Segments:     []float64{6},
		}

		if !validVariant(meta, VideoVariant(480)) || !validVariant(meta, VideoVariant(240)) {
			t.Fatal("expected ladder rungs to be valid")
		}
		if validVariant(meta, VideoVariant(1080)) {
			t.Fatal("expected upscale rung to be rejected")
		}
		if !validVariant(meta, AudioVariant(1)) {
			t.Fatal("expected second audio track to be valid")
		}
		if validVariant(meta, AudioVariant(2)) {
			t.Fatal("expected out-of-range audio track to be rejected")
		}
	})
}
