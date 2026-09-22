package transcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestEnsureSubtitleMediaPlaylists_BackfillsLegacyFlatVTT(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"legacy flat vtt gets a subtitle media playlist wrapper",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			err := os.MkdirAll(filepath.Join(root, "subtitles"), 0o750)
			if err != nil {
				t.Fatalf("mkdir subtitles: %v", err)
			}

			err = os.WriteFile(
				filepath.Join(root, "subtitles", "srt.vtt"),
				[]byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello\n"),
				0o600,
			)
			if err != nil {
				t.Fatalf("write legacy vtt: %v", err)
			}

			err = ensureSubtitleMediaPlaylists(root, 2707.457)
			if err != nil {
				t.Fatalf("ensure subtitle playlists: %v", err)
			}

			playlistPath := filepath.Join(root, "subtitles", "srt", "playlist.m3u8")
			raw, err := os.ReadFile(playlistPath) //nolint:gosec // test temp dir
			if err != nil {
				t.Fatalf("read playlist: %v", err)
			}

			body := string(raw)
			if !strings.Contains(body, "#EXTM3U") || !strings.Contains(body, "../srt.vtt") {
				t.Fatalf("expected wrapped subtitle playlist, got:\n%s", body)
			}

			tracks := listPackagedSubtitleTracks(root)
			if len(tracks) != 1 || tracks[0].DisplayName != "Subtitles" {
				t.Fatalf("tracks: %+v", tracks)
			}
		},
	)
}

func TestSubtitleVTTHasCues_RequiresCueTimestamps(t *testing.T) {
	t.Parallel()

	allure.Test(t, "header-only vtt is invalid", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		path := filepath.Join(root, "empty.vtt")
		err := os.WriteFile(path, []byte("WEBVTT\n"), 0o600)
		if err != nil {
			t.Fatalf("write vtt: %v", err)
		}

		if subtitleVTTHasCues(path) {
			t.Fatal("expected header-only vtt to be invalid")
		}
	})

	allure.Test(t, "vtt with cue timestamps is valid", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		path := filepath.Join(root, "valid.vtt")
		err := os.WriteFile(path, []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n"), 0o600)
		if err != nil {
			t.Fatalf("write vtt: %v", err)
		}

		if !subtitleVTTHasCues(path) {
			t.Fatal("expected vtt with cues to be valid")
		}
	})
}

func TestWriteSubtitleMediaPlaylist_WritesVODPlaylist(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"subtitle media playlist references a single vtt segment",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			playlistPath := filepath.Join(root, "subtitles", "eng", "playlist.m3u8")

			err := writeSubtitleMediaPlaylist(playlistPath, "track.vtt", 120.5)
			if err != nil {
				t.Fatalf("write subtitle playlist: %v", err)
			}

			raw, err := os.ReadFile(playlistPath) //nolint:gosec // test temp dir
			if err != nil {
				t.Fatalf("read playlist: %v", err)
			}

			body := string(raw)
			if !strings.Contains(body, "#EXT-X-PLAYLIST-TYPE:VOD") ||
				!strings.Contains(body, "track.vtt") ||
				!strings.Contains(body, "#EXTINF:120.500,") {
				t.Fatalf("unexpected playlist body:\n%s", body)
			}
		},
	)
}

//nolint:cyclop // filesystem fixture setup branches
func TestPurgeInvalidSubtitleTracks_RemovesCueLessArtifacts(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"purge drops empty track dirs and flat vtt without cues",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			subtitles := filepath.Join(root, "subtitles")
			emptyDir := filepath.Join(subtitles, "empty")
			validDir := filepath.Join(subtitles, "eng")
			err := os.MkdirAll(emptyDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir empty: %v", err)
			}
			err = os.MkdirAll(validDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir valid: %v", err)
			}

			err = os.WriteFile(
				filepath.Join(emptyDir, subtitleSegmentName),
				[]byte("WEBVTT\n"),
				0o600,
			)
			if err != nil {
				t.Fatalf("write empty vtt: %v", err)
			}
			err = os.WriteFile(
				filepath.Join(validDir, subtitleSegmentName),
				[]byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n"),
				0o600,
			)
			if err != nil {
				t.Fatalf("write valid vtt: %v", err)
			}
			err = writeSubtitleMediaPlaylist(
				filepath.Join(validDir, "playlist.m3u8"),
				subtitleSegmentName,
				10,
			)
			if err != nil {
				t.Fatalf("write playlist: %v", err)
			}
			err = os.WriteFile(filepath.Join(subtitles, "bad.vtt"), []byte("WEBVTT\n"), 0o600)
			if err != nil {
				t.Fatalf("write bad flat vtt: %v", err)
			}
			err = os.WriteFile(filepath.Join(subtitles, "readme.txt"), []byte("ignore"), 0o600)
			if err != nil {
				t.Fatalf("write unrelated: %v", err)
			}

			purgeInvalidSubtitleTracks(root)

			_, emptyStatErr := os.Stat(emptyDir)
			if !os.IsNotExist(emptyStatErr) {
				t.Fatal("expected empty track dir removed")
			}
			_, badStatErr := os.Stat(filepath.Join(subtitles, "bad.vtt"))
			if !os.IsNotExist(badStatErr) {
				t.Fatal("expected cue-less flat vtt removed")
			}
			_, validStatErr := os.Stat(validDir)
			if validStatErr != nil {
				t.Fatalf("valid track should remain: %v", validStatErr)
			}
		},
	)
}

