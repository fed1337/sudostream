package httpapi

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
	"sudoStream/internal/playback"
	"sudoStream/internal/skipsegment"
	"sudoStream/internal/transcode"

	"github.com/gin-gonic/gin"
)

const (
	hlsMasterPlaylist      = "master.m3u8"
	errTranscodeInProgress = "transcode in progress"
	errNotFound            = "not found"
	errPlaybackUnavailable = "playback metadata unavailable"
)

// PlaybackResponse is the JSON body for GET/POST /api/playback/{path}.
type PlaybackResponse struct {
	Status                 transcode.Status        `json:"status"`
	Error                  string                  `json:"error,omitempty"`
	DurationSeconds        float64                 `json:"durationSeconds,omitempty"`
	MasterURL              string                  `json:"masterUrl,omitempty"`
	StreamURL              string                  `json:"streamUrl,omitempty"`
	PlayMethod             playback.PlayMethod     `json:"playMethod,omitempty"`
	PackagingMode          transcode.PackagingMode `json:"packagingMode,omitempty"`
	Encoder                string                  `json:"encoder,omitempty"`
	ToneMapped             bool                    `json:"toneMapped,omitempty"`
	Qualities              []transcode.QualityInfo `json:"qualities,omitempty"`
	AudioTracks            []transcode.TrackInfo   `json:"audioTracks,omitempty"`
	SubtitleTracks         []transcode.TrackInfo   `json:"subtitleTracks,omitempty"`
	ProviderSubtitleTracks []ProviderSubtitleTrack `json:"providerSubtitleTracks,omitempty"`
	// UserSubtitle is the current user's uploaded caption when present (TTL 1d).
	UserSubtitle *UserSubtitleTrack `json:"userSubtitle,omitempty"`
	// Chapters are container chapter atoms on any video file (not limited to film/series).
	Chapters []transcode.ChapterInfo `json:"chapters,omitempty"`
	// SkipIntro is set when chapter metadata identifies an opening intro segment (E-23).
	SkipIntro *skipsegment.Intro `json:"skipIntro,omitempty"`
	// Series is set only for paths in a series library with resolvable S/E identity.
	Series *PlaybackSeriesContext `json:"series,omitempty"`
}

// PlaybackNegotiateRequest is the JSON body for POST /api/playback/{path}.
type PlaybackNegotiateRequest struct {
	DeviceProfile *playback.DeviceProfile `json:"deviceProfile,omitempty"`
	// QualityHeight is the selected ladder rung; 0 / omitted means original/source.
	QualityHeight int  `json:"qualityHeight,omitempty"`
	Transcode     bool `json:"transcode,omitempty"`
	Fallback      bool `json:"fallback,omitempty"`
}

// PlaybackSeriesContext identifies the playing episode for Part B season/episode chrome.
type PlaybackSeriesContext struct {
	LibrarySlug string `json:"librarySlug"`
	ShowKey     string `json:"showKey"`
	ShowName    string `json:"showName"`
	Season      int    `json:"season"`
	Episode     int    `json:"episode"`
}

// play handles HLS master playlist and segment requests.
//
//	@Summary		Get HLS playlist or segment
//	@Description	Retrieve HLS master playlist or media segments for video playback
//	@Tags			media
//	@Produce		application/vnd.apple.mpegurl
//	@Param			path	path	string	true	"Media file path with optional HLS resource suffix"
//	@Success		200
//	@Failure		400	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Failure		503	{object}	ErrorResponse
//	@Router			/api/play/{path} [get]
func (h *handler) play(c *gin.Context) {
	mediaPath, resource, ok := transcode.SplitPlayPath(c.Param("path"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{errorKey: invalidPathError})

		return
	}

	if !h.ensureReadAccess(c, mediaPath) {
		return
	}

	if h.transcode == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorKey: "HLS playback unavailable"})

		return
	}

	filePath, info, err := h.openVideoFile(mediaPath)
	if err != nil {
		h.handlePathError(c, err, "failed to open file", "file not found", "path is not a file")

		return
	}

	if resource == "" {
		resource = hlsMasterPlaylist
	}

	cacheKey := h.transcode.CacheKey(filePath, info.ModTime().Unix(), info.Size())
	h.serveHLSTranscode(c, mediaPath, resource, cacheKey, filePath)
}

