package auth

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestNormalizePlaybackPreferences(t *testing.T) {
	t.Parallel()

	allure.Test(t, "accepts up to three languages", func(a *allure.Context) {
		t := a.T()
		got, err := NormalizePlaybackPreferences(PlaybackPreferences{
			AudioLanguages: []string{" EN ", "ru", "jpn"},
		})
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if len(got.AudioLanguages) != 3 || got.AudioLanguages[0] != "en" {
			t.Fatalf("got %+v", got.AudioLanguages)
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
}
