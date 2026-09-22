package metadata

import (
	"errors"
	"sudoStream/internal/access"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestDisplayName_FilmPrefersTitle(t *testing.T) {
	t.Parallel()

	allure.Test(t, "film display uses effective title", func(a *allure.Context) {
		t := a.T()
		title := testFilmTitle
		name := DisplayName(
			access.LibraryTypeFilm,
			"movies/Inception.mkv",
			VideoFields{Title: &title},
			StoredOverride{},
		)

		if name != "Inception" {
			t.Fatalf("expected Inception, got %q", name)
		}
	})
}

func TestDisplayName_SeriesFormatsEpisode(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"series display combines show season episode and title",
		func(a *allure.Context) {
			t := a.T()
			show := testShowName
			episodeTitle := testEpisodeName
			season := 1
			episode := 2

			name := DisplayName(
				access.LibraryTypeSeries,
				"series/Demo/S01E02.mkv",
				VideoFields{
					Show:         &show,
					Season:       &season,
					Episode:      &episode,
					EpisodeTitle: &episodeTitle,
				},
				StoredOverride{},
			)

			if name != "Demo Show — S01E02 — Pilot" {
				t.Fatalf("unexpected display name: %q", name)
			}
		},
	)
}

func TestDisplayName_OtherUsesFilename(t *testing.T) {
	t.Parallel()

	allure.Test(t, "other library ignores metadata for labels", func(a *allure.Context) {
		t := a.T()
		title := "Ignored"
		name := DisplayName(
			access.LibraryTypeOther,
			"files/clip/S01E02.mkv",
			VideoFields{Title: &title},
			StoredOverride{},
		)

		if name != "S01E02" {
			t.Fatalf("expected basename, got %q", name)
		}
	})
}

func TestEffectiveFields_OverrideWins(t *testing.T) {
	t.Parallel()

	allure.Test(t, "override field replaces original", func(a *allure.Context) {
		t := a.T()
		original := "Original"
		override := testOverrideTitle

		effective := EffectiveFields(
			access.LibraryTypeFilm,
			"movies/file.mkv",
			VideoFields{Title: &original},
			StoredOverride{VideoFields: VideoFields{Title: &override}},
		)

		if effective.Title == nil || *effective.Title != "Override" {
			t.Fatalf("expected override title, got %#v", effective.Title)
		}
	})
}

func TestEffectiveFields_NullOverrideFallsBackToOriginal(t *testing.T) {
	t.Parallel()

	allure.Test(t, "empty override key falls back to file/original title", func(a *allure.Context) {
		t := a.T()
		original := "FromFile"
		effective := EffectiveFields(
			access.LibraryTypeFilm,
			"movies/file.mkv",
			VideoFields{Title: &original},
			StoredOverride{},
		)

		if effective.Title == nil || *effective.Title != original {
			t.Fatalf("expected original title, got %#v", effective.Title)
		}
	})
}

func TestApplyPatchFields_ClearsWithNull(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"null patch value removes override field so merge can fall back to file",
		func(a *allure.Context) {
			t := a.T()
			title := "Keep until cleared"
			current := StoredOverride{VideoFields: VideoFields{Title: &title}}

			updated, err := ApplyPatchFields(current, map[string]*jsonValue{
				fieldTitle: {raw: nil},
			})
			if err != nil {
				t.Fatalf("apply patch: %v", err)
			}

			if updated.Title != nil {
				t.Fatalf("expected cleared title, got %#v", updated.Title)
			}
		},
	)
}

func TestApplyPatchFields_RejectsInvalidTypes(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"invalid patch value types return ErrInvalidPatchValue",
		func(a *allure.Context) {
			t := a.T()
			_, err := ApplyPatchFields(StoredOverride{}, map[string]*jsonValue{
				fieldSeason: {raw: true},
			})
			if !errors.Is(err, ErrInvalidPatchValue) {
				t.Fatalf("expected ErrInvalidPatchValue, got %v", err)
			}
		},
	)
}

func TestUsesMetadataForDisplay_LibraryTypes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "film and series use metadata labels", func(a *allure.Context) {
		t := a.T()
		if !UsesMetadataForDisplay(access.LibraryTypeFilm) {
			t.Fatal("expected film to use metadata")
		}
		if !UsesMetadataForDisplay(access.LibraryTypeSeries) {
			t.Fatal("expected series to use metadata")
		}
		if UsesMetadataForDisplay(access.LibraryTypeOther) {
			t.Fatal("expected other library to skip metadata labels")
		}
	})
}