func (h *handler) serveHLSTranscode(
	c *gin.Context,
	mediaPath, resource, cacheKey, filePath string,
) {
	if resource == hlsMasterPlaylist {
		err := h.transcode.StartJob(c.Request.Context(), cacheKey, filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{errorKey: "failed to start transcode"})

			return
		}
	}

	status := h.transcode.Status(cacheKey)
	if h.respondHLSTranscodeBlocked(c, mediaPath, resource, status) {
		return
	}

	if variant, index, ok := transcode.SplitSegmentResource(resource); ok {
		h.serveHLSSegment(c, mediaPath, resource, cacheKey, filePath, variant, index)

		return
	}

	diskPath, ok := h.resolveHLSTranscodeDiskPath(c, cacheKey, resource, status)
	if !ok {
		return
	}

	librarySlug, libraryType := h.libraryLabels(c, mediaPath)
	if resource == hlsMasterPlaylist {
		observability.RecordVideoPlayerOpen(libraryType)
	}

	c.Header("Content-Type", transcode.ContentTypeForResource(resource))

	if strings.HasSuffix(strings.ToLower(resource), ".m3u8") {
		h.serveHLSPlaylist(c, diskPath)
	} else {
		http.ServeFile(c.Writer, c.Request, diskPath)
	}

	observability.LogAction(
		c,
		"media.play",
		slog.String("path", mediaPath),
		slog.String("resource", resource),
		slog.String("library_slug", librarySlug),
	)
}

// serveHLSSegment produces the requested rendition segment on demand and streams it back.
func (h *handler) serveHLSSegment(
	c *gin.Context,
	mediaPath, resource, cacheKey, filePath string,
	variant transcode.Variant,
	index int,
) {
	segmentPath, err := h.transcode.ResolveSegment(
		c.Request.Context(),
		cacheKey,
		filePath,
		variant,
		index,
	)
	if err != nil {
		h.writeSegmentError(c, mediaPath, resource, err)

		return
	}

	c.Header("Content-Type", transcode.ContentTypeForResource(resource))
	c.Header("Cache-Control", "private, max-age=3600")
	http.ServeFile(c.Writer, c.Request, segmentPath)
}

func (h *handler) writeSegmentError(c *gin.Context, mediaPath, resource string, err error) {
	observability.LogAction(
		c,
		"media.play.segment.error",
		slog.String("path", mediaPath),
		slog.String("resource", resource),
		slog.String("error", err.Error()),
	)

	switch {
	case errors.Is(err, transcode.ErrSegmentOutOfRange),
		errors.Is(err, transcode.ErrInvalidVariant):
		c.JSON(http.StatusNotFound, gin.H{errorKey: errNotFound})
	case errors.Is(err, transcode.ErrSourceMetaUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{errorKey: errTranscodeInProgress})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: "segment generation failed"})
	}
}

func (h *handler) respondHLSTranscodeBlocked(
	c *gin.Context,
	mediaPath, resource string,
	status transcode.JobStatus,
) bool {
	if status.Status == transcode.StatusError {
		observability.LogAction(
			c,
			"media.play.error",
			slog.String("path", mediaPath),
			slog.String("resource", resource),
			slog.String("error", status.Error),
		)
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: status.Error})

		return true
	}

	if status.Status == transcode.StatusReady {
		return false
	}

	observability.LogAction(
		c,
		"media.play.pending",
		slog.String("path", mediaPath),
		slog.String("resource", resource),
		slog.String("status", string(status.Status)),
	)
	c.JSON(http.StatusServiceUnavailable, gin.H{errorKey: errTranscodeInProgress})

	return true
}

func (h *handler) resolveHLSTranscodeDiskPath(
	c *gin.Context,
	cacheKey, resource string,
	status transcode.JobStatus,
) (string, bool) {
	diskPath := h.transcode.ResourcePath(cacheKey, resource)
	if diskPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{errorKey: invalidPathError})

		return "", false
	}

	_, statErr := os.Stat(diskPath)
	if statErr != nil {
		h.writeTranscodeResourceMissing(c, status)

		return "", false
	}

	return diskPath, true
}

func queryFlagTrue(c *gin.Context, key string) bool {
	return strings.EqualFold(c.Query(key), "1") || strings.EqualFold(c.Query(key), "true")
}

func (h *handler) writeTranscodeResourceMissing(c *gin.Context, status transcode.JobStatus) {
	if status.Status == transcode.StatusProcessing || status.Status == transcode.StatusIdle {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorKey: errTranscodeInProgress})

		return
	}

	c.JSON(http.StatusNotFound, gin.H{errorKey: "playlist not found"})
}

func (h *handler) serveHLSPlaylist(c *gin.Context, diskPath string) {
	raw, err := os.ReadFile(diskPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errorKey: "failed to read playlist"})

		return
	}

	c.Header("Cache-Control", transcode.PlaylistCacheControl(raw))

	body := transcode.NormalizePlaylistForClient(raw)
	body = transcode.RewritePlaylistTokens(body, accessToken(c))
	_, err = c.Writer.Write(body)
	if err != nil {
		observability.LogAction(c, "media.play.error", slog.String("error", err.Error()))
	}
}

// playback returns transcode status and track metadata (HLS job poll / legacy GET).
//
//	@Summary		Get playback status and metadata
//	@Description	Retrieve job status, qualities, and tracks. Prefer POST + deviceProfile for FI-8.
//	@Tags			media
//	@Produce		json
//	@Param			path		path		string	true	"Media file path"
//	@Param			transcode	query		bool	false	"Start HLS transcode job"
//	@Param			fallback	query		bool	false	"Start transcode after direct playback failed"
//	@Success		200			{object}	PlaybackResponse
//	@Failure		400			{object}	ErrorResponse
//	@Failure		403			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/api/playback/{path} [get]
func (h *handler) playback(c *gin.Context) {
	h.respondPlayback(c, PlaybackNegotiateRequest{
		Transcode: queryFlagTrue(c, "transcode"),
		Fallback:  queryFlagTrue(c, "fallback"),
	})
}

// playbackNegotiate decides Direct Play vs HLS from a device profile (FI-8).
//
//	@Summary		Negotiate playback method
//	@Description	Accepts a device profile; returns playMethod (directPlay/remux/transcode) + URLs.
//	@Tags			media
//	@Accept			json
//	@Produce		json
//	@Param			path	path		string						true	"Media file path"
//	@Param			body	body		PlaybackNegotiateRequest	false	"Device profile and quality selection"
//	@Success		200		{object}	PlaybackResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/playback/{path} [post]
func (h *handler) playbackNegotiate(c *gin.Context) {
	var req PlaybackNegotiateRequest
	if c.Request.ContentLength > 0 {
		err := c.ShouldBindJSON(&req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errorKey: "invalid playback request"})

			return
		}
	}

	h.respondPlayback(c, req)
}

func (h *handler) respondPlayback(c *gin.Context, req PlaybackNegotiateRequest) {
	mediaPath := strings.TrimPrefix(c.Param("path"), "/")
	if !h.ensureReadAccess(c, mediaPath) {
		return
	}

	if h.transcode == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorKey: errPlaybackUnavailable})

		return
	}

	filePath, info, err := h.openVideoFile(mediaPath)
	if err != nil {
		h.handlePathError(c, err, "failed to open file", "file not found", "path is not a file")

		return
	}

	cacheKey := h.transcode.CacheKey(filePath, info.ModTime().Unix(), info.Size())
	outDir := h.transcode.CacheOutDir(cacheKey)
	escaped := mediafs.EscapePathSegments(mediaPath)
	masterURL := "/api/play/" + escaped + "/" + hlsMasterPlaylist
	streamURL := "/api/stream/" + escaped

	decision, caps, capsOK := h.decidePlaybackMethod(c, req, filePath, mediaPath, outDir)
	if capsOK && decision.Method == playback.MethodDirectPlay {
		h.respondDirectPlay(c, mediaPath, filePath, streamURL, caps)

		return
	}

	if req.Transcode || req.Fallback {
		err = h.transcode.StartJob(c.Request.Context(), cacheKey, filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{errorKey: "failed to start transcode"})

			return
		}
	}

	h.respondHLSPlayback(c, mediaPath, filePath, cacheKey, outDir, masterURL, decision, capsOK)
}

