package metadata

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

//nolint:funlen // exhaustive field→tag mapping assertions
func TestFileTagUpdates_MapsEditableFields(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"fileTagUpdates maps every editable patch key to ffmpeg tags",
		func(a *allure.Context) {
			t := a.T()
			title := testEpisodeName
			sortTitle := "Sort Name"
			originalTitle := "Orig Title"
			episodeTitle := "Ep Title"
			show := "Show Name"
			season := 2
			episode := 5
			year := 2020
			releaseDate := "2020-01-02"
			description := "Desc text"
			director := "Director One"
			actor := "Actor One"
			writer := "Writer One"
			producer := "Producer One"
			studio := "Studio Name"
			composer := "Composer Name"
			language := "en"
			country := "US"
			contentRating := "PG"
			imdbID := "tt123"
			tmdbID := "456"

			patch := map[string]*jsonValue{
				fieldTitle:         {raw: title},
				fieldSortTitle:     {raw: sortTitle},
				fieldOriginalTitle: {raw: originalTitle},
				fieldEpisodeTitle:  {raw: episodeTitle},
				fieldShow:          {raw: show},
				fieldSeason:        {raw: "2"},
				fieldEpisode:       {raw: "5"},
				fieldYear:          {raw: "2020"},
				fieldReleaseDate:   {raw: releaseDate},
				fieldDescription:   {raw: description},
				fieldGenres:        {raw: `["Sci-Fi","Drama"]`},
				fieldDirectors:     {raw: `["Director One"]`},
				fieldActors:        {raw: `["Actor One"]`},
				fieldWriters:       {raw: `["Writer One"]`},
				fieldProducers:     {raw: `["Producer One"]`},
				fieldStudio:        {raw: studio},
				fieldComposer:      {raw: composer},
				fieldLanguage:      {raw: language},
				fieldCountry:       {raw: country},
				fieldContentRating: {raw: contentRating},
				fieldIMDBID:        {raw: imdbID},
				fieldTMDBID:        {raw: tmdbID},
				fieldDurationSecs:  {raw: "120"}, // technical — ignored
				"unknown_field":    {raw: "x"},
			}

			fields := VideoFields{
				Title:         &title,
				SortTitle:     &sortTitle,
				OriginalTitle: &originalTitle,
				EpisodeTitle:  &episodeTitle,
				Show:          &show,
				Season:        &season,
				Episode:       &episode,
				Year:          &year,
				ReleaseDate:   &releaseDate,
				Description:   &description,
				Genres:        []string{"Sci-Fi", "Drama"},
				Directors:     []string{director},
				Actors:        []string{actor},
				Writers:       []string{writer},
				Producers:     []string{producer},
				Studio:        &studio,
				Composer:      &composer,
				Language:      &language,
				Country:       &country,
				ContentRating: &contentRating,
				IMDBID:        &imdbID,
				TMDBID:        &tmdbID,
			}

			tags := fileTagUpdates(patch, fields)
			want := map[string]string{
				fieldTitle:         title,
				"sort_name":        sortTitle,
				fieldOriginalTitle: originalTitle,
				fieldEpisodeTitle:  episodeTitle,
				fieldShow:          show,
				"season_number":    "2",
				"episode_id":       "5",
				"year":             "2020",
				fieldReleaseDate:   releaseDate,
				fieldDescription:   description,
				"genre":            "Sci-Fi, Drama",
				"director":         director,
				"actor":            actor,
				"writer":           writer,
				"producer":         producer,
				fieldStudio:        studio,
				fieldComposer:      composer,
				fieldLanguage:      language,
				fieldCountry:       country,
				fieldContentRating: contentRating,
				fieldIMDBID:        imdbID,
				fieldTMDBID:        tmdbID,
			}
			if len(tags) != len(want) {
				t.Fatalf("tag count: got %d want %d (%#v)", len(tags), len(want), tags)
			}
			for key, value := range want {
				if tags[key] != value {
					t.Fatalf("%s tag: got %q want %q", key, tags[key], value)
				}
			}
		},
	)
}

func TestFileTagUpdates_ClearsNilIntFields(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil int fields map to empty ffmpeg tags", func(a *allure.Context) {
		t := a.T()
		patch := map[string]*jsonValue{fieldSeason: {raw: ""}}
		tags := fileTagUpdates(patch, VideoFields{Season: nil})
		if tags["season_number"] != "" {
			t.Fatalf("expected empty season tag, got %q", tags["season_number"])
		}
	})
}

