package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type probeOutput struct {
	Format   probeFormat    `json:"format"`
	Streams  []probeStream  `json:"streams"`
	Chapters []probeChapter `json:"chapters"`
}

// probeChapter mirrors ffprobe -show_chapters JSON (any container with chapter atoms).
type probeChapter struct {
	StartTime string            `json:"start_time"` //nolint:tagliatelle // ffprobe JSON
	EndTime   string            `json:"end_time"`   //nolint:tagliatelle // ffprobe JSON
	Tags      map[string]string `json:"tags"`
}

type probeFormat struct {
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`    //nolint:tagliatelle // ffprobe JSON
	FormatName string `json:"format_name"` //nolint:tagliatelle // ffprobe JSON
}

type probeStream struct {
	Index          int               `json:"index"`
	CodecType      string            `json:"codec_type"`     //nolint:tagliatelle // ffprobe JSON
	CodecName      string            `json:"codec_name"`     //nolint:tagliatelle // ffprobe JSON
	AvgFrameRate   string            `json:"avg_frame_rate"` //nolint:tagliatelle // ffprobe JSON
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	Channels       int               `json:"channels"`
	ChannelLayout  string            `json:"channel_layout"`  //nolint:tagliatelle // ffprobe JSON
	PixFmt         string            `json:"pix_fmt"`         //nolint:tagliatelle // ffprobe JSON
	ColorTransfer  string            `json:"color_transfer"`  //nolint:tagliatelle // ffprobe JSON
	ColorPrimaries string            `json:"color_primaries"` //nolint:tagliatelle // ffprobe JSON
	ColorSpace     string            `json:"color_space"`     //nolint:tagliatelle // ffprobe JSON
	Tags           map[string]string `json:"tags"`
	Disposition    map[string]int    `json:"disposition"`
}

const (
	codecTypeVideo     = "video"
	codecTypeAudio     = "audio"
	codecTypeSubtitle  = "subtitle"
	colorTransferPQ    = "smpte2084"
	colorTransferHLG   = "arib-std-b67"
	colorTransferPQv   = "smpte2084_pq"
	colorPrimaries2020 = "bt2020"
)

// ChapterInfo is one navigable chapter for the player TimeSlider (any video file).
type ChapterInfo struct {
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
	Title        string  `json:"title"`
}

// SourceInfo holds probed stream metadata for HLS packaging and Direct Play.
type SourceInfo struct {
	Height           int
	Width            int
	FrameRate        float64
	DurationSeconds  float64
	AudioStreams     []AudioStream
	SubtitleStreams  []SubtitleStream
	Chapters         []ChapterInfo
	VideoCodec       string
	AudioCodec       string
	VideoStreamCount int
	PixFmt           string
	ColorTransfer    string
	ColorPrimaries   string
	ColorSpace       string
	// Bitrate is the container bit_rate from ffprobe when present (bits/sec).
	Bitrate int64
	// FormatName is ffprobe format_name (e.g. "mov,mp4,m4a,3gp,3g2,mj2").
	FormatName string
}

// SubtitleStream describes one source subtitle track.
type SubtitleStream struct {
	Index    int    `json:"index"`
	Label    string `json:"label"`
	Language string `json:"language,omitempty"`
}

// AudioStream describes one source audio track.
type AudioStream struct {
	Index         int    `json:"index"`
	Label         string `json:"label"`
	Codec         string `json:"codec"`
	Language      string `json:"language,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	ChannelLayout string `json:"channelLayout,omitempty"`
}

// NeedsToneMap reports whether HDR/wide-gamut/10-bit video must be converted for SDR HLS.
func (s SourceInfo) NeedsToneMap() bool {
	return needsToneMap(s.ColorTransfer, s.ColorPrimaries, s.PixFmt)
}

func needsToneMap(transfer, primaries, pixFmt string) bool {
	transfer = strings.ToLower(strings.TrimSpace(transfer))
	primaries = strings.ToLower(strings.TrimSpace(primaries))
	pixFmt = strings.ToLower(strings.TrimSpace(pixFmt))

	switch transfer {
	case colorTransferPQ, colorTransferHLG, colorTransferPQv:
		return true
	}

	if primaries == colorPrimaries2020 {
		return true
	}

	if strings.Contains(pixFmt, "10") || strings.Contains(pixFmt, "12") ||
		strings.Contains(pixFmt, "p010") || strings.Contains(pixFmt, "p012") {
		return true
	}

	return false
}

