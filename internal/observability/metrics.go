// Package observability provides HTTP metrics and structured request logging.
package observability

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	metricLabelMethod      = "method"
	metricLabelRoute       = "route"
	metricLabelStatus      = "status"
	metricLabelLibrarySlug = "library_slug"
	metricLabelAction      = "action"
	metricLabelTrigger     = "trigger"
	metricLabelClient      = "client"
	metricLabelResult      = "result"
	metricLabelProvider    = "provider"
	metricLabelKind        = "kind"
	metricLabelStatusClass = "status_class"
)

//nolint:gochecknoglobals // Prometheus metrics are registered process-wide.
var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{metricLabelMethod, metricLabelRoute, metricLabelStatus},
	)
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{metricLabelMethod, metricLabelRoute},
	)
	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed.",
		},
	)

	videoPlayerOpensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "video_player_opens_total",
			Help: "Total number of video stream requests treated as player opens.",
		},
		[]string{"library_type"},
	)

	playbackDecisionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "playback_decisions_total",
			Help: "Playback method decisions (directPlay / remux / transcode) by client.",
		},
		[]string{metricLabelMethod, metricLabelClient},
	)
	playbackStreamsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "playback_streams_active",
			Help: "Number of progressive media streams currently being served.",
		},
		[]string{metricLabelMethod, metricLabelClient},
	)
	playbackStreamBytesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "playback_stream_bytes_total",
			Help: "Bytes written for progressive media streams.",
		},
		[]string{metricLabelMethod, metricLabelClient},
	)

	metadataProbeTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "metadata_probe_total",
			Help: "Total ffprobe executions during metadata indexing.",
		},
		[]string{metricLabelStatus},
	)
	metadataProbeDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name:    "metadata_probe_duration_seconds",
			Help:    "ffprobe latency during metadata indexing.",
			Buckets: prometheus.DefBuckets,
		},
	)
	metadataIndexFilesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "metadata_index_files_total",
			Help: "Total video files indexed into metadata cache.",
		},
		[]string{metricLabelLibrarySlug},
	)

	videoPosterGenerationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "video_poster_generations_total",
			Help: "Total video poster thumbnail generations.",
		},
		[]string{metricLabelStatus},
	)

	transcodeJobsActive = promauto.NewGauge(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "transcode_jobs_active",
			Help: "Number of active HLS timeline probe/publish jobs.",
		},
	)
	transcodeSegmentSessionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "transcode_segment_sessions_active",
			Help: "Number of active ffmpeg HLS segment encode sessions.",
		},
	)
	transcodeTimelineJobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "transcode_timeline_jobs_total",
			Help: "Finished HLS timeline probe/publish jobs by outcome.",
		},
		[]string{metricLabelStatus},
	)
	transcodeSegmentJobsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "transcode_segment_jobs_total",
			Help: "Finished ffmpeg HLS segment encode sessions by outcome.",
		},
		[]string{metricLabelStatus},
	)
	transcodeTimelineDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name:    "transcode_timeline_duration_seconds",
			Help:    "HLS timeline probe/publish job duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)
	transcodeSegmentDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name:    "transcode_segment_duration_seconds",
			Help:    "ffmpeg HLS segment encode session duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)

	ffmpegErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "ffmpeg_errors_total",
			Help: "Total ffmpeg failures by stage.",
		},
		[]string{"stage"},
	)

	hlsCacheLookupsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "hls_cache_lookups_total",
			Help: "HLS timeline cache lookups by result (hit or miss).",
		},
		[]string{metricLabelResult},
	)
	hlsCacheBytes = promauto.NewGauge(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "hls_cache_bytes",
			Help: "Total bytes used by the on-disk HLS cache.",
		},
	)
	hlsCacheEntries = promauto.NewGauge(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "hls_cache_entries",
			Help: "Number of top-level HLS cache packages (cache keys).",
		},
	)
	hlsSegmentRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "hls_segment_requests_total",
			Help: "Total HLS segment resolutions by how they were served.",
		},
		[]string{metricLabelResult},
	)

	maintenanceRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "maintenance_runs_total",
			Help: "Total maintenance action runs by action, status, and trigger.",
		},
		[]string{metricLabelAction, metricLabelStatus, metricLabelTrigger},
	)
	maintenanceRunDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name:    "maintenance_run_duration_seconds",
			Help:    "Maintenance action run duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{metricLabelAction},
	)
	maintenanceRunning = promauto.NewGaugeVec(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "maintenance_running",
			Help: "Whether a maintenance action is currently running (0 or 1).",
		},
		[]string{metricLabelAction},
	)

	dlnaSOAPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "dlna_soap_requests_total",
			Help: "DLNA SOAP control requests by action and outcome.",
		},
		[]string{metricLabelAction, metricLabelStatus},
	)
	dlnaStreamRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "dlna_stream_requests_total",
			Help: "DLNA signed progressive stream requests by outcome.",
		},
		[]string{metricLabelStatus},
	)

	authLoginAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "auth_login_attempts_total",
			Help: "Login attempts by result (success, invalid_credentials, disabled, …).",
		},
		[]string{metricLabelResult},
	)
	aclPathCanonicalizedTotal = promauto.NewCounter(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "acl_path_canonicalized_total",
			Help: "ACL checks where the request path was rewritten (unescape/clean) before grant matching.",
		},
	)
	authSessionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "auth_sessions_active",
			Help: "Number of refresh sessions currently stored.",
		},
	)

	providerTaskItemsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "provider_task_items_total",
			Help: "Provider enrichment item outcomes by provider, kind, and result.",
		},
		[]string{metricLabelProvider, metricLabelKind, metricLabelResult},
	)
	providerHTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "provider_http_requests_total",
			Help: "Outbound provider HTTP requests by provider and status class.",
		},
		[]string{metricLabelProvider, metricLabelStatusClass},
	)

	libraryMediaFiles = promauto.NewGaugeVec(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "library_media_files",
			Help: "Indexed media files per library slug.",
		},
		[]string{metricLabelLibrarySlug},
	)
	trashItems = promauto.NewGauge(
		prometheus.GaugeOpts{ //nolint:exhaustruct // optional Prometheus fields omitted
			Name: "trash_items",
			Help: "Number of items currently in the recycle bin.",
		},
	)
)