func (h *handler) decidePlaybackMethod(
	c *gin.Context,
	req PlaybackNegotiateRequest,
	filePath, mediaPath, outDir string,
) (playback.Decision, playback.SourceCaps, bool) {
	decision := playback.Decision{Method: playback.MethodTranscode}

	if req.DeviceProfile != nil {
		caps, capsErr := h.sourceCapsForPlayback(c, filePath, mediaPath, outDir)
		if capsErr != nil {
			return decision, playback.SourceCaps{}, false
		}

		return playback.Decide(*req.DeviceProfile, caps, req.QualityHeight), caps, true
	}

	if meta, ok := transcode.ReadSourceMeta(outDir); ok &&
		meta.PackagingMode == transcode.PackagingRemux {
		decision = playback.Decision{Method: playback.MethodRemux}
	}

	return decision, playback.SourceCaps{}, false
}

func (h *handler) respondHLSPlayback(
	c *gin.Context,
	mediaPath, filePath, cacheKey, outDir, masterURL string,
	decision playback.Decision,
	capsOK bool,
) {
	jobStatus := h.transcode.Status(cacheKey)
	playbackInfo := transcode.BuildPlaybackInfoWithSettings(
		outDir, masterURL, jobStatus, h.transcode.CurrentSettings(),
	)

	playMethod := resolvePlayMethod(decision, playbackInfo.PackagingMode, capsOK)
	chapters := playbackInfo.Chapters
	if jobStatus.Status == transcode.StatusReady && chapters == nil {
		probed, probeErr := transcode.ProbeChapters(c.Request.Context(), filePath)
		if probeErr == nil {
			chapters = probed
		}
	}

	librarySlug, _ := h.libraryLabels(c, mediaPath)
	observability.RecordPlaybackDecision(string(playMethod), "web")
	observability.LogAction(
		c,
		"media.playback",
		slog.String("path", mediaPath),
		slog.String("status", string(playbackInfo.Status)),
		slog.String("play_method", string(playMethod)),
		slog.String("library_slug", librarySlug),
	)

	c.JSON(http.StatusOK, PlaybackResponse{
		Status:                 playbackInfo.Status,
		Error:                  playbackInfo.Error,
		DurationSeconds:        playbackInfo.DurationSeconds,
		MasterURL:              playbackInfo.MasterURL,
		PlayMethod:             playMethod,
		PackagingMode:          playbackInfo.PackagingMode,
		Encoder:                playbackInfo.Encoder,
		ToneMapped:             playbackInfo.ToneMapped,
		Qualities:              playbackInfo.Qualities,
		AudioTracks:            playbackInfo.AudioTracks,
		SubtitleTracks:         playbackInfo.SubtitleTracks,
		ProviderSubtitleTracks: h.providerSubtitleTracks(c.Request.Context(), mediaPath),
		UserSubtitle:           h.userSubtitleTrack(c, mediaPath),
		Chapters:               chapters,
		SkipIntro:              skipIntroFromChapters(chapters, playbackInfo.DurationSeconds),
		Series:                 h.playbackSeriesContext(c, mediaPath),
	})
}

func resolvePlayMethod(
	decision playback.Decision,
	packaging transcode.PackagingMode,
	capsOK bool,
) playback.PlayMethod {
	if capsOK {
		return decision.Method
	}

	switch packaging {
	case transcode.PackagingRemux:
		return playback.MethodRemux
	case transcode.PackagingTranscode:
		return playback.MethodTranscode
	default:
		return decision.Method
	}
}

