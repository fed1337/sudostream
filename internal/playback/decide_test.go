package playback

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testContainerMP4  = "mp4"
	testCodecH264     = "h264"
	testCodecAAC      = "aac"
	testContainerWebM = "webm"
	testCodecVP9      = "vp9"
	testCodecOpus     = "opus"
)

func TestDecide_DirectPlayWhenProfileMatches(t *testing.T) {
	t.Parallel()

	allure.Test(t, "Direct Play when profile matches original quality", func(a *allure.Context) {
		t := a.T()
		profile := DeviceProfile{
			MaxStaticBitrate: 120_000_000,
			DirectPlay: []DirectPlayProfile{{
				Container:  testContainerMP4,
				VideoCodec: "h264,hevc",
				AudioCodec: "aac,ac3",
			}},
		}
		source := SourceCaps{
			Container:  testContainerMP4,
			VideoCodec: testCodecH264,
			AudioCodec: testCodecAAC,
			Bitrate:    5_000_000,
			Height:     1080,
			RemuxOK:    true,
		}

		got := Decide(profile, source, 0)
		if got.Method != MethodDirectPlay {
			t.Fatalf("method: got %q want %q", got.Method, MethodDirectPlay)
		}
	})
}

func TestDecide_RemuxWhenDirectPlayImpossible(t *testing.T) {
	t.Parallel()

	allure.Test(t, "remux when original quality and remux-eligible but profile misses", func(a *allure.Context) {
		t := a.T()
		profile := DeviceProfile{
			DirectPlay: []DirectPlayProfile{{
				Container:  testContainerWebM,
				VideoCodec: testCodecVP9,
				AudioCodec: testCodecOpus,
			}},
		}
		source := SourceCaps{
			Container:  "mkv",
			VideoCodec: testCodecH264,
			AudioCodec: testCodecAAC,
			Height:     720,
			RemuxOK:    true,
		}

		got := Decide(profile, source, 0)
		if got.Method != MethodRemux {
			t.Fatalf("method: got %q want %q", got.Method, MethodRemux)
		}
	})
}

func TestDecide_TranscodeWhenLowerQualitySelected(t *testing.T) {
	t.Parallel()

	allure.Test(t, "transcode when user picks a lower quality rung", func(a *allure.Context) {
		t := a.T()
		profile := DeviceProfile{
			DirectPlay: []DirectPlayProfile{{
				Container:  testContainerMP4,
				VideoCodec: testCodecH264,
				AudioCodec: testCodecAAC,
			}},
		}
		source := SourceCaps{
			Container:  testContainerMP4,
			VideoCodec: testCodecH264,
			AudioCodec: testCodecAAC,
			Height:     1080,
			RemuxOK:    true,
		}

		got := Decide(profile, source, 720)
		if got.Method != MethodTranscode {
			t.Fatalf("method: got %q want %q", got.Method, MethodTranscode)
		}
	})
}

func TestDecide_PreferHlsSkipsDirectPlay(t *testing.T) {
	t.Parallel()

	allure.Test(t, "preferHls forces remux/transcode path", func(a *allure.Context) {
		t := a.T()
		profile := DeviceProfile{
			PreferHLS: true,
			DirectPlay: []DirectPlayProfile{{
				Container:  testContainerMP4,
				VideoCodec: testCodecH264,
				AudioCodec: testCodecAAC,
			}},
		}
		source := SourceCaps{
			Container:  testContainerMP4,
			VideoCodec: testCodecH264,
			AudioCodec: testCodecAAC,
			Height:     480,
			RemuxOK:    true,
		}

		got := Decide(profile, source, 0)
		if got.Method != MethodRemux {
			t.Fatalf("method: got %q want %q", got.Method, MethodRemux)
		}
	})
}

func TestDecide_BitrateOverMaxBlocksDirectPlay(t *testing.T) {
	t.Parallel()

	allure.Test(t, "bitrate above maxStaticBitrate blocks Direct Play", func(a *allure.Context) {
		t := a.T()
		profile := DeviceProfile{
			MaxStaticBitrate: 2_000_000,
			DirectPlay: []DirectPlayProfile{{
				Container:  testContainerMP4,
				VideoCodec: testCodecH264,
				AudioCodec: testCodecAAC,
			}},
		}
		source := SourceCaps{
			Container:  testContainerMP4,
			VideoCodec: testCodecH264,
			AudioCodec: testCodecAAC,
			Bitrate:    8_000_000,
			Height:     1080,
			RemuxOK:    false,
		}

		got := Decide(profile, source, 0)
		if got.Method != MethodTranscode {
			t.Fatalf("method: got %q want %q", got.Method, MethodTranscode)
		}
	})
}

func TestContainerFromPath(t *testing.T) {
	t.Parallel()

	allure.Test(t, "container tokens follow common video extensions", func(a *allure.Context) {
		t := a.T()
		cases := map[string]string{
			"movies/foo.mp4":  testContainerMP4,
			"movies/foo.m4v":  testContainerMP4,
			"movies/foo.webm": testContainerWebM,
			"movies/foo.mkv":  "mkv",
			"movies/foo.MOV":  "mov",
		}
		for path, want := range cases {
			if got := ContainerFromPath(path); got != want {
				t.Fatalf("%s: got %q want %q", path, got, want)
			}
		}
	})
}

func TestNormalizeCodecAliases(t *testing.T) {
	t.Parallel()

	allure.Test(t, "avc1 matches h264 profile entry", func(a *allure.Context) {
		t := a.T()
		entry := DirectPlayProfile{
			Container:  testContainerMP4,
			VideoCodec: testCodecH264,
			AudioCodec: testCodecAAC,
		}
		source := SourceCaps{
			Container:  testContainerMP4,
			VideoCodec: "avc1",
			AudioCodec: testCodecAAC,
		}
		if !matchesDirectPlay(entry, source) {
			t.Fatal("expected avc1 to match h264 profile")
		}
	})
}
