package transcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSidecarSubtitleCandidates_SeriesEpisodeAndSubsDir(t *testing.T) {
	t.Parallel()

	allure.Test(t, "series sidecars match by SxxExx and Subs/ folder", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		videoPath := filepath.Join(root, "My Show - S01E02 - Pilot.mkv")
		err := os.WriteFile(videoPath, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}

		err = os.MkdirAll(filepath.Join(root, "Subs"), 0o750)
		if err != nil {
			t.Fatalf("mkdir Subs: %v", err)
		}
		for _, name := range []string{
			filepath.Join(root, "My Show - S01E02 - Pilot.en.srt"),
			filepath.Join(root, "Subs", "S01E02.ru.srt"),
			filepath.Join(root, "Other Show S01E02.srt"),
			filepath.Join(root, "Other Show S01E03.srt"),
		} {
			err = os.WriteFile(name, []byte("sub"), 0o600)
			if err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}

		matches, err := sidecarSubtitleCandidates(videoPath)
		if err != nil {
			t.Fatalf("candidates: %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("want 2 series sidecars (not Other Show), got %v", matches)
		}
	})
}

func TestSidecarSubtitleCandidates_MatchesExactStem(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"sidecars match when basename stem equals video filename stem",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			mediaPath := filepath.Join(root, "movie.mp4")
			err := os.WriteFile(mediaPath, []byte("fake"), 0o600)
			if err != nil {
				t.Fatalf("write media: %v", err)
			}

			for _, name := range []string{"movie.srt", "movie.vtt", "movie.en.srt", "other.srt"} {
				err = os.WriteFile(filepath.Join(root, name), []byte("sub"), 0o600)
				if err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			matches, err := sidecarSubtitleCandidates(mediaPath)
			if err != nil {
				t.Fatalf("sidecar candidates: %v", err)
			}

			if len(matches) != 3 {
				t.Fatalf("expected movie.srt, movie.vtt, movie.en.srt, got %v", matches)
			}
		},
	)
}

func TestSidecarSubtitleLabel_UsesSuffixAfterStem(t *testing.T) {
	t.Parallel()

	allure.Test(t, "sidecar label uses suffix after video stem", func(a *allure.Context) {
		t := a.T()
		mediaPath := "/media/movie.mp4"
		if got := sidecarSubtitleLabel(mediaPath, "/media/movie.srt"); got != "srt" {
			t.Fatalf("label: got %q", got)
		}
	})
}

func TestSidecarSubtitleCandidates_MatchesCyrillicEpisodeMarker(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"sidecar stem matches cyrillic and latin episode markers",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			videoPath := filepath.Join(root, "Show 1х02 Title.avi")
			sidecarPath := filepath.Join(root, "Show 1x02 Title.srt")

			err := os.WriteFile(videoPath, []byte("fake"), 0o600)
			if err != nil {
				t.Fatalf("write video: %v", err)
			}
			err = os.WriteFile(sidecarPath, []byte("sub"), 0o600)
			if err != nil {
				t.Fatalf("write sidecar: %v", err)
			}

			matches, err := sidecarSubtitleCandidates(videoPath)
			if err != nil {
				t.Fatalf("sidecar candidates: %v", err)
			}
			if len(matches) != 1 {
				t.Fatalf("expected sidecar match across x/х, got %v", matches)
			}
		},
	)
}

func writeSubtitleTrackFixture(t *testing.T, root string) {
	t.Helper()

	subtitlesDir := filepath.Join(root, "subtitles", "eng")

	err := os.MkdirAll(subtitlesDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir subtitles: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(subtitlesDir, "track.vtt"),
		[]byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello\n"),
		0o600,
	)
	if err != nil {
		t.Fatalf("write vtt: %v", err)
	}

	err = writeSubtitleMediaPlaylist(
		filepath.Join(subtitlesDir, "playlist.m3u8"),
		subtitleSegmentName,
		120,
	)
	if err != nil {
		t.Fatalf("write subtitle playlist: %v", err)
	}
}

func TestBuildMasterPlaylist_DeclaresRenditionGroups(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"master advertises audio and subtitle groups alongside every ladder rung",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			writeSubtitleTrackFixture(t, root)

			meta := SourceMeta{
				Height:        720,
				Width:         1280,
				PackagingMode: PackagingRemux,
				AudioStreams: []AudioStream{
					{Index: 1, Label: testAudioLabelEnglish, Language: "eng"},
					{Index: 2, Label: "Japanese", Language: "jpn"},
				},
				Segments: []float64{6, 6},
			}

			body := string(BuildMasterPlaylist(meta, listPackagedSubtitleTracks(root)))

			if !strings.Contains(body, `TYPE=AUDIO`) || !strings.Contains(body, `AUDIO="aud"`) {
				t.Fatalf("expected audio rendition group, got:\n%s", body)
			}
			if !strings.Contains(body, `a1/playlist.m3u8`) {
				t.Fatalf("expected alternate audio playlist URI, got:\n%s", body)
			}
			if !strings.Contains(body, `TYPE=SUBTITLES`) ||
				!strings.Contains(body, `SUBTITLES="subs"`) {
				t.Fatalf("expected subtitle group, got:\n%s", body)
			}
			if !strings.Contains(body, "v720/playlist.m3u8") ||
				!strings.Contains(body, "v480/playlist.m3u8") {
				t.Fatalf("expected full ladder, got:\n%s", body)
			}
			if !strings.Contains(body, "RESOLUTION=1280x720") {
				t.Fatalf("expected source aspect ratio, got:\n%s", body)
			}
		},
	)
}
