package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sudoStream/internal/provider"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/gin-gonic/gin"
)

const providerSubTestRel = "shows/ep01.mkv"

func providerSubtitleRequest(recorder *httptest.ResponseRecorder, rel, lang string) *gin.Context {
	ctx, _ := gin.CreateTestContext(recorder)
	url := "/api/provider-subtitle/" + rel
	if lang != "" {
		url += "?lang=" + lang
	}
	ctx.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	ctx.Params = gin.Params{{Key: playTestPathParamKey, Value: "/" + rel}}

	return ctx
}

func TestProviderSubtitle_ServesCachedVTT(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cached provider subtitle is served as WebVTT", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cache, err := provider.NewCache(filepath.Join(t.TempDir(), "providers"))
		if err != nil {
			t.Fatalf("cache: %v", err)
		}
		cachePath := provider.SubtitleCachePath("lib-1", providerSubTestRel, "en")
		err = cache.Write(cachePath, []byte(userSubTestVTT))
		if err != nil {
			t.Fatalf("write: %v", err)
		}

		hdl := &handler{posters: &posterCache{
			artifacts: stubArtifacts{rows: map[string]provider.Artifact{
				providerSubTestRel: {
					ID:        "sub-1",
					LibraryID: providerTestLibraryID,
					RelPath:   providerSubTestRel,
					Kind:      provider.ArtifactKindSubtitle,
					Lang:      new("en"),
					CachePath: cachePath,
				},
			}},
			cache: cache,
		}}

		recorder := httptest.NewRecorder()
		hdl.providerSubtitle(providerSubtitleRequest(recorder, providerSubTestRel, "en"))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status: %d %s", recorder.Code, recorder.Body.String())
		}
		if got := recorder.Header().Get("Content-Type"); got != "text/vtt; charset=utf-8" {
			t.Fatalf("content-type: %q", got)
		}
	})
}

func TestProviderSubtitle_ValidationAndMissing(t *testing.T) {
	t.Parallel()

	allure.Test(t, "provider subtitle validates lang and returns 404 when missing", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cache, err := provider.NewCache(filepath.Join(t.TempDir(), "providers"))
		if err != nil {
			t.Fatalf("cache: %v", err)
		}

		wired := &handler{posters: &posterCache{
			artifacts: stubArtifacts{},
			cache:     cache,
		}}

		badLang := httptest.NewRecorder()
		wired.providerSubtitle(providerSubtitleRequest(badLang, providerSubTestRel, "english"))
		if badLang.Code != http.StatusBadRequest {
			t.Fatalf("bad lang: %d", badLang.Code)
		}

		unwired := &handler{}
		noStack := httptest.NewRecorder()
		unwired.providerSubtitle(providerSubtitleRequest(noStack, "movies/other.mkv", "en"))
		if noStack.Code != http.StatusNotFound {
			t.Fatalf("unwired: %d", noStack.Code)
		}

		missing := httptest.NewRecorder()
		wired.providerSubtitle(providerSubtitleRequest(missing, providerSubTestRel, "en"))
		if missing.Code != http.StatusNotFound {
			t.Fatalf("missing: %d", missing.Code)
		}
	})
}

func TestProviderSubtitle_StoreFailureReturns500(t *testing.T) {
	t.Parallel()

	allure.Test(t, "provider subtitle artifact lookup failures return 500", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cache, err := provider.NewCache(filepath.Join(t.TempDir(), "providers"))
		if err != nil {
			t.Fatalf("cache: %v", err)
		}

		hdl := &handler{posters: &posterCache{
			artifacts: stubArtifacts{err: errArtifactsBoom},
			cache:     cache,
		}}
		recorder := httptest.NewRecorder()
		hdl.providerSubtitle(providerSubtitleRequest(recorder, providerSubTestRel, "en"))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status: %d", recorder.Code)
		}
	})
}

func TestProviderSubtitle_InvalidCachePathReturns500(t *testing.T) {
	t.Parallel()

	allure.Test(t, "provider subtitle rejects cache paths outside root", func(a *allure.Context) {
		t := a.T()
		gin.SetMode(gin.TestMode)

		cache, err := provider.NewCache(filepath.Join(t.TempDir(), "providers"))
		if err != nil {
			t.Fatalf("cache: %v", err)
		}

		hdl := &handler{posters: &posterCache{
			artifacts: stubArtifacts{rows: map[string]provider.Artifact{
				providerSubTestRel: {
					RelPath:   providerSubTestRel,
					Kind:      provider.ArtifactKindSubtitle,
					Lang:      new("en"),
					CachePath: "../escape.vtt",
				},
			}},
			cache: cache,
		}}
		recorder := httptest.NewRecorder()
		hdl.providerSubtitle(providerSubtitleRequest(recorder, providerSubTestRel, "en"))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status: %d %s", recorder.Code, recorder.Body.String())
		}
	})
}

func TestProviderSubtitleTracks_ListsCachedLangs(t *testing.T) {
	t.Parallel()

	allure.Test(t, "providerSubtitleTracks lists langs and skips empty", func(a *allure.Context) {
		t := a.T()
		emptyLang := ""
		hdl := &handler{posters: &posterCache{
			artifacts: stubArtifacts{rows: map[string]provider.Artifact{
				"en": {
					RelPath: providerSubTestRel,
					Kind:    provider.ArtifactKindSubtitle,
					Lang:    new("en"),
				},
				"blank": {
					RelPath: providerSubTestRel,
					Kind:    provider.ArtifactKindSubtitle,
					Lang:    &emptyLang,
				},
				"poster": {
					RelPath: providerSubTestRel,
					Kind:    provider.ArtifactKindPoster,
					Lang:    new("xx"),
				},
			}},
		}}

		tracks := hdl.providerSubtitleTracks(context.Background(), providerSubTestRel)
		if len(tracks) != 1 || tracks[0].Lang != "en" || tracks[0].URL == "" {
			t.Fatalf("tracks: %+v", tracks)
		}

		if got := (&handler{}).providerSubtitleTracks(context.Background(), providerSubTestRel); got != nil {
			t.Fatalf("unwired: %+v", got)
		}

		errHdl := &handler{posters: &posterCache{artifacts: stubArtifacts{err: errArtifactsBoom}}}
		if got := errHdl.providerSubtitleTracks(context.Background(), providerSubTestRel); got != nil {
			t.Fatalf("error: %+v", got)
		}
	})
}