func TestEffectiveFields_AppliesFilenameHeuristics(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"series effective fields parse season episode from filename",
		func(a *allure.Context) {
			t := a.T()
			effective := EffectiveFields(
				access.LibraryTypeSeries,
				"series/Demo/S01E03.mkv",
				VideoFields{},
				StoredOverride{},
			)

			if effective.Season == nil || *effective.Season != 1 {
				t.Fatalf("unexpected season: %#v", effective.Season)
			}
			if effective.Episode == nil || *effective.Episode != 3 {
				t.Fatalf("unexpected episode: %#v", effective.Episode)
			}
			if effective.Title != nil {
				t.Fatalf("expected no title invent from basename, got %#v", effective.Title)
			}
		},
	)
}

func TestEffectiveFields_FileTagsWhenFilenameUnparsed(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"embedded file tags fill season episode title when filename has no markers",
		func(a *allure.Context) {
			t := a.T()
			season := 1
			episode := 7
			show := "Tagged Show"
			episodeTitle := "Tagged Episode"
			effective := EffectiveFields(
				access.LibraryTypeSeries,
				"series/Tagged Show/clip.mkv",
				VideoFields{
					Show:         &show,
					Season:       &season,
					Episode:      &episode,
					EpisodeTitle: &episodeTitle,
				},
				StoredOverride{},
			)

			if effective.Episode == nil || *effective.Episode != 7 {
				t.Fatalf("want file episode 7, got %#v", effective.Episode)
			}
			if effective.EpisodeTitle == nil || *effective.EpisodeTitle != episodeTitle {
				t.Fatalf("want file episode title, got %#v", effective.EpisodeTitle)
			}
			if effective.Show == nil || *effective.Show != show {
				t.Fatalf("want file show, got %#v", effective.Show)
			}
		},
	)
}

func TestEffectiveFields_PathParseOverridesFileEpisode(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"parsed filename episode wins over embedded file episode tags",
		func(a *allure.Context) {
			t := a.T()
			fileEpisode := 1
			fileTitle := "Wrong"
			effective := EffectiveFields(
				access.LibraryTypeSeries,
				"Chilly Willy - 025 - South Pole Pals [SATRip-Rus].avi",
				VideoFields{Episode: &fileEpisode, EpisodeTitle: &fileTitle},
				StoredOverride{},
			)

			if effective.Episode == nil || *effective.Episode != 25 {
				t.Fatalf("want parsed episode 25, got %#v", effective.Episode)
			}
			if effective.EpisodeTitle == nil || *effective.EpisodeTitle != "South Pole Pals" {
				t.Fatalf("want parsed episode title, got %#v", effective.EpisodeTitle)
			}
		},
	)
}

func TestDisplayName_FilmFallsBackToOriginalTitleThenBasename(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"film uses original title then basename when title missing",
		func(a *allure.Context) {
			t := a.T()
			originalTitle := "Le Film"
			withOriginal := DisplayName(
				access.LibraryTypeFilm,
				"movies/Le.Film.2020.mkv",
				VideoFields{OriginalTitle: &originalTitle},
				StoredOverride{},
			)
			if withOriginal != "Le Film" {
				t.Fatalf("expected original title, got %q", withOriginal)
			}
		},
	)
}

func TestDisplayName_MusicAndPhotosUseBasename(t *testing.T) {
	t.Parallel()

	allure.Test(t, "music and photos ignore metadata labels", func(a *allure.Context) {
		t := a.T()
		title := "Tagged"
		for _, libType := range []access.LibraryType{
			access.LibraryTypeMusic,
			access.LibraryTypePhotos,
		} {
			name := DisplayName(
				libType,
				"lib/track.mkv",
				VideoFields{Title: &title},
				StoredOverride{},
			)
			if name != "track" {
				t.Fatalf("%s: expected basename, got %q", libType, name)
			}
		}
	})
}

