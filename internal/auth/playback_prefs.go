package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const maxAudioLanguagePrefs = 3

var (
	// ErrInvalidAudioLanguagePrefs is returned when more than three languages are supplied.
	ErrInvalidAudioLanguagePrefs = errors.New("audio language list exceeds maximum of 3")
	errPlaybackStoreUnavailable  = errors.New("auth store unavailable")
)

// PlaybackPreferences holds per-user playback defaults (E-28).
type PlaybackPreferences struct {
	AudioLanguages []string `json:"audioLanguages"`
}

// PlaybackAudioLanguages returns normalized user audio language priority (may be empty).
func (s *Service) PlaybackAudioLanguages(ctx context.Context, userID string) ([]string, error) {
	if s.store == nil {
		return nil, nil
	}

	prefs, err := s.store.GetPlaybackPreferences(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get playback preferences: %w", err)
	}

	return prefs.AudioLanguages, nil
}

// UpdatePlaybackPreferences stores up to three ordered audio language codes.
func (s *Service) UpdatePlaybackPreferences(
	ctx context.Context,
	userID string,
	prefs PlaybackPreferences,
) (PlaybackPreferences, error) {
	normalized, err := NormalizePlaybackPreferences(prefs)
	if err != nil {
		return PlaybackPreferences{}, err
	}

	if s.store == nil {
		return PlaybackPreferences{}, errPlaybackStoreUnavailable
	}

	err = s.store.SavePlaybackPreferences(ctx, userID, normalized)
	if err != nil {
		return PlaybackPreferences{}, fmt.Errorf("save playback preferences: %w", err)
	}

	return normalized, nil
}

// NormalizePlaybackPreferences validates and normalizes audio language codes.
func NormalizePlaybackPreferences(prefs PlaybackPreferences) (PlaybackPreferences, error) {
	langs := make([]string, 0, len(prefs.AudioLanguages))
	for _, raw := range prefs.AudioLanguages {
		lang := strings.ToLower(strings.TrimSpace(raw))
		if lang == "" {
			continue
		}
		langs = append(langs, lang)
		if len(langs) > maxAudioLanguagePrefs {
			return PlaybackPreferences{}, ErrInvalidAudioLanguagePrefs
		}
	}

	return PlaybackPreferences{AudioLanguages: langs}, nil
}