func TestFileTagUpdates_ClearsNilStringFields(t *testing.T) {
	t.Parallel()

	allure.Test(t, "nil string fields map to empty ffmpeg tags", func(a *allure.Context) {
		t := a.T()
		patch := map[string]*jsonValue{fieldTitle: {raw: ""}}
		tags := fileTagUpdates(patch, VideoFields{Title: nil})
		if tags[fieldTitle] != "" {
			t.Fatalf("expected empty title tag, got %q", tags[fieldTitle])
		}
	})
}

func TestWriteFileTags_EmptyTagsNoOp(t *testing.T) {
	t.Parallel()

	allure.Test(t, "WriteFileTags returns immediately for empty tag map", func(a *allure.Context) {
		t := a.T()
		err := WriteFileTags(context.Background(), filepath.Join(t.TempDir(), "clip.mkv"), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestWriteFileTags_FFmpegFailure(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"WriteFileTags wraps ffmpeg failure on non-media input",
		func(a *allure.Context) {
			t := a.T()
			_, lookErr := exec.LookPath("ffmpeg")
			if lookErr != nil {
				t.Skip("ffmpeg not installed")
			}

			source := filepath.Join(t.TempDir(), "not-a-video.mkv")
			err := os.WriteFile(source, []byte("not media"), 0o600)
			if err != nil {
				t.Fatalf("write source: %v", err)
			}

			err = WriteFileTags(context.Background(), source, map[string]string{fieldTitle: "X"})
			if err == nil {
				t.Fatal("expected ffmpeg failure")
			}
			if !errors.Is(err, ErrFileWriteFailed) {
				t.Fatalf("expected ErrFileWriteFailed wrap, got %v", err)
			}
		},
	)
}

func TestTempOutputPath_MissingDir(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"tempOutputPath fails when parent directory is missing",
		func(a *allure.Context) {
			t := a.T()
			missing := filepath.Join(t.TempDir(), "missing", "clip.mkv")
			_, err := tempOutputPath(missing)
			if err == nil {
				t.Fatal("expected error for missing parent dir")
			}
		},
	)
}

func TestTempOutputPath_ReservesFileBesideSource(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"tempOutputPath uses hidden tagwrite prefix beside source",
		func(a *allure.Context) {
			t := a.T()
			dir := t.TempDir()
			source := filepath.Join(dir, "episode.mkv")
			err := os.WriteFile(source, []byte("fake"), 0o600)
			if err != nil {
				t.Fatalf("write source: %v", err)
			}

			tempPath, err := tempOutputPath(source)
			if err != nil {
				t.Fatalf("tempOutputPath: %v", err)
			}
			if !strings.Contains(filepath.Base(tempPath), ".episode.tagwrite-") {
				t.Fatalf("unexpected temp name: %q", tempPath)
			}
			if filepath.Dir(tempPath) != dir {
				t.Fatalf("temp dir: got %q want %q", filepath.Dir(tempPath), dir)
			}
			if filepath.Ext(tempPath) != ".mkv" {
				t.Fatalf("temp ext: got %q", filepath.Ext(tempPath))
			}
		},
	)
}

func TestWriteFileTags_Integration(t *testing.T) {
	t.Parallel()

	allure.Test(t, "WriteFileTags remuxes a synthetic video with ffmpeg", func(a *allure.Context) {
		t := a.T()
		_, lookErr := exec.LookPath("ffmpeg")
		if lookErr != nil {
			t.Skip("ffmpeg not installed")
		}

		dir := t.TempDir()
		source := filepath.Join(dir, "clip.mkv")
		//nolint:gosec // test fixture under t.TempDir()
		cmd := exec.CommandContext(
			context.Background(),
			"ffmpeg",
			"-v", "error",
			"-f", "lavfi",
			"-i", "color=c=black:s=64x64:d=1",
			"-c:v", "libx264",
			"-y", source,
		)
		err := cmd.Run()
		if err != nil {
			t.Fatalf("create test video: %v", err)
		}

		err = WriteFileTags(context.Background(), source, map[string]string{"title": "Test Clip"})
		if err != nil {
			t.Fatalf("WriteFileTags: %v", err)
		}

		info, err := os.Stat(source)
		if err != nil || info.Size() == 0 {
			t.Fatal("expected rewritten source file")
		}
	})
}
