package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const (
	playTestPathParamKey  = "path"
	playTestSampleRel     = "movies/sample.webm"
	playTestLibraryPath   = "movies"
	playTestLibraryName   = "Movies"
	playTestFileMode      = 0o600
	playTestSourceHeight  = 480
	testAudioLabelEnglish = "English"
	playTestCodecH264     = "h264"
)

type playTestFixture struct {
	root          string
	rel           string
	abs           string
	info          os.FileInfo
	media         *mediafs.Service
	transcode     *transcode.Service
	transcodeRoot string
}

func newPlayTestFixture(t *testing.T, rel string) *playTestFixture {
	t.Helper()

	root := t.TempDir()
	abs := filepath.Join(root, rel)
	err := os.MkdirAll(filepath.Dir(abs), 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err = os.WriteFile(abs, []byte("fake-video"), playTestFileMode)
	if err != nil {
		t.Fatalf("write video: %v", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("media service: %v", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		t.Fatalf("stat video: %v", err)
	}

	transcodeRoot := t.TempDir()
	transcodeSvc, err := transcode.NewService(transcodeRoot)
	if err != nil {
		t.Fatalf("transcode service: %v", err)
	}

	return &playTestFixture{
		root:          root,
		rel:           rel,
		abs:           abs,
		info:          info,
		media:         media,
		transcode:     transcodeSvc,
		transcodeRoot: transcodeRoot,
	}
}

func (f *playTestFixture) cacheKey() string {
	return f.transcode.CacheKey(f.abs, f.info.ModTime().Unix(), f.info.Size())
}

func (f *playTestFixture) cacheDir() string {
	return filepath.Join(f.transcodeRoot, f.cacheKey())
}

func (f *playTestFixture) sourceMeta() transcode.SourceMeta {
	return transcode.SourceMeta{
		Height:          playTestSourceHeight,
		Width:           854,
		DurationSeconds: 18,
		PackagingMode:   transcode.PackagingRemux,
		AudioStreams: []transcode.AudioStream{
			{Index: 1, Label: testAudioLabelEnglish, Language: "eng"},
		},
		Chapters: []transcode.ChapterInfo{
			{StartSeconds: 0, EndSeconds: 9, Title: "Cold open"},
			{StartSeconds: 9, EndSeconds: 18, Title: "Chapter 2"},
		},
		Segments: []float64{6, 6, 6},
	}
}

// writePublishedCache seeds a cache whose timeline is published but has no segments yet.
func (f *playTestFixture) writePublishedCache(t *testing.T) {
	t.Helper()
	f.writePublishedCacheWithMeta(t, f.sourceMeta())
}

func (f *playTestFixture) writePublishedCacheWithMeta(t *testing.T, meta transcode.SourceMeta) {
	t.Helper()

	outDir := f.cacheDir()

	err := os.MkdirAll(outDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}

	err = transcode.WriteSourceMeta(outDir, meta)
	if err != nil {
		t.Fatalf("write source meta: %v", err)
	}

	err = transcode.PublishPlaylists(outDir, meta)
	if err != nil {
		t.Fatalf("publish playlists: %v", err)
	}

	err = os.WriteFile(filepath.Join(outDir, ".complete"), []byte("ok"), playTestFileMode)
	if err != nil {
		t.Fatalf("write complete marker: %v", err)
	}
}

func (f *playTestFixture) writeReadyCache(t *testing.T) {
	t.Helper()

	outDir := f.cacheDir()

	err := os.MkdirAll(outDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	err = os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte("#EXTM3U\n"), playTestFileMode)
	if err != nil {
		t.Fatalf("write master: %v", err)
	}
	err = os.WriteFile(filepath.Join(outDir, ".complete"), []byte("ok"), playTestFileMode)
	if err != nil {
		t.Fatalf("write complete marker: %v", err)
	}
}

func (f *playTestFixture) writeProcessingCache(t *testing.T) {
	t.Helper()

	outDir := f.cacheDir()

	err := os.MkdirAll(outDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	err = os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte("#EXTM3U\n"), playTestFileMode)
	if err != nil {
		t.Fatalf("write master: %v", err)
	}

	f.transcode.HoldProcessingJob(f.cacheKey())
}

func (f *playTestFixture) holdProcessingJob() {
	f.transcode.HoldProcessingJob(f.cacheKey())
}

func (f *playTestFixture) handler() *handler {
	return &handler{media: f.media, transcode: f.transcode}
}

func TestIsVideoMediaFile(t *testing.T) {
	t.Parallel()

	allure.Test(t, "video extensions and MIME are accepted", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		webm := filepath.Join(root, "clip.webm")
		err := os.WriteFile(webm, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write webm: %v", err)
		}

		if !isVideoMediaFile(webm) {
			t.Fatal("expected webm to be video")
		}
		if isVideoMediaFile(filepath.Join(root, "notes.txt")) {
			t.Fatal("expected txt to be rejected")
		}
	})
}

func TestPlaybackHandler_ReturnsSkipIntroFromChapterTitles(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET /api/playback exposes skipIntro from opening chapter", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, playTestSampleRel)
		meta := fixture.sourceMeta()
		meta.DurationSeconds = 3600
		meta.Chapters = []transcode.ChapterInfo{
			{StartSeconds: 0, EndSeconds: 30, Title: "Recap"},
			{StartSeconds: 30, EndSeconds: 120, Title: "Opening Theme"},
		}
		fixture.writePublishedCacheWithMeta(t, meta)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/playback/"+fixture.rel,
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

		fixture.handler().playback(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var body PlaybackResponse
		err := json.Unmarshal(recorder.Body.Bytes(), &body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.SkipIntro == nil {
			t.Fatal("missing skipIntro")
		}
		if body.SkipIntro.StartMs != 30000 || body.SkipIntro.EndMs != 120000 {
			t.Fatalf("skipIntro range: %+v", body.SkipIntro)
		}
	})
}

func TestPlaybackHandler_ReturnsReadyFromSeededCache(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"GET /api/playback returns ready when cache is complete",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			fixture := newPlayTestFixture(t, playTestSampleRel)
			fixture.writePublishedCache(t)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/playback/"+fixture.rel,
				nil,
			)
			ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

			fixture.handler().playback(ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
			}

			var body PlaybackResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &body)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Status != transcode.StatusReady {
				t.Fatalf("status: got %q want ready", body.Status)
			}
			if body.MasterURL == "" {
				t.Fatalf("missing master URL: %+v", body)
			}
			if len(body.Chapters) != 2 {
				t.Fatalf("chapters: got %d want 2 (%+v)", len(body.Chapters), body.Chapters)
			}
			if body.Chapters[0].Title != "Cold open" {
				t.Fatalf("chapter title: %+v", body.Chapters[0])
			}
			if body.Series != nil {
				t.Fatalf("film path must not set series: %+v", body.Series)
			}
		},
	)
}