func (h *handler) respondDirectPlay(
	c *gin.Context,
	mediaPath, filePath, streamURL string,
	caps playback.SourceCaps,
) {
	var chapters []transcode.ChapterInfo
	probed, probeErr := transcode.ProbeChapters(c.Request.Context(), filePath)
	if probeErr == nil {
		chapters = probed
	}

	librarySlug, _ := h.libraryLabels(c, mediaPath)
	observability.RecordPlaybackDecision(string(playback.MethodDirectPlay), "web")
	observability.LogAction(
		c,
		"media.playback",
		slog.String("path", mediaPath),
		slog.String("status", string(transcode.StatusReady)),
		slog.String("play_method", string(playback.MethodDirectPlay)),
		slog.String("library_slug", librarySlug),
	)

	qualities := []transcode.QualityInfo{}
	if caps.Height > 0 {
		// Full ladder so the client can leave Direct Play for a lower rung.
		qualities = (transcode.SourceMeta{Height: caps.Height}).Qualities()
	}

	c.JSON(http.StatusOK, PlaybackResponse{
		Status:                 transcode.StatusReady,
		DurationSeconds:        caps.DurationSeconds,
		StreamURL:              streamURL,
		PlayMethod:             playback.MethodDirectPlay,
		Qualities:              qualities,
		ProviderSubtitleTracks: h.providerSubtitleTracks(c.Request.Context(), mediaPath),
		UserSubtitle:           h.userSubtitleTrack(c, mediaPath),
		Chapters:               chapters,
		SkipIntro:              skipIntroFromChapters(chapters, caps.DurationSeconds),
		Series:                 h.playbackSeriesContext(c, mediaPath),
	})
}

func skipIntroFromChapters(chapters []transcode.ChapterInfo, durationSeconds float64) *skipsegment.Intro {
	if len(chapters) == 0 || durationSeconds <= 0 {
		return nil
	}

	cues := make([]skipsegment.ChapterCue, len(chapters))
	for index, chapter := range chapters {
		cues[index] = skipsegment.ChapterCue{
			StartSeconds: chapter.StartSeconds,
			EndSeconds:   chapter.EndSeconds,
			Title:        chapter.Title,
		}
	}

	return skipsegment.IntroFromChapters(cues, durationSeconds)
}

func (h *handler) sourceCapsForPlayback(
	c *gin.Context,
	filePath, mediaPath, outDir string,
) (playback.SourceCaps, error) {
	container := playback.ContainerFromPath(mediaPath)

	if meta, ok := transcode.ReadSourceMeta(outDir); ok {
		audioCodec := ""
		if len(meta.AudioStreams) > 0 {
			audioCodec = meta.AudioStreams[0].Codec
		}

		return playback.SourceCaps{
			Container:       container,
			VideoCodec:      meta.VideoCodec,
			AudioCodec:      audioCodec,
			Height:          meta.Height,
			DurationSeconds: meta.DurationSeconds,
			RemuxOK:         meta.PackagingMode == transcode.PackagingRemux,
		}, nil
	}

	source, err := transcode.ProbeSource(c.Request.Context(), filePath)
	if err != nil {
		return playback.SourceCaps{}, fmt.Errorf("probe source for playback: %w", err)
	}

	return playback.SourceCaps{
		Container:       container,
		VideoCodec:      source.VideoCodec,
		AudioCodec:      source.AudioCodec,
		Bitrate:         source.Bitrate,
		Height:          source.Height,
		DurationSeconds: source.DurationSeconds,
		RemuxOK:         transcode.RemuxEligible(source),
	}, nil
}

func (h *handler) playbackSeriesContext(c *gin.Context, mediaPath string) *PlaybackSeriesContext {
	if h.access == nil || h.catalog == nil {
		return nil
	}

	library, ok, err := h.access.LibraryForRelPath(c.Request.Context(), mediaPath)
	if err != nil || !ok || library.Type != access.LibraryTypeSeries {
		return nil
	}

	identity, resolved := h.catalog.ResolveSeriesEpisode(
		c.Request.Context(),
		library.Type,
		mediaPath,
	)
	if !resolved {
		return nil
	}

	slug := library.Slug
	if slug == "" {
		slug = library.RelPath
	}

	return &PlaybackSeriesContext{
		LibrarySlug: slug,
		ShowKey:     identity.ShowKey,
		ShowName:    identity.ShowName,
		Season:      identity.Season,
		Episode:     identity.Episode,
	}
}

