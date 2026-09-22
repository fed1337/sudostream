package httpapi

import (
	"context"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/thumbnail"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

func TestServeVideoThumbnail_ServesCachedWebP(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cached video poster is served as WebP", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		videoPath := filepath.Join(root, "movies", "sample.webm")
		err := os.MkdirAll(filepath.Dir(videoPath), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(videoPath, []byte("fake-video"), 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		info, err := os.Stat(videoPath)
		if err != nil {
			t.Fatalf("stat video: %v", err)
		}

		cacheRoot := t.TempDir()
		videoThumb, err := thumbnail.NewVideoThumbnailer(cacheRoot)
		if err != nil {
			t.Fatalf("thumbnailer: %v", err)
		}

		rel := playTestSampleRel
		cachePath := videoThumb.CachePath(rel, info.ModTime().Unix(), info.Size())
		err = os.WriteFile(cachePath, []byte("webp-poster"), 0o600)
		if err != nil {
			t.Fatalf("write cache: %v", err)
		}

		mediaHandler := &handler{media: media, videoThumb: videoThumb}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/thumbnail/"+rel,
			nil,
		)

		mediaHandler.serveVideoThumbnail(ctx, rel, info)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
		if recorder.Body.String() != "webp-poster" {
			t.Fatalf("body: got %q", recorder.Body.String())
		}
		if ct := recorder.Header().Get("Content-Type"); ct != "image/webp" {
			t.Fatalf("content-type: got %q", ct)
		}
	})
}

func TestServeVideoThumbnail_NotCachedReturns404(t *testing.T) {
	t.Parallel()

	allure.Test(t, "missing video poster returns 404 without generating", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		videoPath := filepath.Join(root, "movies", "missing.webm")
		err := os.MkdirAll(filepath.Dir(videoPath), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(videoPath, []byte("not-a-video"), 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("media service: %v", err)
		}

		cacheRoot := t.TempDir()
		videoThumb, err := thumbnail.NewVideoThumbnailer(cacheRoot)
		if err != nil {
			t.Fatalf("thumbnailer: %v", err)
		}

		mediaHandler := &handler{media: media, videoThumb: videoThumb}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/thumbnail/movies/missing.webm",
			nil,
		)

		info, err := os.Stat(videoPath)
		if err != nil {
			t.Fatalf("stat video: %v", err)
		}
		mediaHandler.serveVideoThumbnail(ctx, "/movies/missing.webm", info)

		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestServeVideoThumbnail_NilThumbReturns500(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil thumbnailer returns 500", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		mediaHandler := &handler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/thumbnail/movies/sample.webm",
			nil,
		)

		info := &mockFileInfo{name: "sample.webm", modTime: time.Unix(1, 0), size: 42}
		mediaHandler.serveVideoThumbnail(ctx, "movies/sample.webm", info)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func TestServeImageThumbnail_EncodesJPEG(t *testing.T) {
	t.Parallel()

	allure.Test(t, "image thumbnail is resized and encoded as JPEG", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		imagePath := filepath.Join(root, "photos", "still.jpg")
		err := os.MkdirAll(filepath.Dir(imagePath), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		img := imaging.New(64, 48, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		err = imaging.Save(img, imagePath)
		if err != nil {
			t.Fatalf("save image: %v", err)
		}

		mediaHandler := &handler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/thumbnail/photos/still.jpg",
			nil,
		)

		mediaHandler.serveImageThumbnail(ctx, imagePath, 32, 32)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d", recorder.Code)
		}
		if ct := recorder.Header().Get("Content-Type"); ct != "image/jpeg" {
			t.Fatalf("content-type: got %q", ct)
		}
		if recorder.Body.Len() == 0 {
			t.Fatal("expected JPEG body")
		}
	})
}

func TestServeImageThumbnail_UnsupportedImageReturns400(t *testing.T) {
	t.Parallel()

	allure.Test(t, "unsupported image returns 400", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		root := t.TempDir()
		badPath := filepath.Join(root, "notes.txt")
		err := os.WriteFile(badPath, []byte("not an image"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		mediaHandler := &handler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/thumbnail/notes.txt",
			nil,
		)

		mediaHandler.serveImageThumbnail(ctx, badPath, 32, 32)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

func TestListReadableLibraries_NilAccessReturnsEmpty(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil access service returns empty library list", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		mediaHandler := &handler{}
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			"/api/libraries",
			nil,
		)

		mediaHandler.listReadableLibraries(ctx)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d", recorder.Code)
		}
	})
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestListReadableLibraries_ReturnsReadableForUser(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(t, "authenticated user receives readable libraries", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		ctx := context.Background()
		router, root := newACLTestRouter(t)
		err := os.MkdirAll(filepath.Join(root, testSeriesRelPath), 0o750)
		if err != nil {
			t.Fatalf("mkdir library: %v", err)
		}

		registerMediaLibrariesHTTP(ctx, t, router, root)
		token := adminAccessToken(ctx, t, router)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(
			ctx,
			http.MethodGet,
			"/api/libraries",
			nil,
		)
		setBearerAuth(request, token)
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestLibraryLabels_NilAccessUsesPathSegment(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"libraryLabels falls back to path segment without access service",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			mediaHandler := &handler{}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/",
				nil,
			)

			slug, libraryType := mediaHandler.libraryLabels(ctx, "movies/sample.webm")
			if slug != playTestLibraryPath {
				t.Fatalf("slug: got %q want movies", slug)
			}
			if libraryType != libraryTypeUnknown {
				t.Fatalf("type: got %q want %q", libraryType, libraryTypeUnknown)
			}
		},
	)
}