func TestServeHLSPlaylist_RewritesTokensAndNormalizes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "variant playlist is normalized and tokens appended", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		playlistPath := filepath.Join(root, "playlist.m3u8")
		body := `#EXTM3U
#EXT-X-PLAYLIST-TYPE:EVENT
#EXTINF:10.0,
seg_000.ts
#EXT-X-ENDLIST
`
		err := os.WriteFile(playlistPath, []byte(body), 0o600)
		if err != nil {
			t.Fatalf("write playlist: %v", err)
		}

		playHandler := &handler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/play/movies/sample.webm/stream_0/playlist.m3u8?access_token=test-token",
			nil,
		)

		playHandler.serveHLSPlaylist(ctx, playlistPath)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d", recorder.Code)
		}
		out := recorder.Body.String()
		if !strings.Contains(out, "#EXT-X-PLAYLIST-TYPE:VOD") {
			t.Fatalf("expected VOD normalization, got:\n%s", out)
		}
		if !strings.Contains(out, "seg_000.ts?access_token=test-token") {
			t.Fatalf("expected token rewrite, got:\n%s", out)
		}
		if recorder.Header().Get("Cache-Control") == "" {
			t.Fatal("expected cache-control header")
		}
	})
}

func TestPlayHandler_ExtensionlessVariantPlaylistResolvesMedia(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"extensionless media + vN playlist does not swallow the variant into the file path",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			// Same shape as Ubuntu screencast dumps: no extension, EBML/WebM sniff.
			rel := "folder/SDSO8J~O"
			fixture := newPlayTestFixture(t, rel)
			err := os.WriteFile(
				fixture.abs,
				[]byte{0x1a, 0x45, 0xdf, 0xa3, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1f},
				playTestFileMode,
			)
			if err != nil {
				t.Fatalf("rewrite extensionless webm: %v", err)
			}
			fixture.info, err = os.Stat(fixture.abs)
			if err != nil {
				t.Fatalf("restat: %v", err)
			}

			fixture.writePublishedCache(t)

			resource := "v480/playlist.m3u8"
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/play/"+rel+"/"+resource,
				nil,
			)
			ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: rel + "/" + resource}}

			fixture.handler().play(ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "#EXT-X-ENDLIST") {
				t.Fatalf("expected VOD playlist, got %q", recorder.Body.String())
			}
		},
	)
}

func TestPlayHandler_ServesPublishedMasterWithRenditionGroups(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET master.m3u8 returns the published ladder", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, playTestSampleRel)
		fixture.writePublishedCache(t)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/play/"+fixture.rel+"/master.m3u8?access_token=abc",
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel + "/master.m3u8"}}

		fixture.handler().play(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		body := recorder.Body.String()
		if !strings.Contains(body, "v480/playlist.m3u8?access_token=abc") {
			t.Fatalf("expected token rewrite in master, got %q", body)
		}
		if !strings.Contains(body, `TYPE=AUDIO`) {
			t.Fatalf("expected audio rendition group, got %q", body)
		}
	})
}