// deleteMedia moves a file or directory into the recycle bin.
//
//	@Summary		Delete media file or folder
//	@Description	Moves a media path into /media/.trash; directories are always recursive
//	@Tags			media
//	@Param			path		path	string	true	"Path under media root"
//	@Param			recursive	query	bool	false	"Ignored for recycle-bin deletes; directories always move as a tree"
//	@Success		204
//	@Failure		400	{object}	ErrorResponse
//	@Failure		403	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		409	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/media/{path} [delete]
func (h *handler) deleteMedia(c *gin.Context) {
	pathParam := c.Param("path")
	if !h.ensureDeleteAccess(c, pathParam) {
		return
	}

	recursive := strings.EqualFold(c.Query("recursive"), "1") ||
		strings.EqualFold(c.Query("recursive"), "true")

	filePath, info, statErr := h.statMediaPath(pathParam)
	if statErr != nil {
		h.handlePathError(
			c,
			statErr,
			"failed to stat media",
			"file not found",
			"path is not a file",
		)

		return
	}

	if h.trash != nil {
		h.moveMediaToTrash(c, pathParam)

		return
	}

	err := h.media.Delete(pathParam, recursive)
	if err != nil {
		if errors.Is(err, mediafs.ErrDeleteNotEmpty) {
			observability.LogAction(c, "media.delete",
				slog.String("path", pathParam),
				slog.String("outcome", "not_empty"),
			)
			c.JSON(http.StatusConflict, gin.H{errorKey: "directory is not empty"})

			return
		}

		if errors.Is(err, mediafs.ErrDeleteReadOnly) {
			folder := firstMediaPathSegment(pathParam)
			if folder == "" {
				folder = "media"
			}

			observability.LogAction(c, "media.delete",
				slog.String("path", pathParam),
				slog.String("outcome", "read_only"),
			)
			c.JSON(
				http.StatusForbidden,
				gin.H{errorKey: fmt.Sprintf("no write access to the %s folder", folder)},
			)

			return
		}

		observability.LogAction(c, "media.delete",
			slog.String("path", pathParam),
			slog.String("outcome", "error"),
			slog.String("error", err.Error()),
		)
		h.handlePathError(c, err, "delete failed", "file not found", invalidPathError)

		return
	}

	h.cleanupDeletedMedia(c, pathParam, filePath, info)
	observability.LogAction(c, "media.delete",
		slog.String("path", pathParam),
		slog.String("outcome", "success"),
	)

	c.Status(http.StatusNoContent)
}

func (h *handler) moveMediaToTrash(c *gin.Context, pathParam string) {
	userID := ""
	if user, ok := currentUser(c); ok {
		userID = user.ID
	}

	_, err := h.trash.Move(c.Request.Context(), pathParam, userID)
	if err != nil {
		if errors.Is(err, mediafs.ErrTrashForbidden) {
			observability.LogAction(c, "media.trash",
				slog.String("path", pathParam),
				slog.String("outcome", "forbidden"),
				slog.String("deleted_by", userID),
			)
			c.JSON(http.StatusForbidden, gin.H{errorKey: "cannot delete trash or media root"})

			return
		}
		if errors.Is(err, mediafs.ErrDeleteReadOnly) {
			folder := firstMediaPathSegment(pathParam)
			if folder == "" {
				folder = "media"
			}
			observability.LogAction(c, "media.trash",
				slog.String("path", pathParam),
				slog.String("outcome", "read_only"),
				slog.String("deleted_by", userID),
			)
			c.JSON(
				http.StatusForbidden,
				gin.H{errorKey: fmt.Sprintf("no write access to the %s folder", folder)},
			)

			return
		}

		observability.LogAction(c, "media.trash",
			slog.String("path", pathParam),
			slog.String("outcome", "error"),
			slog.String("deleted_by", userID),
			slog.String("error", err.Error()),
		)
		h.handlePathError(c, err, "delete failed", "file not found", invalidPathError)

		return
	}

	observability.LogAction(c, "media.trash",
		slog.String("path", pathParam),
		slog.String("outcome", "success"),
		slog.String("deleted_by", userID),
	)
	c.Status(http.StatusNoContent)
}

