package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

func TestServeHLSTranscode_VariantPlaylistIsServedBeforeAnySegment(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"every ladder rung is playable immediately, with no segments encoded",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			fixture := newPlayTestFixture(t, playTestSampleRel)
			fixture.writePublishedCache(t)

			for _, resource := range []string{
				"v240/playlist.m3u8",
				"v480/playlist.m3u8",
				"a0/playlist.m3u8",
			} {
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
					t.Fatalf("%s: status got %d body=%s", resource, recorder.Code, recorder.Body)
				}
				if !strings.Contains(recorder.Body.String(), "#EXT-X-ENDLIST") {
					t.Fatalf("%s: expected complete VOD playlist, got %q", resource, recorder.Body)
				}
			}
		},
	)
}

func TestPlaybackHandler_QualitiesHaveNoPerRungState(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"playback lists the whole ladder as immediately usable",
		func(a *allure.Context) {
			t := a.T()
			gin.SetMode(gin.TestMode)

			fixture := newPlayTestFixture(t, playTestSampleRel)
			fixture.writePublishedCache(t)

			outDir := fixture.cacheDir()

			meta, ok := transcode.ReadSourceMeta(outDir)
			if !ok {
				t.Fatal("expected published source meta")
			}

			qualities := meta.Qualities()
			if len(qualities) < 2 {
				t.Fatalf("expected a multi-rung ladder, got %+v", qualities)
			}
			if qualities[0].Height != playTestSourceHeight {
				t.Fatalf("expected native rung first, got %+v", qualities)
			}
		},
	)
}

func setTestUser(c *gin.Context, user auth.PublicUser) {
	c.Set(userContextKey, user)
}