// InventoryFuncs supplies optional scrape-time gauge refreshers.
type InventoryFuncs struct {
	HLSCacheStats func() (entries int, bytes int64)
	LibraryFiles  func() map[string]int64
	TrashItems    func() int64
	AuthSessions  func() int64
}

//nolint:gochecknoglobals // process-wide inventory hooks
var (
	inventoryMu    sync.RWMutex
	inventoryFuncs InventoryFuncs
)

// RegisterInventoryFuncs wires optional scrapers for HLS/library/trash/session gauges.
func RegisterInventoryFuncs(funcs InventoryFuncs) {
	inventoryMu.Lock()
	inventoryFuncs = funcs
	inventoryMu.Unlock()
}

func refreshInventoryMetrics() {
	inventoryMu.RLock()
	funcs := inventoryFuncs
	inventoryMu.RUnlock()

	if funcs.HLSCacheStats != nil {
		entries, bytes := funcs.HLSCacheStats()
		SetHLSCacheStats(entries, bytes)
	}
	if funcs.LibraryFiles != nil {
		SetLibraryMediaFiles(funcs.LibraryFiles())
	}
	if funcs.TrashItems != nil {
		trashItems.Set(float64(funcs.TrashItems()))
	}
	if funcs.AuthSessions != nil {
		authSessionsActive.Set(float64(funcs.AuthSessions()))
	}
}

// MetricsHandler serves Prometheus metrics at GET /metrics.
//
//	@Summary		Prometheus metrics
//	@Description	Returns Prometheus exposition format metrics.
//	@Tags			system
//	@Produce		plain
//	@Success		200	{string}	string
//	@Router			/metrics [get]
func MetricsHandler() gin.HandlerFunc {
	handler := promhttp.Handler()

	return func(c *gin.Context) {
		UpdateDBPoolMetrics()
		refreshInventoryMetrics()
		handler.ServeHTTP(c.Writer, c.Request)
	}
}

// HTTPMetrics records request counts, latency, and in-flight gauge.
func HTTPMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		httpRequestsInFlight.Inc()
		start := time.Now()

		c.Next()

		httpRequestsInFlight.Dec()

		route := c.FullPath()
		if route == "" {
			route = unknownRouteLabel
		}

		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		httpRequestsTotal.WithLabelValues(method, route, status).Inc()
		httpRequestDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

