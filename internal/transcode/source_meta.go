package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

const (
	sourceMetaMarker       = ".source_meta.json"
	defaultAudioTrackLabel = "Default"
	defaultStubBandwidth   = 1400000
	// hlsCodecsAttr advertises H.264 High + AAC-LC; every rendition is normalized to these.
	hlsCodecsAttr = "avc1.640028,mp4a.40.2"
)

var (
	// ErrSourceMetaUnavailable is returned when cached probe metadata is missing.
	ErrSourceMetaUnavailable = errors.New("source metadata unavailable")
	// ErrQualityNotAvailable is returned when the requested ladder rung does not exist.
	ErrQualityNotAvailable = errors.New("quality not available for source")
	// ErrSegmentOutOfRange is returned for segment indexes beyond the media timeline.
	ErrSegmentOutOfRange = errors.New("segment index out of range")
)

// SourceMeta caches the probe results that drive playlist generation.
//
// Everything needed to publish the full HLS timeline lives here, so playlists are pure
// arithmetic over Segments and no ffmpeg run is required before playback can start.
type SourceMeta struct {
	Height          int              `json:"height"`
	Width           int              `json:"width,omitempty"`
	DurationSeconds float64          `json:"durationSeconds,omitempty"`
	FrameRate       float64          `json:"frameRate,omitempty"`
	VideoCodec      string           `json:"videoCodec,omitempty"`
	PackagingMode   PackagingMode    `json:"packagingMode,omitempty"`
	PixFmt          string           `json:"pixFmt,omitempty"`
	ColorTransfer   string           `json:"colorTransfer,omitempty"`
	ColorPrimaries  string           `json:"colorPrimaries,omitempty"`
	ColorSpace      string           `json:"colorSpace,omitempty"`
	AudioStreams    []AudioStream    `json:"audioStreams,omitempty"`
	SubtitleStreams []SubtitleStream `json:"subtitleStreams,omitempty"`
	// Chapters are container chapter atoms for any video (film, series, other); not series S/E.
	Chapters []ChapterInfo `json:"chapters"`
	Segments []float64     `json:"segments,omitempty"`
}

// NeedsToneMap reports whether segment encodes must apply HDR→SDR conversion.
func (m SourceMeta) NeedsToneMap() bool {
	return needsToneMap(m.ColorTransfer, m.ColorPrimaries, m.PixFmt)
}

// AudioTracks maps probed audio streams to the client track list.
func (m SourceMeta) AudioTracks() []TrackInfo {
	if len(m.AudioStreams) == 0 {
		return []TrackInfo{{ID: "0", Label: defaultAudioTrackLabel}}
	}

	tracks := make([]TrackInfo, 0, len(m.AudioStreams))
	for index, stream := range m.AudioStreams {
		tracks = append(tracks, TrackInfo{ID: strconv.Itoa(index), Label: stream.Label})
	}

	return tracks
}

// Qualities lists ladder rungs available for the source.
func (m SourceMeta) Qualities() []QualityInfo {
	heights := LadderHeights(m.Height)
	qualities := make([]QualityInfo, 0, len(heights))

	for _, height := range heights {
		qualities = append(qualities, QualityInfo{
			Height: height,
			Label:  formatQualityLabel(height),
		})
	}

	return qualities
}

// SegmentCount returns the number of segments on the shared timeline.
func (m SourceMeta) SegmentCount() int {
	return len(m.Segments)
}

// sourceMetaFromProbe builds cache metadata, including the shared segment table.
func sourceMetaFromProbe(source SourceInfo, keyframes []float64) SourceMeta {
	packaging := PackagingTranscode
	if RemuxEligible(source) {
		packaging = PackagingRemux
	}

	meta := SourceMeta{
		Height:          source.Height,
		Width:           source.Width,
		DurationSeconds: source.DurationSeconds,
		FrameRate:       source.FrameRate,
		VideoCodec:      source.VideoCodec,
		PackagingMode:   packaging,
		PixFmt:          source.PixFmt,
		ColorTransfer:   source.ColorTransfer,
		ColorPrimaries:  source.ColorPrimaries,
		ColorSpace:      source.ColorSpace,
		AudioStreams:    source.AudioStreams,
		SubtitleStreams: source.SubtitleStreams,
		Chapters:        source.Chapters,
		Segments:        SegmentTable(keyframes, source.DurationSeconds, SegmentTargetSeconds),
	}
	if meta.Chapters == nil {
		meta.Chapters = []ChapterInfo{}
	}

	return meta
}

