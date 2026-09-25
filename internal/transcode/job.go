// Package transcode manages ffmpeg jobs for HLS streaming.
package transcode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sudoStream/internal/observability"
	"time"
)

const (
	jobMetricSuccess = "success"
	jobMetricError   = "error"
	// subtitlePackageTimeout bounds embedded/sidecar → WebVTT conversion so a stuck
	// ffmpeg subtitle demux cannot leave playback in "processing" forever.
	subtitlePackageTimeout = 45 * time.Second
)

// Job publishes the HLS timeline for one media file. It probes the source and writes every
// playlist; no media is encoded here — segments are produced on demand by segment sessions.
type Job struct {
	cacheKey  string
	mediaPath string
	outDir    string
	Failed    bool
	ErrorMsg  string
}

// Run probes the source and publishes master and media playlists.
func (j *Job) Run(service *Service) {
	defer service.wg.Done()
	defer service.finishJob(j)

	start := time.Now()
	status := jobMetricSuccess

	defer func() {
		observability.RecordTranscodeTimelineJob(status, time.Since(start))
	}()

	ctx, cancel := service.workerContext()
	defer cancel()

	slog.Info("hls probe started", slog.String("path", j.mediaPath))

	meta, err := ProbeAndPublish(ctx, j.outDir, j.mediaPath)
	if err != nil {
		j.fail(err, "probe failed")
		status = jobMetricError

		observability.RecordFFmpegError("probe")

		return
	}

	_ = WriteSourceDuration(j.outDir, meta.DurationSeconds)

	// Mark ready as soon as the seekable VOD timeline exists. Subtitle packaging can be
	// slow (or hang on bad embedded tracks); do not block playback on it.
	markTranscodeComplete(j.outDir)
	slog.Info("hls timeline published",
		slog.String("path", j.mediaPath),
		slog.Int("height", meta.Height),
		slog.Int("segments", meta.SegmentCount()),
		slog.String("packaging", string(meta.PackagingMode)),
	)

	subCtx, subCancel := context.WithTimeout(ctx, subtitlePackageTimeout)
	err = EnsurePackagedSubtitles(subCtx, j.mediaPath, j.outDir)
	subCancel()
	if err != nil {
		slog.Warn("subtitle packaging failed",
			slog.String("path", j.mediaPath),
			slog.String("error", err.Error()),
		)
	}

	err = PublishPlaylists(j.outDir, meta)
	if err != nil {
		slog.Warn("subtitle playlist refresh failed",
			slog.String("path", j.mediaPath),
			slog.String("error", err.Error()),
		)
	}
}

func appendHWGlobalArgs(args []string, encoder VideoEncoder, render string) []string {
	switch encoder {
	case EncoderNVENC:
		return append(args, "-hwaccel", "cuda", "-hwaccel_output_format", "cuda")
	case EncoderQSV:
		return append(args, "-hwaccel", "qsv", "-hwaccel_output_format", "qsv")
	case EncoderVAAPI:
		if render == "" {
			return args
		}

		return append(args,
			"-init_hw_device", "vaapi=va:"+render,
			"-filter_hw_device", "va",
		)
	case EncoderRockchip:
		return append(args, "-init_hw_device", "rkmpp=rk")
	case EncoderSoftware:
		return args
	default:
		return args
	}
}

