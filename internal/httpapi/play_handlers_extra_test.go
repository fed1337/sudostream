package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sudoStream/internal/transcode"
	"sudoStream/internal/trash"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestPlayHandler_InvalidPathReturns400(t *testing.T) {
	t.Parallel()

	allure.Test(t, "play rejects invalid HLS path", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, playTestSampleRel)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/play//master.m3u8",
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: "/master.m3u8"}}

		fixture.handler().play(ctx)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPlayHandler_TranscodeErrorReturns500(t *testing.T) {
	t.Parallel()

	allure.Test(t, "play returns 500 when transcode job failed", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, playTestSampleRel)
		cacheDir := fixture.cacheDir()
		err := os.MkdirAll(cacheDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}
		err = os.WriteFile(filepath.Join(cacheDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600)
		if err != nil {
			t.Fatalf("write master: %v", err)
		}
		err = os.WriteFile(filepath.Join(cacheDir, ".error"), []byte("ffmpeg failed"), 0o600)
		if err != nil {
			t.Fatalf("write error marker: %v", err)
		}

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

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestWriteTranscodeResourceMissing_Returns404WhenReady(t *testing.T) {
	t.Parallel()

	allure.Test(t, "missing segment returns 404 when job is ready", func(a *allure.Context) {
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
			transcode.JobStatus{Status: transcode.StatusReady},
		)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func TestServeHLSPlaylist_ReadErrorReturns500(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"serveHLSPlaylist returns 500 when playlist file is missing",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			playHandler := &handler{}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/play/movies/sample.webm/master.m3u8",
				nil,
			)

			playHandler.serveHLSPlaylist(ctx, filepath.Join(t.TempDir(), "missing.m3u8"))

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status: got %d", recorder.Code)
			}
		},
	)
}

func TestPlaybackHandler_ReturnsStatusFromHeldJob(t *testing.T) {
	t.Parallel()

	allure.Test(t, "playback reflects held transcode job state", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cases := []struct {
			name          string
			prepare       func(*testing.T, *playTestFixture)
			transcodeQ    bool
			wantStatus    transcode.Status
			wantMasterURL bool
		}{
			{
				name: "transcode=1 with held job",
				prepare: func(_ *testing.T, f *playTestFixture) {
					f.holdProcessingJob()
				},
				transcodeQ:    true,
				wantStatus:    transcode.StatusProcessing,
				wantMasterURL: false,
			},
			{
				name: "ready cache without transcode query",
				prepare: func(t *testing.T, f *playTestFixture) {
					t.Helper()
					f.writeReadyCache(t)
				},
				transcodeQ:    false,
				wantStatus:    transcode.StatusReady,
				wantMasterURL: true,
			},
		}

		for _, testCase := range cases {
			fixture := newPlayTestFixture(t, playTestSampleRel)
			testCase.prepare(t, fixture)

			path := "/api/playback/" + fixture.rel
			if testCase.transcodeQ {
				path += "?transcode=1"
			}

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				path,
				nil,
			)
			ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel}}

			fixture.handler().playback(ctx)

			if recorder.Code != http.StatusOK {
				t.Fatalf(
					"%s: status code got %d body=%s",
					testCase.name,
					recorder.Code,
					recorder.Body.String(),
				)
			}

			var body PlaybackResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &body)
			if err != nil {
				t.Fatalf("%s: decode: %v", testCase.name, err)
			}
			if body.Status != testCase.wantStatus {
				t.Fatalf(
					"%s: job status got %q want %q",
					testCase.name,
					body.Status,
					testCase.wantStatus,
				)
			}
			if testCase.wantMasterURL && body.MasterURL == "" {
				t.Fatalf("%s: expected master URL", testCase.name)
			}
			if !testCase.wantMasterURL && body.MasterURL != "" {
				t.Fatalf("%s: unexpected master URL %q", testCase.name, body.MasterURL)
			}
		}
	})
}

