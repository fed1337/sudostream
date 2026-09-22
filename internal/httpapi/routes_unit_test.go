package httpapi

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

var errRoutesUnitTestInternal = errors.New("routes unit test internal error")

func TestClampInt_ParsesAndBoundsValues(t *testing.T) {
	t.Parallel()

	allure.Test(t, "clampInt parses integers and applies bounds", func(a *allure.Context) {
		t := a.T()
		if got := clampInt("", 5, 1, 10); got != 5 {
			t.Fatalf("fallback: got %d", got)
		}
		if got := clampInt("abc", 5, 1, 10); got != 5 {
			t.Fatalf("invalid: got %d", got)
		}
		if got := clampInt("0", 5, 1, 10); got != 1 {
			t.Fatalf("min: got %d", got)
		}
		if got := clampInt("99", 5, 1, 10); got != 10 {
			t.Fatalf("max: got %d", got)
		}
		if got := clampInt("7", 5, 1, 10); got != 7 {
			t.Fatalf("in-range: got %d", got)
		}
	})
}

//nolint:paralleltest // setTestCacheRoot mutates package testCacheRoot
func TestCacheSubdir_UsesDefaultRoot(t *testing.T) {
	allure.Test(t, "cacheSubdir joins default or test root with subdir", func(a *allure.Context) {
		t := a.T()
		restore := setTestCacheRoot("/tmp/sudostream-cache")
		defer restore()

		got := cacheSubdir("hls")
		if got != filepath.Join("/tmp/sudostream-cache", "hls") {
			t.Fatalf("got %q", got)
		}

		restoreDefault := setTestCacheRoot("")
		defer restoreDefault()

		defaultGot := cacheSubdir("thumbs")
		if defaultGot != filepath.Join(defaultCacheRoot, "thumbs") {
			t.Fatalf("default: got %q", defaultGot)
		}
	})
}

func TestHandlePathError_MapsKnownErrors(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"handlePathError maps filesystem and validation errors",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			cases := []struct {
				name       string
				err        error
				wantStatus int
			}{
				{catalogMissingSlug, os.ErrNotExist, http.StatusNotFound},
				{"outside root", mediafs.ErrPathOutsideRoot, http.StatusBadRequest},
				{"invalid path", mediafs.ErrInvalidPath, http.StatusBadRequest},
				{"invalid type", fs.ErrInvalid, http.StatusBadRequest},
				{"internal", errRoutesUnitTestInternal, http.StatusInternalServerError},
			}

			for _, testCase := range cases {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequestWithContext(
					context.Background(),
					http.MethodGet,
					"/",
					nil,
				)

				handler := &handler{}
				handler.handlePathError(
					ctx,
					testCase.err,
					"internal",
					catalogMissingSlug,
					"invalid type",
				)

				if recorder.Code != testCase.wantStatus {
					t.Fatalf(
						"%s: got status %d want %d",
						testCase.name,
						recorder.Code,
						testCase.wantStatus,
					)
				}
			}
		},
	)
}

func TestCleanupDeletedMedia_RemovesTranscodeCache(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"cleanupDeletedMedia clears transcode cache for deleted files",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			fixture := newPlayTestFixture(t, playTestSampleRel)
			fixture.writeReadyCache(t)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodDelete,
				"/api/media/"+fixture.rel,
				nil,
			)

			playHandler := &handler{transcode: fixture.transcode}
			playHandler.cleanupDeletedMedia(ctx, fixture.rel, fixture.abs, fixture.info)

			_, statErr := os.Stat(fixture.cacheDir())
			if !os.IsNotExist(statErr) {
				t.Fatal("expected transcode cache removed")
			}
		},
	)
}

func TestEnrichItemsPermissions_RecursesWithoutAccess(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"enrichItemsPermissions recurses without granting delete when access is nil",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			playHandler := &handler{}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/api/browse",
				nil,
			)

			items := []mediafs.Item{
				{
					Name: "Demo",
					Path: "series/Demo",
					Children: []mediafs.Item{
						{Name: "S01E01.mkv", Path: "series/Demo/S01E01.mkv"},
					},
				},
			}

			enriched := playHandler.enrichItemsPermissions(ctx, auth.PublicUser{}, items)
			if enriched[0].Actions.CanDelete {
				t.Fatal("expected parent canDelete false")
			}
			if enriched[0].Children[0].Actions.CanDelete {
				t.Fatal("expected nested canDelete false without access service")
			}
		},
	)
}
