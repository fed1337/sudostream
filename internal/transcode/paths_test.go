package transcode_test

import (
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testMasterPlaylist     = "master.m3u8"
	testExtensionlessMedia = "folder/SDSO8J~O"
)

func TestSplitPlayPath_MasterPlaylist(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"media path without suffix defaults to master playlist",
		func(a *allure.Context) {
			t := a.T()
			media, resource, ok := transcode.SplitPlayPath("movies/foo.mp4")
			if !ok {
				t.Fatal("expected ok")
			}
			if media != "movies/foo.mp4" {
				t.Fatalf("media=%q", media)
			}
			if resource != testMasterPlaylist {
				t.Fatalf("resource=%q", resource)
			}
		},
	)
}

func TestSplitPlayPath_SegmentPath(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nested HLS resources are preserved", func(a *allure.Context) {
		t := a.T()
		media, resource, ok := transcode.SplitPlayPath("movies/foo.mp4/stream_0/seg_001.ts")
		if !ok {
			t.Fatal("expected ok")
		}
		if media != "movies/foo.mp4" {
			t.Fatalf("media=%q", media)
		}
		if resource != "stream_0/seg_001.ts" {
			t.Fatalf("resource=%q", resource)
		}
	})
}

func TestSplitPlayPath_ExtensionlessMaster(t *testing.T) {
	t.Parallel()

	allure.Test(t, "master playlist for extensionless media", func(a *allure.Context) {
		t := a.T()
		media, resource, parsed := transcode.SplitPlayPath(testExtensionlessMedia + "/master.m3u8")
		if !parsed {
			t.Fatal("expected ok")
		}
		if media != testExtensionlessMedia {
			t.Fatalf("media=%q", media)
		}
		if resource != testMasterPlaylist {
			t.Fatalf("resource=%q", resource)
		}
	})
}

func TestSplitPlayPath_ExtensionlessBarePath(t *testing.T) {
	t.Parallel()

	allure.Test(t, "bare extensionless media path defaults to master", func(a *allure.Context) {
		t := a.T()
		media, resource, parsed := transcode.SplitPlayPath(testExtensionlessMedia)
		if !parsed {
			t.Fatal("expected ok for bare extensionless path")
		}
		if media != testExtensionlessMedia {
			t.Fatalf("media=%q", media)
		}
		if resource != testMasterPlaylist {
			t.Fatalf("resource=%q", resource)
		}
	})
}

func TestSplitPlayPath_ExtensionlessVariant(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"extensionless variant playlist keeps vN directory in resource",
		func(a *allure.Context) {
			t := a.T()
			media, resource, parsed := transcode.SplitPlayPath(
				testExtensionlessMedia + "/v480/playlist.m3u8",
			)
			if !parsed {
				t.Fatal("expected ok for extensionless variant playlist")
			}
			if media != testExtensionlessMedia {
				t.Fatalf("media=%q", media)
			}
			if resource != "v480/playlist.m3u8" {
				t.Fatalf("resource=%q", resource)
			}
		},
	)
}

func TestSplitPlayPath_ExtensionlessSegment(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"extensionless segment path keeps vN directory in resource",
		func(a *allure.Context) {
			t := a.T()
			media, resource, parsed := transcode.SplitPlayPath(
				testExtensionlessMedia + "/v480/seg_00001.ts",
			)
			if !parsed {
				t.Fatal("expected ok for extensionless segment path")
			}
			if media != testExtensionlessMedia {
				t.Fatalf("media=%q", media)
			}
			if resource != "v480/seg_00001.ts" {
				t.Fatalf("resource=%q", resource)
			}
		},
	)
}

func TestSplitPlayPath_ExtensionlessAudioAndSubtitles(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"extensionless audio and subtitle resources split correctly",
		func(a *allure.Context) {
			t := a.T()
			media, resource, parsed := transcode.SplitPlayPath(
				testExtensionlessMedia + "/a0/playlist.m3u8",
			)
			if !parsed || media != testExtensionlessMedia || resource != "a0/playlist.m3u8" {
				t.Fatalf("audio: media=%q resource=%q ok=%v", media, resource, parsed)
			}

			media, resource, parsed = transcode.SplitPlayPath(
				testExtensionlessMedia + "/subtitles/eng/playlist.m3u8",
			)
			if !parsed || media != testExtensionlessMedia ||
				resource != "subtitles/eng/playlist.m3u8" {
				t.Fatalf("subs: media=%q resource=%q ok=%v", media, resource, parsed)
			}
		},
	)
}

func TestSplitPlayPath_WebmVariant(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"webm media keeps extension and strips variant resource",
		func(a *allure.Context) {
			t := a.T()
			media, resource, parsed := transcode.SplitPlayPath(
				"folder/clip.webm/v360/playlist.m3u8",
			)
			if !parsed {
				t.Fatal("expected ok")
			}
			if media != "folder/clip.webm" {
				t.Fatalf("media=%q", media)
			}
			if resource != "v360/playlist.m3u8" {
				t.Fatalf("resource=%q", resource)
			}
		},
	)
}

func TestQualityRungs_NoUpscale(t *testing.T) {
	t.Parallel()

	allure.Test(t, "rungs are capped by source height", func(a *allure.Context) {
		t := a.T()
		rungs := transcode.QualityRungs(720)
		if len(rungs) == 0 {
			t.Fatal("expected rungs")
		}
		for _, rung := range rungs {
			if rung.Height > 720 {
				t.Fatalf("upscale rung %d", rung.Height)
			}
		}
	})
}

func TestContentTypeForResource(t *testing.T) {
	t.Parallel()

	allure.Test(t, "HLS resources map to expected MIME types", func(a *allure.Context) {
		t := a.T()
		if got := transcode.ContentTypeForResource("master.m3u8"); got == "" {
			t.Fatal("expected m3u8 content type")
		}
		if got := transcode.ContentTypeForResource("seg.ts"); got == "" {
			t.Fatal("expected ts content type")
		}
	})
}