func TestDeleteMedia_NonEmptyDirectoryReturns409(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"DELETE /api/media returns 409 for non-empty directory",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			root := t.TempDir()
			dirPath := filepath.Join(root, "movies", "pack")
			err := os.MkdirAll(dirPath, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(filepath.Join(dirPath, "clip.webm"), []byte("x"), 0o600)
			if err != nil {
				t.Fatalf("write file: %v", err)
			}

			media, err := newMediaAtRoot(t, root)
			if err != nil {
				t.Fatalf("media service: %v", err)
			}

			playHandler := &handler{media: media}
			router := gin.New()
			router.DELETE("/api/media/*path", playHandler.deleteMedia)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodDelete,
				"/api/media/movies/pack",
				nil,
			)
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusConflict {
				t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
			}
		},
	)
}

func TestDeleteMedia_MovesIntoTrashWhenWired(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DELETE /api/media moves a directory into recycle bin", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		dirPath := filepath.Join(root, "movies", "pack")
		err := os.MkdirAll(dirPath, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(dirPath, "clip.webm"), []byte("x"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		media, err := newMediaAtRoot(t, root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		playHandler := &handler{
			media: media,
			trash: trash.NewService(media, newTrashMemoryStore(), nil, nil),
		}
		router := gin.New()
		router.DELETE("/api/media/*path", playHandler.deleteMedia)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			"/api/media/movies/pack",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
		_, err = os.Stat(dirPath)
		if !os.IsNotExist(err) {
			t.Fatal("expected source directory gone")
		}
		ids, err := media.ListTrashItemIDs()
		if err != nil || len(ids) != 1 {
			t.Fatalf("trash ids: %v %v", ids, err)
		}
	})
}

func TestDeleteMedia_ReadOnlyVolumeReturns403(t *testing.T) {
	t.Parallel()

	allure.Test(t, "DELETE /api/media returns 403 on read-only volume", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		dirPath := filepath.Join(root, "movies")
		err := os.MkdirAll(dirPath, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		filePath := filepath.Join(dirPath, "clip.webm")
		err = os.WriteFile(filePath, []byte("x"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		err = os.Chmod( //nolint:gosec // simulate read-only parent directory for delete test
			dirPath,
			0o555,
		)
		if err != nil {
			t.Fatalf("chmod dir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(dirPath, 0o750) //nolint:gosec // restore permissions for temp dir cleanup
		})

		media, err := newMediaAtRoot(t, root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		playHandler := &handler{media: media}
		router := gin.New()
		router.DELETE("/api/media/*path", playHandler.deleteMedia)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			context.Background(),
			http.MethodDelete,
			"/api/media/movies/clip.webm",
			nil,
		)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}

		var body map[string]string
		err = json.Unmarshal(recorder.Body.Bytes(), &body)
		if err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["error"] != "no write access to the movies folder" {
			t.Fatalf("unexpected error: %#v", body["error"])
		}
	})
}

func TestStatMediaPath_ResolvesDirectory(t *testing.T) {
	t.Parallel()

	allure.Test(t, "statMediaPath resolves directory paths", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		dirPath := filepath.Join(root, "movies")
		err := os.MkdirAll(dirPath, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		media, err := newMediaAtRoot(t, root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		playHandler := &handler{media: media}
		gotPath, info, statErr := playHandler.statMediaPath("movies")
		if statErr != nil {
			t.Fatalf("statMediaPath: %v", statErr)
		}
		if gotPath != dirPath {
			t.Fatalf("path: got %q want %q", gotPath, dirPath)
		}
		if !info.IsDir() {
			t.Fatal("expected directory info")
		}
	})
}

func TestPlayHandler_NilTranscodeReturns503(t *testing.T) {
	t.Parallel()

	allure.Test(t, "play returns 503 when transcode service is nil", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		fixture := newPlayTestFixture(t, playTestSampleRel)
		playHandler := &handler{media: fixture.media}

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/play/"+fixture.rel+"/master.m3u8",
			nil,
		)
		ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: fixture.rel + "/master.m3u8"}}

		playHandler.play(ctx)

		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func TestOpenVideoFile_RejectsNonVideo(t *testing.T) {
	t.Parallel()

	allure.Test(t, "openVideoFile rejects non-video files", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("text"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		media, err := newMediaAtRoot(t, root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		playHandler := &handler{media: media}
		_, _, openErr := playHandler.openVideoFile("notes.txt")
		if !errors.Is(openErr, fs.ErrInvalid) {
			t.Fatalf("expected fs.ErrInvalid, got %v", openErr)
		}
	})
}