func TestPlaybackHandler_ExposesLadderFromPublishedCache(t *testing.T) {
	t.Parallel()

	allure.Test(t, "GET /api/playback lists every rung and track", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, "movies/partial.webm")
		fixture.writePublishedCache(t)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/playback/"+fixture.rel,
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

		fixture.handler().playback(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var body PlaybackResponse
		err := json.Unmarshal(recorder.Body.Bytes(), &body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Status != transcode.StatusReady {
			t.Fatalf("status: got %q want ready", body.Status)
		}
		if body.MasterURL == "" {
			t.Fatal("expected master URL")
		}
		if len(body.Qualities) == 0 {
			t.Fatalf("expected ladder qualities, got %+v", body.Qualities)
		}
		if len(body.AudioTracks) == 0 {
			t.Fatalf("expected audio tracks, got %+v", body.AudioTracks)
		}
	})
}

func TestServeHLSTranscode_Returns503WhenMasterNotReady(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"master.m3u8 returns 503 while processing without segments",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			fixture := newPlayTestFixture(t, "movies/processing.webm")
			fixture.writeProcessingCache(t)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/play/"+fixture.rel+"/master.m3u8",
				nil,
			)

			fixture.handler().serveHLSTranscode(
				ctx,
				fixture.rel,
				"master.m3u8",
				fixture.cacheKey(),
				fixture.abs,
			)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
			}
		},
	)
}

func TestDeleteMedia_RemovesTranscodeCache(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DELETE /api/media removes HLS cache for file", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, "movies/delete-me.webm")
		cacheDir := fixture.cacheDir()
		err := os.MkdirAll(cacheDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}

		playHandler := fixture.handler()
		router := gin.New()
		router.DELETE("/api/media/*path", playHandler.deleteMedia)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			"/api/media/"+fixture.rel,
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		_, statErr := os.Stat(fixture.abs)
		if !os.IsNotExist(statErr) {
			t.Fatal("expected media file deleted")
		}
		_, statErr = os.Stat(cacheDir)
		if !os.IsNotExist(statErr) {
			t.Fatal("expected transcode cache removed")
		}
	})
}

func TestWriteTranscodeResourceMissing_Returns503WhileProcessing(t *testing.T) {
	t.Parallel()

	allure.Test(t, "missing segment returns 503 while job is processing", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/play/movies/sample.webm/seg_000.ts",
			nil,
		)

		playHandler := &handler{}
		playHandler.writeTranscodeResourceMissing(
			ctx,
			transcode.JobStatus{Status: transcode.StatusProcessing},
		)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func TestServeHLSTranscode_ServesCachedSegment(t *testing.T) {
	t.Parallel()

	allure.Test(t, "an already produced segment is served without ffmpeg", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, playTestSampleRel)
		fixture.writePublishedCache(t)

		variantDir := filepath.Join(fixture.cacheDir(), transcode.VariantDir(playTestSourceHeight))

		err := os.MkdirAll(variantDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir variant: %v", err)
		}

		// Two segments on disk: segment 0 counts as complete because a later one exists.
		for index := range 2 {
			err = os.WriteFile(
				filepath.Join(variantDir, transcode.SegmentName(index)),
				[]byte("mpegts"),
				playTestFileMode,
			)
			if err != nil {
				t.Fatalf("write segment %d: %v", index, err)
			}
		}

		resource := transcode.VariantDir(playTestSourceHeight) + "/" + transcode.SegmentName(0)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/play/"+fixture.rel+"/"+resource,
			nil,
		)

		fixture.handler().serveHLSTranscode(
			ctx,
			fixture.rel,
			resource,
			fixture.cacheKey(),
			fixture.abs,
		)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
		if recorder.Body.String() != "mpegts" {
			t.Fatalf("unexpected body: %q", recorder.Body.String())
		}
	})
}

func TestServeHLSTranscode_UnknownVariantReturns404(t *testing.T) {
	t.Parallel()

	allure.Test(t, "segments for a rung above the source are rejected", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, playTestSampleRel)
		fixture.writePublishedCache(t)

		resource := "v2160/" + transcode.SegmentName(0)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/play/"+fixture.rel+"/"+resource,
			nil,
		)

		fixture.handler().serveHLSTranscode(
			ctx,
			fixture.rel,
			resource,
			fixture.cacheKey(),
			fixture.abs,
		)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPlaybackHandler_TranscodeUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"GET /api/playback returns 503 when transcode service is nil",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			fixture := newPlayTestFixture(t, playTestSampleRel)
			playHandler := &handler{media: fixture.media}

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/playback/"+fixture.rel,
				nil,
			)
			ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

			playHandler.playback(ctx)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status: got %d", recorder.Code)
			}
		},
	)
}