// ProbeSource inspects a media file with ffprobe.
func ProbeSource(ctx context.Context, mediaPath string) (SourceInfo, error) {
	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		mediaPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return SourceInfo{}, fmt.Errorf("ffprobe %s: %w", filepath.Base(mediaPath), err)
	}

	var parsed probeOutput
	err = json.Unmarshal(output, &parsed)
	if err != nil {
		return SourceInfo{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	info := sourceInfoFromProbeOutput(parsed)
	info.DurationSeconds = parseProbeDuration(parsed.Format.Duration)
	info.FormatName = strings.TrimSpace(parsed.Format.FormatName)
	info.Bitrate = parseProbeBitrate(parsed.Format.BitRate)
	info.Chapters = mapProbeChapters(parsed.Chapters)

	return info, nil
}

// ProbeChapters inspects chapter atoms only (fallback when cached source meta lacks them).
func ProbeChapters(ctx context.Context, mediaPath string) ([]ChapterInfo, error) {
	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_chapters",
		mediaPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe chapters %s: %w", filepath.Base(mediaPath), err)
	}

	var parsed probeOutput
	err = json.Unmarshal(output, &parsed)
	if err != nil {
		return nil, fmt.Errorf("parse ffprobe chapters json: %w", err)
	}

	return mapProbeChapters(parsed.Chapters), nil
}

func mapProbeChapters(raw []probeChapter) []ChapterInfo {
	if len(raw) == 0 {
		return []ChapterInfo{}
	}

	chapters := make([]ChapterInfo, 0, len(raw))
	for index, chapter := range raw {
		title := ""
		if chapter.Tags != nil {
			title = strings.TrimSpace(chapter.Tags["title"])
		}
		if title == "" {
			title = fmt.Sprintf("Chapter %d", index+1)
		}

		chapters = append(chapters, ChapterInfo{
			StartSeconds: parseProbeDuration(chapter.StartTime),
			EndSeconds:   parseProbeDuration(chapter.EndTime),
			Title:        title,
		})
	}

	return chapters
}

func sourceInfoFromProbeOutput(parsed probeOutput) SourceInfo {
	info := SourceInfo{
		Height:       0,
		AudioStreams: []AudioStream{},
	}

	defaultAudioIndex := -1

	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case codecTypeVideo:
			applyVideoStream(&info, stream)
		case codecTypeAudio:
			appendAudioStream(&info, stream, &defaultAudioIndex)
		case codecTypeSubtitle:
			lang := ""
			if stream.Tags != nil {
				lang = strings.TrimSpace(stream.Tags["language"])
			}
			info.SubtitleStreams = append(info.SubtitleStreams, SubtitleStream{
				Index:    stream.Index,
				Label:    subtitleLabel(stream),
				Language: lang,
			})
		}
	}

	promoteDefaultAudioStream(&info, defaultAudioIndex)

	if len(info.AudioStreams) > 0 {
		info.AudioCodec = info.AudioStreams[0].Codec
	}

	return info
}

func applyVideoStream(info *SourceInfo, stream probeStream) {
	if stream.Disposition["attached_pic"] == 1 {
		return
	}

	info.VideoStreamCount++
	if info.VideoStreamCount == 1 {
		info.VideoCodec = stream.CodecName
		info.FrameRate = parseFrameRate(stream.AvgFrameRate)
		info.PixFmt = strings.TrimSpace(stream.PixFmt)
		info.ColorTransfer = strings.TrimSpace(stream.ColorTransfer)
		info.ColorPrimaries = strings.TrimSpace(stream.ColorPrimaries)
		info.ColorSpace = strings.TrimSpace(stream.ColorSpace)
	}

	if stream.Height > info.Height {
		info.Height = stream.Height
		info.Width = stream.Width
	}
}

// parseFrameRate converts ffprobe's `num/den` rational to frames per second.
func parseFrameRate(raw string) float64 {
	numerator, denominator, ok := strings.Cut(strings.TrimSpace(raw), "/")
	if !ok {
		return 0
	}

	num, err := strconv.ParseFloat(numerator, 64)
	if err != nil {
		return 0
	}

	den, err := strconv.ParseFloat(denominator, 64)
	if err != nil || den == 0 {
		return 0
	}

	return num / den
}

func appendAudioStream(info *SourceInfo, stream probeStream, defaultAudioIndex *int) {
	if stream.Disposition["default"] == 1 && *defaultAudioIndex < 0 {
		*defaultAudioIndex = len(info.AudioStreams)
	}

	info.AudioStreams = append(info.AudioStreams, AudioStream{
		Index:         stream.Index,
		Label:         audioLabel(stream),
		Codec:         stream.CodecName,
		Language:      strings.TrimSpace(stream.Tags["language"]),
		Channels:      stream.Channels,
		ChannelLayout: strings.TrimSpace(stream.ChannelLayout),
	})
}

// promoteDefaultAudioStream moves the ffprobe disposition-flagged default audio track to
// index 0 so every downstream consumer (remux eligibility, initial packaging, track labels)
// treats it as the track to play first. Falls back to the existing ffprobe stream order
// (AudioStreams[0]) when no track is flagged default.
func promoteDefaultAudioStream(info *SourceInfo, defaultAudioIndex int) {
	if defaultAudioIndex <= 0 {
		return
	}

	streams := info.AudioStreams
	def := streams[defaultAudioIndex]
	streams = append(streams[:defaultAudioIndex], streams[defaultAudioIndex+1:]...)
	info.AudioStreams = append([]AudioStream{def}, streams...)
}

func audioLabel(stream probeStream) string {
	if stream.Tags != nil {
		if title := strings.TrimSpace(stream.Tags["title"]); title != "" {
			return title
		}
		if lang := strings.TrimSpace(stream.Tags["language"]); lang != "" {
			return lang
		}
	}

	if stream.Channels > 0 {
		return fmt.Sprintf("Audio %d (%d ch)", stream.Index, stream.Channels)
	}

	return fmt.Sprintf("Audio %d", stream.Index)
}

func subtitleLabel(stream probeStream) string {
	if stream.Tags != nil {
		if title := strings.TrimSpace(stream.Tags["title"]); title != "" {
			return title
		}
		if lang := strings.TrimSpace(stream.Tags["language"]); lang != "" {
			return lang
		}
	}

	return fmt.Sprintf("Subtitles %d", stream.Index)
}

// QualityRung describes one HLS video variant.
type QualityRung struct {
	Height  int
	Bitrate string
	MaxRate string
	BufSize string
}

// QualityRungs returns HLS ladder rungs capped by source height (no upscale).
func QualityRungs(sourceHeight int) []QualityRung {
	//nolint:mnd // HLS ladder heights and bitrates are domain constants
	all := []QualityRung{
		{Height: 2160, Bitrate: "14000k", MaxRate: "14980k", BufSize: "21000k"},
		{Height: 1440, Bitrate: "8000k", MaxRate: "8560k", BufSize: "12000k"},
		{Height: 1080, Bitrate: "5000k", MaxRate: "5350k", BufSize: "7500k"},
		{Height: 720, Bitrate: "2800k", MaxRate: "2996k", BufSize: "4200k"},
		{Height: 480, Bitrate: "1400k", MaxRate: "1498k", BufSize: "2100k"},
		{Height: 360, Bitrate: "800k", MaxRate: "856k", BufSize: "1200k"},
		{Height: 240, Bitrate: "400k", MaxRate: "428k", BufSize: "600k"},
	}

	if sourceHeight <= 0 {
		return []QualityRung{all[len(all)-1]}
	}

	rungs := make([]QualityRung, 0, len(all))
	for _, rung := range all {
		if rung.Height <= sourceHeight {
			rungs = append(rungs, rung)
		}
	}

	if len(rungs) == 0 {
		rungs = append(rungs, QualityRung{
			Height:  sourceHeight,
			Bitrate: "1400k",
			MaxRate: "1498k",
			BufSize: "2100k",
		})
	}

	return rungs
}

// FastStartRung returns the highest applicable ladder rung for first-playback packaging.
func FastStartRung(sourceHeight int) QualityRung {
	rungs := QualityRungs(sourceHeight)

	return rungs[0]
}