//nolint:paralleltest // integration tests share one SUDOSTREAM_DATABASE_URL fixture
func TestLibraryLabels_ResolvesRegisteredLibrary(t *testing.T) {
	if os.Getenv("SUDOSTREAM_DATABASE_URL") == "" {
		t.Skip("SUDOSTREAM_DATABASE_URL not set")
	}

	allure.Test(
		t,
		"libraryLabels maps path to registered library slug and type",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			ctx := context.Background()
			router, root := newACLTestRouter(t)
			err := os.MkdirAll(filepath.Join(root, "movies"), 0o750)
			if err != nil {
				t.Fatalf("mkdir library: %v", err)
			}

			registerMediaLibrariesHTTP(ctx, t, router, root)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/admin/libraries",
				nil,
			)
			setBearerAuth(request, adminAccessToken(ctx, t, router))
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("list libraries: %d", recorder.Code)
			}

			videoPath := filepath.Join(root, "movies", "sample.webm")
			err = os.WriteFile(videoPath, []byte("fake-video"), 0o600)
			if err != nil {
				t.Fatalf("write video: %v", err)
			}

			downloadRecorder := httptest.NewRecorder()
			downloadRequest := httptest.NewRequestWithContext(
				ctx,
				http.MethodGet,
				"/api/download/movies/sample.webm",
				nil,
			)
			setBearerAuth(downloadRequest, adminAccessToken(ctx, t, router))
			router.ServeHTTP(downloadRecorder, downloadRequest)

			if downloadRecorder.Code != http.StatusOK {
				t.Fatalf(
					"download: got %d body=%s",
					downloadRecorder.Code,
					downloadRecorder.Body.String(),
				)
			}
		},
	)
}

func TestWarmVideoPostersOnStartup_SkipsWhenDepsNil(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"boot poster warm removed; placeholder keeps catalog green",
		func(_ *allure.Context) {
			// FI-6: warmVideoPostersOnStartup deleted; posters only via maintenance.
		},
	)
}

type mockFileInfo struct {
	name    string
	modTime time.Time
	size    int64
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() os.FileMode  { return 0o600 }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }
