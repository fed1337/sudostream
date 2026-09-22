package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sudoStream/internal/observability"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestMetricsHandler_ExposesPrometheusMetrics(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET /metrics returns prometheus exposition format", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.Use(observability.HTTPMetrics())
		router.GET("/metrics", observability.MetricsHandler())
		router.GET("/api/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(
			context.Background(), http.MethodGet, "/api/health", nil,
		))
		seedMetrics()

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequestWithContext(
			context.Background(), http.MethodGet, "/metrics", nil,
		))
		if recorder.Code != http.StatusOK {
			t.Fatalf("metrics status: got %d want %d", recorder.Code, http.StatusOK)
		}
		body := recorder.Body.String()
		for _, metric := range requiredMetrics() {
			if !strings.Contains(body, metric) {
				t.Fatalf("expected %q in metrics body", metric)
			}
		}
		for _, gone := range removedMetrics() {
			if strings.Contains(body, gone) {
				t.Fatalf("did not expect removed metric %q", gone)
			}
		}
	})
}

func seedMetrics() {
	observability.RecordVideoPlayerOpen("series")
	observability.RecordPlaybackDecision("directPlay", "web")
	observability.IncPlaybackStream("directPlay", "web")
	observability.AddPlaybackStreamBytes("directPlay", "web", 1024)
	observability.DecPlaybackStream("directPlay", "web")
	observability.RecordMetadataProbe("success", time.Millisecond)
	observability.RecordMetadataIndexFile("movies")
	observability.RecordVideoPosterGeneration("success")
	observability.RecordTranscodeTimelineJob("success", time.Second)
	observability.RecordTranscodeSegmentJob("success", time.Second)
	observability.SetTranscodeJobsActive(1)
	observability.SetTranscodeSegmentSessionsActive(2)
	observability.RecordFFmpegError("encode")
	observability.RecordHLSCacheLookup("hit")
	observability.RecordHLSCacheLookup("miss")
	observability.SetHLSCacheStats(3, 4096)
	observability.RecordHLSSegmentRequest("hit")
	observability.RecordDLNASOAPRequest("Browse", "success")
	observability.RecordDLNAStreamRequest("success")
	observability.RecordAuthLoginAttempt("success")
	observability.SetAuthSessionsActive(2)
	observability.RecordProviderTaskItems("tmdb", "metadata", map[string]int{"applied": 1})
	observability.RecordProviderHTTP("tmdb", 200)
	observability.SetLibraryMediaFiles(map[string]int64{"movies": 10})
	observability.SetTrashItems(4)
}

func requiredMetrics() []string {
	return []string{
		"http_requests_total", "playback_decisions_total", "playback_streams_active",
		"playback_stream_bytes_total", "transcode_timeline_jobs_total", "transcode_segment_jobs_total",
		"hls_cache_lookups_total", "hls_cache_bytes", "dlna_soap_requests_total",
		"auth_login_attempts_total", "provider_task_items_total", "library_media_files", "trash_items",
	}
}

func removedMetrics() []string {
	return []string{
		"transcode_jobs_total", "transcode_duration_seconds", "metadata_reads_total",
		"media_delete_total", "oauth_token_requests_total", "watch_updates_total",
		"favorite_updates_total", "hls_cache_hits_total",
	}
}
