package metadata

import (
	"sudoStream/internal/access"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestCatalogDenormFrom_SeriesCyrillicX(t *testing.T) {
	t.Parallel()

	allure.Test(t, "series denorm uses Cyrillic х season-episode parse", func(a *allure.Context) {
		t := a.T()
		path := "series/Melrose Place/season 1/Melrose Place 1х01 - Pilot.avi"
		got := CatalogDenormFrom(access.LibraryTypeSeries, path, VideoFields{}, StoredOverride{})
		if got.ShowKey != "melrose-place" {
			t.Fatalf("showKey: got %q", got.ShowKey)
		}
		if got.Season == nil || *got.Season != 1 {
			t.Fatalf("season: got %v", got.Season)
		}
		if got.Episode == nil || *got.Episode != 1 {
			t.Fatalf("episode: got %v", got.Episode)
		}
		if got.SortTitle == "" || got.DisplayName == "" {
			t.Fatalf("sort/display empty: %+v", got)
		}
	})
}

func TestCatalogDenormFrom_OverrideShowWins(t *testing.T) {
	t.Parallel()

	allure.Test(t, "override show name wins over path for catalog_show_key", func(a *allure.Context) {
		t := a.T()
		show := "Custom Show"
		got := CatalogDenormFrom(
			access.LibraryTypeSeries,
			"tv/Other Name/S01E02.mkv",
			VideoFields{},
			StoredOverride{VideoFields: VideoFields{Show: &show}},
		)
		if got.ShowKey != "custom-show" {
			t.Fatalf("showKey: got %q", got.ShowKey)
		}
		if got.SortTitle != "Custom Show" {
			t.Fatalf("sortTitle: got %q", got.SortTitle)
		}
	})
}

func TestCatalogDenormFrom_FilmTitleYear(t *testing.T) {
	t.Parallel()

	allure.Test(t, "film denorm fills sort title and year from path", func(a *allure.Context) {
		t := a.T()
		got := CatalogDenormFrom(
			access.LibraryTypeFilm,
			"movies/The Matrix (1999).mkv",
			VideoFields{},
			StoredOverride{},
		)
		if got.ShowKey != "" {
			t.Fatalf("film must not set showKey: %q", got.ShowKey)
		}
		if got.SortTitle == "" {
			t.Fatal("expected sort title")
		}
		if got.Year == nil || *got.Year != 1999 {
			t.Fatalf("year: got %v", got.Year)
		}
	})
}