func labelOrUnknown(value string) string {
	if value == "" {
		return unknownRouteLabel
	}

	return value
}

// RecordVideoPlayerOpen increments player open counter for a library type label.
func RecordVideoPlayerOpen(libraryType string) {
	videoPlayerOpensTotal.WithLabelValues(labelOrUnknown(libraryType)).Inc()
}

// RecordPlaybackDecision counts a Direct Play / remux / HLS decision.
func RecordPlaybackDecision(method, client string) {
	playbackDecisionsTotal.WithLabelValues(labelOrUnknown(method), labelOrUnknown(client)).Inc()
}

// IncPlaybackStream marks a progressive stream as active.
func IncPlaybackStream(method, client string) {
	playbackStreamsActive.WithLabelValues(labelOrUnknown(method), labelOrUnknown(client)).Inc()
}

// DecPlaybackStream marks a progressive stream as finished.
func DecPlaybackStream(method, client string) {
	playbackStreamsActive.WithLabelValues(labelOrUnknown(method), labelOrUnknown(client)).Dec()
}

// AddPlaybackStreamBytes adds bytes served for a progressive stream.
func AddPlaybackStreamBytes(method, client string, n int64) {
	if n <= 0 {
		return
	}

	playbackStreamBytesTotal.WithLabelValues(labelOrUnknown(method), labelOrUnknown(client)).Add(float64(n))
}

// RecordMetadataProbe records probe count and duration.
func RecordMetadataProbe(status string, duration time.Duration) {
	metadataProbeTotal.WithLabelValues(labelOrUnknown(status)).Inc()
	metadataProbeDuration.Observe(duration.Seconds())
}

// RecordMetadataIndexFile increments indexed file counter for a library slug.
func RecordMetadataIndexFile(librarySlug string) {
	metadataIndexFilesTotal.WithLabelValues(labelOrUnknown(librarySlug)).Inc()
}

// RecordVideoPosterGeneration increments poster generation counter.
func RecordVideoPosterGeneration(status string) {
	videoPosterGenerationsTotal.WithLabelValues(labelOrUnknown(status)).Inc()
}

// RecordTranscodeTimelineJob records a finished timeline probe/publish job.
func RecordTranscodeTimelineJob(status string, duration time.Duration) {
	transcodeTimelineJobsTotal.WithLabelValues(labelOrUnknown(status)).Inc()
	transcodeTimelineDurationSeconds.Observe(duration.Seconds())
}

// RecordTranscodeSegmentJob records a finished segment encode session.
func RecordTranscodeSegmentJob(status string, duration time.Duration) {
	transcodeSegmentJobsTotal.WithLabelValues(labelOrUnknown(status)).Inc()
	transcodeSegmentDurationSeconds.Observe(duration.Seconds())
}

// SetTranscodeJobsActive sets how many HLS timeline probe/publish jobs are in flight.
func SetTranscodeJobsActive(n int) {
	if n < 0 {
		n = 0
	}

	transcodeJobsActive.Set(float64(n))
}

// SetTranscodeSegmentSessionsActive sets how many ffmpeg segment encode sessions are in flight.
func SetTranscodeSegmentSessionsActive(n int) {
	if n < 0 {
		n = 0
	}

	transcodeSegmentSessionsActive.Set(float64(n))
}

// RecordFFmpegError increments ffmpeg error counter.
func RecordFFmpegError(stage string) {
	ffmpegErrorsTotal.WithLabelValues(labelOrUnknown(stage)).Inc()
}

// RecordHLSCacheLookup counts a timeline cache hit or miss.
func RecordHLSCacheLookup(result string) {
	hlsCacheLookupsTotal.WithLabelValues(labelOrUnknown(result)).Inc()
}

// SetHLSCacheStats updates on-disk HLS cache gauges.
func SetHLSCacheStats(entries int, bytes int64) {
	if entries < 0 {
		entries = 0
	}
	if bytes < 0 {
		bytes = 0
	}

	hlsCacheEntries.Set(float64(entries))
	hlsCacheBytes.Set(float64(bytes))
}

