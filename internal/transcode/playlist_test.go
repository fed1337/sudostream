package transcode_test

import (
	"strings"
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestRewritePlaylistTokens_AppendsTokenToRelativeURIs(t *testing.T) {
	t.Parallel()

	allure.Test(t, "master and media playlists rewrite child URIs", func(a *allure.Context) {
		t := a.T()
		master := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=800000
stream_0/playlist.m3u8
`
		rewritten := string(transcode.RewritePlaylistTokens([]byte(master), "jwt-token"))
		if !strings.Contains(rewritten, "stream_0/playlist.m3u8?access_token=jwt-token") {
			t.Fatalf("master rewrite missing token: %q", rewritten)
		}

		media := `#EXTM3U
#EXTINF:6.000000,
seg_000.ts
`
		rewritten = string(transcode.RewritePlaylistTokens([]byte(media), "jwt-token"))
		if !strings.Contains(rewritten, "seg_000.ts?access_token=jwt-token") {
			t.Fatalf("media rewrite missing token: %q", rewritten)
		}
	})
}

func TestRewritePlaylistTokens_RewritesEXTXMEDIAURI(t *testing.T) {
	t.Parallel()

	allure.Test(t, "EXT-X-MEDIA URI tags get access tokens", func(a *allure.Context) {
		t := a.T()
		body := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="English",URI="audio/eng.m3u8"
`
		rewritten := string(transcode.RewritePlaylistTokens([]byte(body), "jwt-token"))
		if !strings.Contains(rewritten, `audio/eng.m3u8?access_token=jwt-token`) {
			t.Fatalf("expected URI rewrite, got:\n%s", rewritten)
		}
	})
}

func TestRewritePlaylistTokens_LeavesBodyUnchangedWithoutToken(t *testing.T) {
	t.Parallel()

	allure.Test(t, "empty token is a no-op", func(a *allure.Context) {
		t := a.T()
		body := []byte("#EXTM3U\nseg_000.ts\n")
		rewritten := transcode.RewritePlaylistTokens(body, "")
		if string(rewritten) != string(body) {
			t.Fatalf("unexpected rewrite: %q", rewritten)
		}
	})
}

func TestNormalizePlaylistForClient_EventToVodWhenComplete(t *testing.T) {
	t.Parallel()

	allure.Test(t, "completed event playlist becomes vod", func(a *allure.Context) {
		t := a.T()
		input := []byte(`#EXTM3U
#EXT-X-PLAYLIST-TYPE:EVENT
#EXTINF:4.0,
seg_000.ts
#EXT-X-ENDLIST
`)
		out := string(transcode.NormalizePlaylistForClient(input))
		if !strings.Contains(out, "#EXT-X-PLAYLIST-TYPE:VOD") {
			t.Fatalf("expected VOD playlist type, got:\n%s", out)
		}
		if strings.Contains(out, "EVENT") {
			t.Fatalf("expected EVENT removed, got:\n%s", out)
		}
	})
}

func TestNormalizePlaylistForClient_LeavesInProgressEvent(t *testing.T) {
	t.Parallel()

	allure.Test(t, "in-progress event playlist unchanged", func(a *allure.Context) {
		t := a.T()
		input := []byte(`#EXTM3U
#EXT-X-PLAYLIST-TYPE:EVENT
#EXTINF:4.0,
seg_000.ts
`)
		out := string(transcode.NormalizePlaylistForClient(input))
		if !strings.Contains(out, "#EXT-X-PLAYLIST-TYPE:EVENT") {
			t.Fatalf("expected EVENT preserved, got:\n%s", out)
		}
	})
}

func TestNormalizePlaylistForClient_StripsLeadingDiscontinuity(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"leading discontinuity before first segment is removed",
		func(a *allure.Context) {
			t := a.T()
			input := []byte(`#EXTM3U
#EXT-X-PLAYLIST-TYPE:EVENT
#EXT-X-DISCONTINUITY
#EXTINF:10.0,
seg_000.ts
#EXT-X-ENDLIST
`)
			out := string(transcode.NormalizePlaylistForClient(input))
			if strings.Contains(out, "#EXT-X-DISCONTINUITY") {
				t.Fatalf("expected leading discontinuity removed, got:\n%s", out)
			}
		},
	)
}

func TestPlaylistCacheControl(t *testing.T) {
	t.Parallel()

	allure.Test(t, "complete playlist is cacheable", func(a *allure.Context) {
		t := a.T()
		control := transcode.PlaylistCacheControl([]byte("#EXTM3U\n#EXT-X-ENDLIST\n"))
		if !strings.Contains(control, "max-age") {
			t.Fatalf("expected cacheable control, got %q", control)
		}
	})

	allure.Test(t, "growing playlist is not cacheable", func(a *allure.Context) {
		t := a.T()
		control := transcode.PlaylistCacheControl([]byte("#EXTM3U\n#EXTINF:10,\nseg.ts\n"))
		if !strings.Contains(control, "no-cache") {
			t.Fatalf("expected no-cache control, got %q", control)
		}
	})
}
