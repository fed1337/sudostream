// Package playback decides Direct Play vs HLS remux vs HLS transcode from a
// Jellyfin-style device profile and source media capabilities (FI-8).
package playback

// PlayMethod is how the server will deliver media for a negotiate request.
type PlayMethod string

const (
	// MethodDirectPlay serves the original file via Range (/api/stream).
	MethodDirectPlay PlayMethod = "directPlay"
	// MethodRemux packages the source into HLS with stream-copy video.
	MethodRemux PlayMethod = "remux"
	// MethodTranscode re-encodes HLS (lower quality rungs or ineligible source).
	MethodTranscode PlayMethod = "transcode"
)

// DeviceProfile describes browser playback capabilities (rebuilt each play).
type DeviceProfile struct {
	MaxStaticBitrate int64               `json:"maxStaticBitrate,omitempty"`
	DirectPlay       []DirectPlayProfile `json:"directPlay,omitempty"`
	PreferHLS        bool                `json:"preferHls,omitempty"`
	// Subtitle lists supported subtitle delivery modes (e.g. "external").
	Subtitle []string `json:"subtitle,omitempty"`
}

// DirectPlayProfile is one container + codec combination the client can play.
// Codec fields are comma-separated lists (Jellyfin-style), case-insensitive.
type DirectPlayProfile struct {
	Container  string `json:"container"`
	VideoCodec string `json:"videoCodec,omitempty"`
	AudioCodec string `json:"audioCodec,omitempty"`
}

// SourceCaps is the probed/derived media identity used by Decide.
type SourceCaps struct {
	Container       string
	VideoCodec      string
	AudioCodec      string
	Bitrate         int64
	Height          int
	DurationSeconds float64
	RemuxOK         bool
}