// RecordHLSSegmentRequest counts a segment resolution as hit, wait or restart.
func RecordHLSSegmentRequest(result string) {
	hlsSegmentRequestsTotal.WithLabelValues(labelOrUnknown(result)).Inc()
}

// RecordMaintenanceRun records a finished maintenance run.
func RecordMaintenanceRun(action, status, trigger string, duration time.Duration) {
	maintenanceRunsTotal.WithLabelValues(
		labelOrUnknown(action),
		labelOrUnknown(status),
		labelOrUnknown(trigger),
	).Inc()
	maintenanceRunDuration.WithLabelValues(labelOrUnknown(action)).Observe(duration.Seconds())
}

// SetMaintenanceRunning sets the in-progress gauge for an action.
func SetMaintenanceRunning(action string, running bool) {
	value := 0.0
	if running {
		value = 1
	}

	maintenanceRunning.WithLabelValues(labelOrUnknown(action)).Set(value)
}

// RecordDLNASOAPRequest counts a DLNA SOAP control call.
func RecordDLNASOAPRequest(action, status string) {
	dlnaSOAPRequestsTotal.WithLabelValues(labelOrUnknown(action), labelOrUnknown(status)).Inc()
}

// RecordDLNAStreamRequest counts a DLNA progressive stream attempt.
func RecordDLNAStreamRequest(status string) {
	dlnaStreamRequestsTotal.WithLabelValues(labelOrUnknown(status)).Inc()
}

// RecordAuthLoginAttempt counts a login attempt outcome.
func RecordAuthLoginAttempt(result string) {
	authLoginAttemptsTotal.WithLabelValues(labelOrUnknown(result)).Inc()
}

// ObserveACLPathCanonicalized increments when ACL rewrote a request path before matching grants.
func ObserveACLPathCanonicalized() {
	aclPathCanonicalizedTotal.Inc()
}

// SetAuthSessionsActive sets how many refresh sessions exist.
func SetAuthSessionsActive(n int64) {
	if n < 0 {
		n = 0
	}

	authSessionsActive.Set(float64(n))
}

// RecordProviderTaskItems adds provider enrichment outcome counts for one run.
func RecordProviderTaskItems(provider, kind string, results map[string]int) {
	provider = labelOrUnknown(provider)
	kind = labelOrUnknown(kind)
	for result, count := range results {
		if count <= 0 {
			continue
		}
		providerTaskItemsTotal.WithLabelValues(provider, kind, labelOrUnknown(result)).Add(float64(count))
	}
}

// RecordProviderHTTP counts an outbound provider HTTP response by status class (2xx/3xx/4xx/5xx).
func RecordProviderHTTP(provider string, statusCode int) {
	class := "unknown"
	switch {
	case statusCode >= 200 && statusCode < 300:
		class = "2xx"
	case statusCode >= 300 && statusCode < 400:
		class = "3xx"
	case statusCode >= 400 && statusCode < 500:
		class = "4xx"
	case statusCode >= 500 && statusCode < 600:
		class = "5xx"
	}

	providerHTTPRequestsTotal.WithLabelValues(labelOrUnknown(provider), class).Inc()
}

// SetLibraryMediaFiles replaces per-library indexed file gauges.
func SetLibraryMediaFiles(counts map[string]int64) {
	libraryMediaFiles.Reset()
	for slug, n := range counts {
		if n < 0 {
			n = 0
		}
		libraryMediaFiles.WithLabelValues(labelOrUnknown(slug)).Set(float64(n))
	}
}

// SetTrashItems sets recycle-bin depth.
func SetTrashItems(n int64) {
	if n < 0 {
		n = 0
	}

	trashItems.Set(float64(n))
}

// CountingResponseWriter wraps a gin response writer and tracks bytes written.
type CountingResponseWriter struct {
	gin.ResponseWriter

	bytesWritten int64
}

// Write implements io.Writer and counts payload bytes.
func (w *CountingResponseWriter) Write(payload []byte) (int, error) {
	written, err := w.ResponseWriter.Write(payload)
	if err != nil {
		return written, fmt.Errorf("write response: %w", err)
	}
	w.bytesWritten += int64(written)

	return written, nil
}

// BytesWritten returns the total number of payload bytes written.
func (w *CountingResponseWriter) BytesWritten() int64 {
	return w.bytesWritten
}