func appendVideoEncodeArgs(
	args []string,
	encoder VideoEncoder,
	rung QualityRung,
	toneMap ToneMapMode,
	algo ToneMappingAlgorithm,
) []string {
	bitrate := []string{
		"-b:v", rung.Bitrate,
		"-maxrate", rung.MaxRate,
		"-bufsize", rung.BufSize,
	}

	switch encoder {
	case EncoderNVENC:
		return append(append(args,
			"-vf", nvencVideoFilter(rung, toneMap, algo),
			"-c:v", string(EncoderNVENC),
			"-preset", "p4",
		), bitrate...)
	case EncoderQSV:
		return append(append(args,
			"-vf", qsvVideoFilter(rung, toneMap, algo),
			"-c:v", string(EncoderQSV),
		), bitrate...)
	case EncoderVAAPI:
		return append(append(args,
			"-vf", vaapiVideoFilter(rung, toneMap, algo),
			"-c:v", string(EncoderVAAPI),
		), bitrate...)
	case EncoderRockchip:
		return append(append(args,
			"-vf", rockchipVideoFilter(rung, toneMap, algo),
			"-c:v", string(EncoderRockchip),
		), bitrate...)
	case EncoderSoftware:
		return appendSoftwareEncodeArgs(args, rung, toneMap, algo, bitrate)
	default:
		return appendSoftwareEncodeArgs(args, rung, toneMap, algo, bitrate)
	}
}

func softwareScaleExpr(height int) string {
	return fmt.Sprintf("scale=-2:min(%d\\,ih)", height)
}

func nvencVideoFilter(rung QualityRung, toneMap ToneMapMode, algo ToneMappingAlgorithm) string {
	scaleCUDA := fmt.Sprintf("scale_cuda=-2:min(%d\\,ih):format=nv12", rung.Height)
	switch toneMap {
	case ToneMapCUDA:
		return joinFilters(hwTonemapFilter("cuda", "nv12", algo), scaleCUDA)
	case ToneMapCPU:
		return joinFilters(cpuTonemapFilter(algo),
			fmt.Sprintf("scale=-2:min(%d\\,ih),format=nv12,hwupload_cuda", rung.Height))
	case ToneMapNone, ToneMapQSVVPP, ToneMapVulkan, ToneMapOpenCL:
		return scaleCUDA
	default:
		return scaleCUDA
	}
}

func qsvVideoFilter(rung QualityRung, toneMap ToneMapMode, algo ToneMappingAlgorithm) string {
	if toneMap == ToneMapQSVVPP {
		return qsvVPPTonemapScale(rung.Height)
	}

	scaleQSV := fmt.Sprintf("scale_qsv=-2:min(%d\\,ih):format=nv12", rung.Height)
	if toneMap == ToneMapCPU {
		return joinFilters(
			cpuTonemapFilter(algo),
			fmt.Sprintf(
				"format=nv12,hwupload=extra_hw_frames=64,scale_qsv=-2:min(%d\\,ih):format=nv12",
				rung.Height,
			),
		)
	}

	return scaleQSV
}

func vaapiVideoFilter(rung QualityRung, toneMap ToneMapMode, algo ToneMappingAlgorithm) string {
	// Ladder scale via scale_vaapi (E-5 E1). Never VAAPI VPP tonemap.
	scaleVA := fmt.Sprintf("scale_vaapi=w=-2:h=min(%d\\,ih):format=nv12", rung.Height)
	switch toneMap {
	case ToneMapVulkan:
		return joinFilters(vulkanTonemapFilter("nv12", algo), "hwupload", scaleVA)
	case ToneMapCPU:
		return joinFilters(cpuTonemapFilter(algo), "format=nv12,hwupload", scaleVA)
	case ToneMapNone, ToneMapCUDA, ToneMapQSVVPP, ToneMapOpenCL:
		return joinFilters("format=nv12,hwupload", scaleVA)
	default:
		return joinFilters("format=nv12,hwupload", scaleVA)
	}
}

func rockchipVideoFilter(rung QualityRung, toneMap ToneMapMode, algo ToneMappingAlgorithm) string {
	scaleExpr := softwareScaleExpr(rung.Height)
	switch toneMap {
	case ToneMapOpenCL:
		return joinFilters(
			"format=nv12,hwupload=derive_device=opencl",
			hwTonemapFilter("opencl", "nv12", algo),
			"hwdownload,format=nv12",
			scaleExpr,
		)
	case ToneMapCPU:
		return joinFilters(cpuTonemapFilter(algo), scaleExpr)
	case ToneMapNone, ToneMapCUDA, ToneMapQSVVPP, ToneMapVulkan:
		return scaleExpr
	default:
		return scaleExpr
	}
}

func appendSoftwareEncodeArgs(
	args []string,
	rung QualityRung,
	toneMap ToneMapMode,
	algo ToneMappingAlgorithm,
	bitrate []string,
) []string {
	videoFilter := softwareScaleExpr(rung.Height)
	if toneMap != ToneMapNone {
		videoFilter = joinFilters(cpuTonemapFilter(algo), videoFilter)
	}

	return append(append(args,
		"-vf", videoFilter,
		"-c:v", string(EncoderSoftware),
		"-preset", "veryfast",
		"-crf", "23",
	), bitrate...)
}

func (j *Job) fail(err error, detail string) {
	j.Failed = true
	j.ErrorMsg = strings.TrimSpace(detail)
	if j.ErrorMsg == "" {
		j.ErrorMsg = err.Error()
	}

	slog.Error("ffmpeg transcode failed",
		slog.String("path", j.mediaPath),
		slog.String("error", err.Error()),
		slog.String("detail", j.ErrorMsg),
	)
}

func attachSidecarSubtitles(
	ctx context.Context,
	mediaPath, outDir string,
	durationSeconds float64,
) error {
	candidates, err := sidecarSubtitleCandidates(mediaPath)
	if err != nil {
		return err
	}

	if len(candidates) == 0 {
		return nil
	}

	subtitlesDir := filepath.Join(outDir, "subtitles")
	//nolint:mnd // permissions
	err = os.MkdirAll(subtitlesDir, 0o750)
	if err != nil {
		return fmt.Errorf("mkdir subtitles: %w", err)
	}

	for _, candidate := range candidates {
		trackID := sidecarSubtitleLabel(mediaPath, candidate)
		trackDir := filepath.Join(subtitlesDir, trackID)
		outVTT := filepath.Join(trackDir, subtitleSegmentName)

		//nolint:mnd // permissions
		err = os.MkdirAll(trackDir, 0o750)
		if err != nil {
			return fmt.Errorf("mkdir subtitle track %q: %w", trackID, err)
		}

		convErr := convertSubtitleToWebVTT(ctx, candidate, outVTT)
		if convErr != nil {
			slog.Warn("sidecar subtitle conversion failed",
				slog.String("path", candidate),
				slog.String("error", convErr.Error()),
			)

			_ = os.RemoveAll(trackDir)

			continue
		}

		playlistErr := writeSubtitleMediaPlaylist(
			filepath.Join(trackDir, "playlist.m3u8"),
			subtitleSegmentName,
			durationSeconds,
		)
		if playlistErr != nil {
			return playlistErr
		}
	}

	return nil
}

// EnsurePackagedSubtitles attaches sidecar or embedded subtitles when the cache has none.
func EnsurePackagedSubtitles(ctx context.Context, mediaPath, outDir string) error {
	purgeInvalidSubtitleTracks(outDir)

	durationSeconds := sourceDurationForSubtitles(outDir, SourceInfo{})
	if durationSeconds <= 1 {
		source, err := ProbeSource(ctx, mediaPath)
		if err == nil {
			durationSeconds = sourceDurationForSubtitles(outDir, source)
		}
	}

	playlistErr := ensureSubtitleMediaPlaylists(outDir, durationSeconds)
	if playlistErr != nil {
		return playlistErr
	}

	if hasPackagedSubtitleTracks(outDir) {
		return nil
	}

	err := attachSidecarSubtitles(ctx, mediaPath, outDir, durationSeconds)
	if err != nil {
		return err
	}

	if hasPackagedSubtitleTracks(outDir) {
		return nil
	}

	embedErr := attachEmbeddedSubtitles(ctx, mediaPath, outDir, durationSeconds)
	if embedErr != nil {
		return embedErr
	}

	return ensureSubtitleMediaPlaylists(outDir, durationSeconds)
}