func (h *handler) openVideoFile(pathParam string) (string, os.FileInfo, error) {
	filePath, err := h.media.FilePath(pathParam)
	if err != nil {
		return "", nil, fmt.Errorf("resolve video path: %w", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("stat video path: %w", err)
	}

	if !isVideoMediaFile(filePath) {
		return "", nil, fs.ErrInvalid
	}

	return filePath, info, nil
}

func isVideoMediaFile(filePath string) bool {
	ext := filepath.Ext(filePath)
	if mediafs.IsVideoExtension(ext) {
		return true
	}

	mimeType := mediafs.DetectMimeType(filePath, ext, false)

	return strings.HasPrefix(mimeType, "video/")
}

func (h *handler) statMediaPath(pathParam string) (string, os.FileInfo, error) {
	filePath, err := h.media.FilePath(pathParam)
	if err == nil {
		info, statErr := os.Stat(filePath)
		if statErr != nil {
			return "", nil, fmt.Errorf("stat media file: %w", statErr)
		}

		return filePath, info, nil
	}

	dirPath, dirErr := h.media.DirPath(pathParam)
	if dirErr != nil {
		return "", nil, fmt.Errorf("resolve media path: %w", err)
	}

	info, statErr := os.Stat(dirPath)
	if statErr != nil {
		return "", nil, fmt.Errorf("stat media directory: %w", statErr)
	}

	return dirPath, info, nil
}

//nolint:cyclop // sequential best-effort cleanup of metadata/thumbs/hls/watch/favorites
func (h *handler) cleanupDeletedMedia(
	c *gin.Context,
	pathParam, filePath string,
	info os.FileInfo,
) {
	if h.metadata != nil {
		err := h.metadata.DeleteForPath(c.Request.Context(), pathParam)
		if err != nil {
			observability.LogAction(
				c,
				"media.delete.cleanup",
				slog.String("path", pathParam),
				slog.String("target", "metadata"),
				slog.String("error", err.Error()),
			)
		}
	}

	if h.videoThumb != nil && info != nil && !info.IsDir() {
		h.videoThumb.Remove(pathParam, info.ModTime().Unix(), info.Size())
	}

	if h.transcode != nil && info != nil && !info.IsDir() {
		cacheKey := h.transcode.CacheKey(filePath, info.ModTime().Unix(), info.Size())
		h.transcode.RemoveCache(cacheKey)
	}

	if h.watch != nil {
		h.cleanupWatchState(c, pathParam)
	}
	if h.favorite != nil {
		h.cleanupFavoriteState(c, pathParam)
	}
}

func (h *handler) cleanupWatchState(c *gin.Context, pathParam string) {
	err := h.watch.DeleteForPath(c.Request.Context(), pathParam)
	if err != nil {
		observability.LogAction(
			c,
			"media.delete.cleanup",
			slog.String("path", pathParam),
			slog.String("target", "watch"),
			slog.String("error", err.Error()),
		)
	}
}

func (h *handler) cleanupFavoriteState(c *gin.Context, pathParam string) {
	err := h.favorite.DeleteForPath(c.Request.Context(), pathParam)
	if err != nil {
		observability.LogAction(
			c,
			"media.delete.cleanup",
			slog.String("path", pathParam),
			slog.String("target", "favorite"),
			slog.String("error", err.Error()),
		)
	}
}

func (h *handler) ensureDeleteAccess(c *gin.Context, rawPath string) bool {
	return h.ensurePermission(c, rawPath, h.access.CanDelete)
}

const defaultCacheRoot = "/var/lib/sudostream/cache"

// testCacheRoot overrides the cache root for tests (empty = production default).
var testCacheRoot string

func setTestCacheRoot(root string) func() {
	previous := testCacheRoot
	testCacheRoot = root

	return func() {
		testCacheRoot = previous
	}
}

func cacheSubdir(subdir string) string {
	root := defaultCacheRoot
	if testCacheRoot != "" {
		root = testCacheRoot
	}

	return filepath.Join(root, subdir)
}