func TestEnsureTrackDirSubtitlePlaylist_BackfillsMissingPlaylist(t *testing.T) {
	t.Parallel()

	allure.Test(t, "track dir with vtt but no playlist gets one", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		trackDir := filepath.Join(root, "subtitles", "eng")
		err := os.MkdirAll(trackDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(
			filepath.Join(trackDir, subtitleSegmentName),
			[]byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n"),
			0o600,
		)
		if err != nil {
			t.Fatalf("write vtt: %v", err)
		}

		err = ensureSubtitleMediaPlaylists(root, 42)
		if err != nil {
			t.Fatalf("ensure playlists: %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(trackDir, "playlist.m3u8")) //nolint:gosec // temp
		if err != nil {
			t.Fatalf("read playlist: %v", err)
		}
		if !strings.Contains(string(raw), subtitleSegmentName) {
			t.Fatalf("unexpected playlist:\n%s", raw)
		}

		tracks := listPackagedSubtitleTracks(root)
		if len(tracks) != 1 || tracks[0].ID != "eng" {
			t.Fatalf("tracks: %+v", tracks)
		}
	})
}

func TestPackagedSubtitleTrackFromLegacyVTT_RequiresPlaylist(t *testing.T) {
	t.Parallel()

	allure.Test(t, "legacy flat listing needs an existing media playlist", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		subtitles := filepath.Join(root, "subtitles")
		err := os.MkdirAll(subtitles, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(
			filepath.Join(subtitles, "fra.vtt"),
			[]byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nBonjour\n"),
			0o600,
		)
		if err != nil {
			t.Fatalf("write vtt: %v", err)
		}

		_, found := packagedSubtitleTrackFromLegacyVTT(root, "fra.vtt", map[string]struct{}{})
		if found {
			t.Fatal("expected false without playlist wrapper")
		}

		err = writeSubtitleMediaPlaylist(
			filepath.Join(subtitles, "fra", "playlist.m3u8"),
			"../fra.vtt",
			5,
		)
		if err != nil {
			t.Fatalf("write playlist: %v", err)
		}

		track, found := packagedSubtitleTrackFromLegacyVTT(root, "fra.vtt", map[string]struct{}{})
		if !found || track.ID != "fra" {
			t.Fatalf("legacy track: %+v found=%v", track, found)
		}

		_, found = packagedSubtitleTrackFromLegacyVTT(
			root,
			"fra.vtt",
			map[string]struct{}{"fra": {}},
		)
		if found {
			t.Fatal("seen track should be skipped")
		}
	})
}

func TestSourceDurationForSubtitles_FallbackChain(t *testing.T) {
	t.Parallel()

	allure.Test(t, "duration prefers marker then meta then probe source", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		if got := sourceDurationForSubtitles(root, SourceInfo{}); got != 1 {
			t.Fatalf("empty fallback: %v", got)
		}
		if got := sourceDurationForSubtitles(root, SourceInfo{DurationSeconds: 9}); got != 9 {
			t.Fatalf("source fallback: %v", got)
		}

		err := WriteSourceMeta(
			root,
			SourceMeta{Height: 480, Segments: []float64{6}, DurationSeconds: 12},
		)
		if err != nil {
			t.Fatalf("write meta: %v", err)
		}
		if got := sourceDurationForSubtitles(root, SourceInfo{DurationSeconds: 9}); got != 12 {
			t.Fatalf("meta duration: %v", got)
		}

		err = WriteSourceDuration(root, 33)
		if err != nil {
			t.Fatalf("write duration: %v", err)
		}
		if got := sourceDurationForSubtitles(root, SourceInfo{}); got != 33 {
			t.Fatalf("duration marker: %v", got)
		}
	})
}