func attachEmbeddedSubtitles(
	ctx context.Context,
	mediaPath, outDir string,
	durationSeconds float64,
) error {
	source, err := ProbeSource(ctx, mediaPath)
	if err != nil {
		slog.Warn("embedded subtitle probe failed",
			slog.String("path", mediaPath),
			slog.String("error", err.Error()),
		)

		return nil
	}

	if len(source.SubtitleStreams) == 0 {
		return nil
	}

	subtitlesDir := filepath.Join(outDir, "subtitles")
	//nolint:mnd // permissions
	err = os.MkdirAll(subtitlesDir, 0o750)
	if err != nil {
		return fmt.Errorf("mkdir subtitles: %w", err)
	}

	for index, stream := range source.SubtitleStreams {
		trackID := subtitleVTTFilename(stream.Label, index)
		trackDir := filepath.Join(subtitlesDir, trackID)
		outVTT := filepath.Join(trackDir, subtitleSegmentName)

		//nolint:mnd // permissions
		err = os.MkdirAll(trackDir, 0o750)
		if err != nil {
			return fmt.Errorf("mkdir subtitle track %q: %w", trackID, err)
		}

		convErr := convertEmbeddedSubtitleToWebVTT(ctx, mediaPath, stream.Index, outVTT)
		if convErr != nil {
			slog.Warn("embedded subtitle conversion failed",
				slog.String("path", mediaPath),
				slog.Int("stream", stream.Index),
				slog.String("error", convErr.Error()),
			)

			_ = os.RemoveAll(trackDir)

			continue
		}

		playlistErr := writeSubtitleMediaPlaylist(
			filepath.Join(trackDir, "playlist.m3u8"),
			subtitleSegmentName,
			durationSeconds,
		)
		if playlistErr != nil {
			return playlistErr
		}
	}

	return nil
}

func subtitleVTTFilename(label string, index int) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return strconv.Itoa(index)
	}

	safe := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, label)

	if safe == "" {
		return strconv.Itoa(index)
	}

	return safe
}

var errSubtitleNoCues = errors.New("subtitle conversion produced no cues")

var sidecarSubtitleEncodings = []string{"CP1251", "", "CP866", "ISO-8859-1"}

func convertSubtitleToWebVTT(ctx context.Context, inputPath, outputPath string) error {
	if strings.EqualFold(filepath.Ext(inputPath), ".srt") {
		goErr := convertSRTFileToWebVTT(inputPath, outputPath)
		if goErr == nil && subtitleVTTHasCues(outputPath) {
			return nil
		}

		_ = os.Remove(outputPath)
	}

	for _, encoding := range sidecarSubtitleEncodings {
		_ = os.Remove(outputPath)

		args := []string{"-v", "warning", "-y"}
		if encoding != "" {
			args = append(args, "-sub_charenc", encoding)
		}

		args = append(args, "-i", inputPath, "-c:s", "webvtt", outputPath)

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		runErr := cmd.Run()
		if runErr != nil {
			continue
		}

		if subtitleVTTHasCues(outputPath) {
			if encoding != "" {
				slog.Info("sidecar subtitle converted with legacy encoding",
					slog.String("path", inputPath),
					slog.String("encoding", encoding),
				)
			}

			return nil
		}
	}

	return fmt.Errorf("convert %s: %w", filepath.Base(inputPath), errSubtitleNoCues)
}

func convertEmbeddedSubtitleToWebVTT(
	ctx context.Context,
	mediaPath string,
	streamIndex int,
	outputPath string,
) error {
	_ = os.Remove(outputPath)

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-v", "warning",
		"-y",
		"-i", mediaPath,
		"-map", mapInputStream(streamIndex),
		"-c:s", "webvtt",
		outputPath,
	)
	runErr := cmd.Run()
	if runErr != nil {
		return fmt.Errorf("ffmpeg embedded subtitle stream %d: %w", streamIndex, runErr)
	}

	if !subtitleVTTHasCues(outputPath) {
		return fmt.Errorf("embedded stream %d: %w", streamIndex, errSubtitleNoCues)
	}

	return nil
}
