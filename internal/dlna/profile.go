package dlna

import "sudoStream/internal/playback"

const (
	tvMaxStaticBitrate = 120_000_000
	tvAudioCodecs      = "aac,ac3,eac3,mp3"
)

// TVDeviceProfile is a conservative generic living-room profile for Decide.
func TVDeviceProfile() playback.DeviceProfile {
	return playback.DeviceProfile{
		MaxStaticBitrate: tvMaxStaticBitrate,
		PreferHLS:        false,
		DirectPlay: []playback.DirectPlayProfile{
			{
				Container:  "mp4,m4v",
				VideoCodec: "h264,hevc",
				AudioCodec: tvAudioCodecs,
			},
			{
				Container:  "mpegts,ts,m2ts",
				VideoCodec: "h264,hevc",
				AudioCodec: tvAudioCodecs,
			},
			{
				Container:  "mkv",
				VideoCodec: "h264",
				AudioCodec: tvAudioCodecs,
			},
		},
		Subtitle: []string{"external"},
	}
}

// StreamModeForDecision maps FI-8 Decide output to DLNA delivery.
func StreamModeForDecision(method playback.PlayMethod) StreamMode {
	switch method {
	case playback.MethodDirectPlay:
		return StreamDirect
	case playback.MethodRemux:
		return StreamRemux
	case playback.MethodTranscode:
		return StreamTranscode
	default:
		return StreamTranscode
	}
}