// WriteSourceMeta persists probe results beside the transcode cache.
func WriteSourceMeta(outDir string, meta SourceMeta) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal source meta: %w", err)
	}

	//nolint:mnd // permissions
	err = os.WriteFile(filepath.Join(outDir, sourceMetaMarker), raw, 0o600)
	if err != nil {
		return fmt.Errorf("write source meta: %w", err)
	}

	return nil
}

// ReadSourceMeta loads the cached probe summary when present.
func ReadSourceMeta(outDir string) (SourceMeta, bool) {
	raw, err := os.ReadFile(filepath.Join(outDir, sourceMetaMarker))
	if err != nil {
		return SourceMeta{}, false
	}

	var meta SourceMeta

	err = json.Unmarshal(raw, &meta)
	if err != nil || len(meta.Segments) == 0 {
		return SourceMeta{}, false
	}

	return meta, true
}

// ProbeAndPublish probes a source and writes every playlist for it.
//
// This is the whole "make it playable" step: after it returns the client has a complete,
// seekable VOD timeline for every rung, audio track and subtitle track, with zero media
// encoded. Segments are produced later, on request.
func ProbeAndPublish(ctx context.Context, outDir, mediaPath string) (SourceMeta, error) {
	source, err := ProbeSource(ctx, mediaPath)
	if err != nil {
		return SourceMeta{}, err
	}

	// Keyframe extraction is best effort; equal-length segments are the documented fallback.
	// ProbeKeyframes applies its own timeout so a full-file packet scan cannot stall publish.
	keyframes, kfErr := ProbeKeyframes(ctx, mediaPath)
	if kfErr != nil {
		slog.Warn("keyframe probe skipped",
			slog.String("path", mediaPath),
			slog.String("error", kfErr.Error()),
		)
		keyframes = nil
	} else {
		_ = WriteKeyframes(outDir, keyframes)
	}

	meta := sourceMetaFromProbe(source, keyframes)
	if len(meta.Segments) == 0 {
		return SourceMeta{}, fmt.Errorf(
			"%w: no timeline for %s",
			ErrSourceMetaUnavailable,
			mediaPath,
		)
	}

	err = WriteSourceMeta(outDir, meta)
	if err != nil {
		return SourceMeta{}, err
	}

	err = PublishPlaylists(outDir, meta)
	if err != nil {
		return SourceMeta{}, err
	}

	return meta, nil
}

// PublishPlaylists writes the master plus every media playlist from the segment table.
func PublishPlaylists(outDir string, meta SourceMeta) error {
	media := BuildMediaPlaylist(meta.Segments)

	for _, height := range LadderHeights(meta.Height) {
		err := writePlaylistFile(filepath.Join(outDir, VariantDir(height), mediaPlaylist), media)
		if err != nil {
			return err
		}
	}

	for index := range meta.AudioStreams {
		err := writePlaylistFile(filepath.Join(outDir, AudioDir(index), mediaPlaylist), media)
		if err != nil {
			return err
		}
	}

	master := BuildMasterPlaylist(meta, listPackagedSubtitleTracks(outDir))

	return writePlaylistFile(filepath.Join(outDir, masterPlaylistName), master)
}

func writePlaylistFile(path string, body []byte) error {
	//nolint:mnd // permissions
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		return fmt.Errorf("mkdir playlist dir: %w", err)
	}

	//nolint:mnd // permissions
	err = os.WriteFile(path, body, 0o600)
	if err != nil {
		return fmt.Errorf("write playlist %s: %w", filepath.Base(path), err)
	}

	return nil
}
