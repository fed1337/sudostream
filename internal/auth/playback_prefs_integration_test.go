package auth_test

import (
	"context"
	"errors"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestService_PlaybackPreferences(t *testing.T) {
	allure.Test(t, "playback prefs round-trip via auth service", func(a *allure.Context) {
		t := a.T()
		ctx := context.Background()
		service, _ := newAuthIntegration(ctx, t)
		admin := seedAdminUser(ctx, t, service)

		empty, err := service.PlaybackAudioLanguages(ctx, admin.ID)
		if err != nil {
			t.Fatalf("get empty: %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("expected no langs, got %v", empty)
		}

		saved, err := service.UpdatePlaybackPreferences(ctx, admin.ID, auth.PlaybackPreferences{
			AudioLanguages: []string{" EN ", "ru"},
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if len(saved.AudioLanguages) != 2 || saved.AudioLanguages[0] != "en" {
			t.Fatalf("saved: %+v", saved.AudioLanguages)
		}

		langs, err := service.PlaybackAudioLanguages(ctx, admin.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if len(langs) != 2 || langs[1] != "ru" {
			t.Fatalf("reload: %v", langs)
		}

		_, err = service.UpdatePlaybackPreferences(ctx, admin.ID, auth.PlaybackPreferences{
			AudioLanguages: []string{"en", "ru", "de", "fr"},
		})
		if !errors.Is(err, auth.ErrInvalidAudioLanguagePrefs) {
			t.Fatalf("expected ErrInvalidAudioLanguagePrefs, got %v", err)
		}

		_, err = service.PlaybackAudioLanguages(ctx, "00000000-0000-0000-0000-000000000099")
		if err == nil {
			t.Fatal("expected error for missing user")
		}
	})
}
