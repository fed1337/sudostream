package playback

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testContainerMP4  = "mp4"
	testContainerMKV  = "mkv"
	testCodecH264     = "h264"
	testCodecAAC      = "aac"
	testContainerWebM = "webm"
	testCodecVP9      = "vp9"
	testCodecOpus     = "opus"
)

func TestDecide(t *testing.T) {
	t.Parallel()

	mp4H264AAC := DirectPlayProfile{
		Container:  testContainerMP4,
		VideoCodec: testCodecH264,
		AudioCodec: testCodecAAC,
	}

	for _, tc := range []struct {
		name    string
		profile DeviceProfile
		source  SourceCaps
		quality int
		want    PlayMethod
	}{
		{
			name: "direct play when profile matches",
			profile: DeviceProfile{
				MaxStaticBitrate: 120_000_000,
				DirectPlay:       []DirectPlayProfile{{Container: testContainerMP4, VideoCodec: "h264,hevc", AudioCodec: "aac,ac3"}},
			},
			source: SourceCaps{
				Container: testContainerMP4, VideoCodec: testCodecH264, AudioCodec: testCodecAAC,
				Bitrate: 5_000_000, Height: 1080, RemuxOK: true,
			},
			want: MethodDirectPlay,
		},
		{
			name: "remux when direct play impossible",
			profile: DeviceProfile{
				DirectPlay: []DirectPlayProfile{{
					Container: testContainerWebM, VideoCodec: testCodecVP9, AudioCodec: testCodecOpus,
				}},
			},
			source: SourceCaps{
				Container: "mkv", VideoCodec: testCodecH264, AudioCodec: testCodecAAC,
				Height: 720, RemuxOK: true,
			},
			want: MethodRemux,
		},
		{
			name:    "transcode when lower quality selected",
			profile: DeviceProfile{DirectPlay: []DirectPlayProfile{mp4H264AAC}},
			source: SourceCaps{
				Container: testContainerMP4, VideoCodec: testCodecH264, AudioCodec: testCodecAAC,
				Height: 1080, RemuxOK: true,
			},
			quality: 720,
			want:    MethodTranscode,
		},
		{
			name: "preferHls skips direct play",
			profile: DeviceProfile{
				PreferHLS:  true,
				DirectPlay: []DirectPlayProfile{mp4H264AAC},
			},
			source: SourceCaps{
				Container: testContainerMP4, VideoCodec: testCodecH264, AudioCodec: testCodecAAC,
				Height: 480, RemuxOK: true,
			},
			want: MethodRemux,
		},
		{
			name: "bitrate over max blocks direct play",
			profile: DeviceProfile{
				MaxStaticBitrate: 2_000_000,
				DirectPlay:       []DirectPlayProfile{mp4H264AAC},
			},
			source: SourceCaps{
				Container: testContainerMP4, VideoCodec: testCodecH264, AudioCodec: testCodecAAC,
				Bitrate: 8_000_000, Height: 1080, RemuxOK: false,
			},
			want: MethodTranscode,
		},
		{
			name: "ac3 5.1 direct play",
			profile: DeviceProfile{
				DirectPlay: []DirectPlayProfile{{
					Container: testContainerMKV, VideoCodec: testCodecH264, AudioCodec: "ac3",
				}},
			},
			source: SourceCaps{
				Container: testContainerMKV, VideoCodec: testCodecH264, AudioCodec: "ac3",
				Height: 1080, RemuxOK: true,
			},
			want: MethodDirectPlay,
		},
	} {
		allure.Test(t, "decide playback — "+tc.name, func(a *allure.Context) {
			t := a.T()
			got := Decide(tc.profile, tc.source, tc.quality)
			if got.Method != tc.want {
				t.Fatalf("method: got %q want %q", got.Method, tc.want)
			}
		})
	}
}

func TestContainerFromPath(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "movies/foo.mp4", want: testContainerMP4},
		{path: "movies/foo.m4v", want: testContainerMP4},
		{path: "movies/foo.webm", want: testContainerWebM},
		{path: "movies/foo.mkv", want: "mkv"},
		{path: "movies/foo.MOV", want: "mov"},
	} {
		allure.Test(t, "container from path — "+tc.path, func(a *allure.Context) {
			t := a.T()
			if got := ContainerFromPath(tc.path); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
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
