package auth

import (
	"context"
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestNormalizePlaybackPreferences(t *testing.T) {
	t.Parallel()

	allure.Test(t, "accepts up to three languages", func(a *allure.Context) {
		t := a.T()
		got, err := NormalizePlaybackPreferences(PlaybackPreferences{
			AudioLanguages:  []string{" EN ", "ru", "jpn"},
			AutoSkipIntro:   true,
			AutoNextEpisode: true,
		})
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if len(got.AudioLanguages) != 3 || got.AudioLanguages[0] != "en" {
			t.Fatalf("got %+v", got.AudioLanguages)
		}
		if !got.AutoSkipIntro || !got.AutoNextEpisode {
			t.Fatalf("bools not preserved: %+v", got)
		}
	})

	allure.Test(t, "rejects more than three languages", func(a *allure.Context) {
		t := a.T()
		_, err := NormalizePlaybackPreferences(PlaybackPreferences{
			AudioLanguages: []string{"en", "ru", "de", "fr"},
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	allure.Test(t, "empty list normalizes to empty slice", func(a *allure.Context) {
		t := a.T()
		got, err := NormalizePlaybackPreferences(PlaybackPreferences{
			AudioLanguages: []string{"  ", ""},
		})
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if len(got.AudioLanguages) != 0 {
			t.Fatalf("got %+v", got.AudioLanguages)
		}
	})
}

func TestPlaybackPreferences_StoreUnavailable(t *testing.T) {
	t.Parallel()

	allure.Test(t, "update fails when auth store is unset", func(a *allure.Context) {
		t := a.T()
		service := &Service{}
		_, err := service.UpdatePlaybackPreferences(context.Background(), "user-id", PlaybackPreferences{
			AudioLanguages: []string{"en"},
		})
		if !errors.Is(err, errPlaybackStoreUnavailable) {
			t.Fatalf("expected errPlaybackStoreUnavailable, got %v", err)
		}
	})

	allure.Test(t, "playback languages empty when store is unset", func(a *allure.Context) {
		t := a.T()
		service := &Service{}
		langs, err := service.PlaybackAudioLanguages(context.Background(), "user-id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(langs) != 0 {
			t.Fatalf("expected empty langs, got %v", langs)
		}
	})

	allure.Test(t, "get prefs returns defaults when store is unset", func(a *allure.Context) {
		t := a.T()
		service := &Service{}
		prefs, err := service.GetPlaybackPreferences(context.Background(), "user-id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prefs.AutoSkipIntro || prefs.AutoNextEpisode || len(prefs.AudioLanguages) != 0 {
			t.Fatalf("expected empty defaults, got %+v", prefs)
		}
	})
}