func TestApplyPatchFields_SetsAllFieldTypes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "patch assigns editable string, int, and slice fields", func(a *allure.Context) {
		t := a.T()
		patch := map[string]*jsonValue{
			fieldTitle:         {raw: "Movie"},
			fieldSortTitle:     {raw: "Sort"},
			fieldOriginalTitle: {raw: "Original"},
			fieldEpisodeTitle:  {raw: "Episode"},
			fieldShow:          {raw: testShowName},
			fieldReleaseDate:   {raw: "2020-01-01"},
			fieldDescription:   {raw: "Desc"},
			fieldStudio:        {raw: "Studio"},
			fieldComposer:      {raw: "Composer"},
			fieldLanguage:      {raw: "en"},
			fieldCountry:       {raw: "US"},
			fieldContentRating: {raw: "PG"},
			fieldIMDBID:        {raw: "tt1"},
			fieldTMDBID:        {raw: "tm1"},
			fieldSeason:        {raw: float64(2)},
			fieldEpisode:       {raw: float64(5)},
			fieldYear:          {raw: float64(2020)},
			fieldGenres:        {raw: []any{"drama", "sci-fi"}},
			fieldDirectors:     {raw: []any{"Dir"}},
			fieldActors:        {raw: []any{"Actor"}},
			fieldWriters:       {raw: []any{"Writer"}},
			fieldProducers:     {raw: []any{"Producer"}},
		}

		updated, err := ApplyPatchFields(StoredOverride{}, patch)
		if err != nil {
			t.Fatalf("apply patch: %v", err)
		}

		if updated.Title == nil || *updated.Title != "Movie" {
			t.Fatalf("unexpected title: %#v", updated.Title)
		}
		if updated.Show == nil || *updated.Show != testShowName {
			t.Fatalf("unexpected show: %#v", updated.Show)
		}
		if updated.Season == nil || *updated.Season != 2 {
			t.Fatalf("unexpected season: %#v", updated.Season)
		}
		if len(updated.Genres) != 2 {
			t.Fatalf("unexpected genres: %#v", updated.Genres)
		}
	})
}

func TestApplyPatchFields_ClearsAllFields(t *testing.T) {
	t.Parallel()

	allure.Test(t, "null patch clears every editable populated field", func(a *allure.Context) {
		t := a.T()
		value := "x"
		number := 1
		current := StoredOverride{VideoFields: VideoFields{
			Title: &value, SortTitle: &value, OriginalTitle: &value, EpisodeTitle: &value,
			Show: &value, Season: &number, Episode: &number, Year: &number,
			ReleaseDate: &value, Description: &value, Genres: []string{"a"},
			Directors: []string{"a"}, Actors: []string{"a"}, Writers: []string{"a"},
			Producers: []string{"a"}, Studio: &value,
			Composer: &value, Language: &value, Country: &value, ContentRating: &value,
			IMDBID: &value, TMDBID: &value,
		}}

		patch := make(map[string]*jsonValue, len(EditableFieldKeys))
		for _, key := range EditableFieldKeys {
			patch[key] = &jsonValue{raw: nil}
		}

		updated, err := ApplyPatchFields(current, patch)
		if err != nil {
			t.Fatalf("apply patch: %v", err)
		}

		if updated.Title != nil || updated.Season != nil {
			t.Fatalf("expected scalar fields cleared: %#v", updated.VideoFields)
		}
		if updated.Genres != nil || updated.Directors != nil {
			t.Fatalf("expected slice fields cleared: %#v", updated.VideoFields)
		}
	})
}

func TestApplyPatchFields_CoercesAndRejectsValues(t *testing.T) {
	t.Parallel()

	allure.Test(t, "patch coerces numeric strings and rejects bad types", func(a *allure.Context) {
		t := a.T()
		coerced, err := ApplyPatchFields(StoredOverride{}, map[string]*jsonValue{
			fieldSeason: {raw: "3"},
			fieldTitle:  {raw: float64(7)},
		})
		if err != nil {
			t.Fatalf("apply coercions: %v", err)
		}
		if coerced.Season == nil || *coerced.Season != 3 {
			t.Fatalf("expected season coerced from string, got %#v", coerced.Season)
		}
		if coerced.Title == nil || *coerced.Title != "7" {
			t.Fatalf("expected numeric title coerced to string, got %#v", coerced.Title)
		}

		_, err = ApplyPatchFields(StoredOverride{}, map[string]*jsonValue{
			fieldSeason: {raw: "not-a-number"},
		})
		if !errors.Is(err, ErrInvalidPatchValue) {
			t.Fatalf("expected invalid int string to fail, got %v", err)
		}

		_, err = ApplyPatchFields(StoredOverride{}, map[string]*jsonValue{
			fieldGenres: {raw: 42},
		})
		if !errors.Is(err, ErrInvalidPatchValue) {
			t.Fatalf("expected invalid slice type to fail, got %v", err)
		}

		_, err = ApplyPatchFields(StoredOverride{}, map[string]*jsonValue{
			fieldTitle: {raw: []any{"x"}},
		})
		if !errors.Is(err, ErrInvalidPatchValue) {
			t.Fatalf("expected invalid string type to fail, got %v", err)
		}

		_, err = ApplyPatchFields(StoredOverride{}, map[string]*jsonValue{
			fieldBitrate: {raw: float64(128000)},
		})
		if !errors.Is(err, ErrInvalidPatchValue) {
			t.Fatalf("expected technical field to fail, got %v", err)
		}
	})
}

