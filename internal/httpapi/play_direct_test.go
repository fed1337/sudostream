package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sudoStream/internal/playback"
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestStreamHandler_ServesInlineRange(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET /api/stream serves video inline with Accept-Ranges", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, "movies/direct.mp4")
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/stream/"+fixture.rel,
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: "/" + fixture.rel}}

		fixture.handler().stream(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Header().Get("Content-Disposition"), "inline") {
			t.Fatalf("expected inline disposition, got %q", recorder.Header().Get("Content-Disposition"))
		}
		if recorder.Body.String() != "fake-video" {
			t.Fatalf("body: got %q", recorder.Body.String())
		}
	})
}

func TestPlaybackNegotiate_DirectPlayWhenProfileMatches(t *testing.T) {
	t.Parallel()

	allure.Test(t, "POST /api/playback returns directPlay + streamUrl", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, "movies/direct.mp4")
		meta := fixture.sourceMeta()
		meta.VideoCodec = playTestCodecH264
		meta.PackagingMode = transcode.PackagingRemux
		fixture.writePublishedCacheWithMeta(t, meta)

		body, err := json.Marshal(PlaybackNegotiateRequest{
			DeviceProfile: &playback.DeviceProfile{
				MaxStaticBitrate: 120_000_000,
				DirectPlay: []playback.DirectPlayProfile{{
					Container:  "mp4",
					VideoCodec: playTestCodecH264,
					AudioCodec: "aac",
				}},
			},
			Transcode: true,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/playback/"+fixture.rel,
			bytes.NewReader(body),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

		fixture.handler().playbackNegotiate(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var resp PlaybackResponse
		err = json.Unmarshal(recorder.Body.Bytes(), &resp)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.PlayMethod != playback.MethodDirectPlay {
			t.Fatalf("playMethod: got %q want directPlay", resp.PlayMethod)
		}
		if resp.Status != transcode.StatusReady {
			t.Fatalf("status: got %q want ready", resp.Status)
		}
		if !strings.HasPrefix(resp.StreamURL, "/api/stream/") {
			t.Fatalf("streamUrl: got %q", resp.StreamURL)
		}
		if resp.MasterURL != "" {
			t.Fatalf("expected empty masterUrl for Direct Play, got %q", resp.MasterURL)
		}
	})
}

func TestPlaybackNegotiate_RemuxWhenProfileMisses(t *testing.T) {
	t.Parallel()

	allure.Test(t, "POST /api/playback falls back to remux HLS when Direct Play impossible", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, "movies/remux.mkv")
		meta := fixture.sourceMeta()
		meta.VideoCodec = playTestCodecH264
		meta.PackagingMode = transcode.PackagingRemux
		fixture.writePublishedCacheWithMeta(t, meta)

		body, err := json.Marshal(PlaybackNegotiateRequest{
			DeviceProfile: &playback.DeviceProfile{
				DirectPlay: []playback.DirectPlayProfile{{
					Container:  "webm",
					VideoCodec: "vp9",
					AudioCodec: "opus",
				}},
			},
			Transcode: true,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/playback/"+fixture.rel,
			bytes.NewReader(body),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

		fixture.handler().playbackNegotiate(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var resp PlaybackResponse
		err = json.Unmarshal(recorder.Body.Bytes(), &resp)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.PlayMethod != playback.MethodRemux {
			t.Fatalf("playMethod: got %q want remux", resp.PlayMethod)
		}
		if resp.StreamURL != "" {
			t.Fatalf("expected no streamUrl, got %q", resp.StreamURL)
		}
		if resp.MasterURL == "" {
			t.Fatal("expected masterUrl for HLS remux")
		}
	})
}

func TestPlaybackNegotiate_TranscodeWhenLowerQuality(t *testing.T) {
	t.Parallel()

	allure.Test(t, "POST /api/playback uses transcode when lower quality selected", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, "movies/ladder.mp4")
		meta := fixture.sourceMeta()
		meta.Height = 1080
		meta.VideoCodec = playTestCodecH264
		meta.PackagingMode = transcode.PackagingRemux
		fixture.writePublishedCacheWithMeta(t, meta)

		body, err := json.Marshal(PlaybackNegotiateRequest{
			DeviceProfile: &playback.DeviceProfile{
				DirectPlay: []playback.DirectPlayProfile{{
					Container:  "mp4",
					VideoCodec: playTestCodecH264,
					AudioCodec: "aac",
				}},
			},
			QualityHeight: 720,
			Transcode:     true,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"/api/playback/"+fixture.rel,
			bytes.NewReader(body),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

		fixture.handler().playbackNegotiate(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var resp PlaybackResponse
		err = json.Unmarshal(recorder.Body.Bytes(), &resp)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.PlayMethod != playback.MethodTranscode {
			t.Fatalf("playMethod: got %q want transcode", resp.PlayMethod)
		}
	})
}