func TestApplyPatchFields_IgnoresUnknownAndNilEntries(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"unknown keys are rejected; nil map values clear fields",
		func(a *allure.Context) {
			t := a.T()
			_, err := ApplyPatchFields(StoredOverride{}, map[string]*jsonValue{
				"unknown_field": {raw: "value"},
			})
			if !errors.Is(err, ErrInvalidPatchValue) {
				t.Fatalf("expected unknown field to fail, got %v", err)
			}

			title := testKeepTitle
			updated, err := ApplyPatchFields(
				StoredOverride{VideoFields: VideoFields{Title: &title}},
				map[string]*jsonValue{fieldTitle: nil},
			)
			if err != nil {
				t.Fatalf("nil entry: %v", err)
			}
			if updated.Title != nil {
				t.Fatalf("expected title cleared, got %#v", updated.Title)
			}
		},
	)
}

func TestClearOverrideKeys_RemovesPatchedEditableKeys(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"clear non-null written keys; keep override for null file clears",
		func(a *allure.Context) {
			t := a.T()
			title := testKeepTitle
			show := testShowName
			override := StoredOverride{
				VideoFields: VideoFields{Title: &title, Show: &show},
			}
			cleared := ClearOverrideKeys(override, map[string]*jsonValue{
				fieldTitle: {raw: "X"},
				fieldShow:  nil,
			})
			if cleared.Title != nil {
				t.Fatalf("expected title cleared after non-null file write, got %#v", cleared.Title)
			}
			if cleared.Show == nil || *cleared.Show != testShowName {
				t.Fatalf("expected show kept after null file clear, got %#v", cleared.Show)
			}
		},
	)
}

func TestClearSemantics_FileAndOverride(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"empty DB falls back to file; empty file keeps override; both clears empty",
		func(a *allure.Context) {
			t := a.T()
			fileTitle := "FromFile"
			overrideTitle := "FromOverride"
			original := VideoFields{Title: &fileTitle}
			override := StoredOverride{VideoFields: VideoFields{Title: &overrideTitle}}

			afterDB, err := ApplyPatchFields(override, map[string]*jsonValue{fieldTitle: nil})
			if err != nil {
				t.Fatalf("clear override: %v", err)
			}
			effectiveA := EffectiveFields(access.LibraryTypeFilm, "movies/x.mkv", original, afterDB)
			if effectiveA.Title == nil || *effectiveA.Title != fileTitle {
				t.Fatalf("a: expected file title, got %#v", effectiveA.Title)
			}

			both := StoredOverride{VideoFields: VideoFields{Title: &overrideTitle}}
			afterFileKeep := ClearOverrideKeys(both, map[string]*jsonValue{fieldTitle: nil})
			clearedOriginal := VideoFields{}
			effectiveB := EffectiveFields(
				access.LibraryTypeFilm,
				"movies/x.mkv",
				clearedOriginal,
				afterFileKeep,
			)
			if effectiveB.Title == nil || *effectiveB.Title != overrideTitle {
				t.Fatalf("b: expected override title, got %#v", effectiveB.Title)
			}

			effectiveC := EffectiveFields(
				access.LibraryTypeFilm,
				"movies/x.mkv",
				clearedOriginal,
				afterDB,
			)
			if effectiveC.Title == nil || *effectiveC.Title != "x" {
				t.Fatalf("c: expected heuristic title x, got %#v", effectiveC.Title)
			}
		},
	)
}
