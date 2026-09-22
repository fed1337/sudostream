package transcode

import (
	"context"
	"encoding/json"
	"fmt"
)

const transcodeSettingsKey = "transcode"

// RawSettingsKV reads and writes opaque JSON blobs in the shared settings table.
type RawSettingsKV interface {
	GetSettingValue(ctx context.Context, key string) ([]byte, error)
	SaveSettingValue(ctx context.Context, key string, value []byte) error
}

// KVSettingsStore persists TranscodeSettings under settings.key = "transcode".
type KVSettingsStore struct {
	kv RawSettingsKV
}

// NewKVSettingsStore wraps a settings-table KV backend.
func NewKVSettingsStore(kv RawSettingsKV) *KVSettingsStore {
	return &KVSettingsStore{kv: kv}
}

// GetTranscodeSettings returns stored settings or defaults when missing/invalid.
func (s *KVSettingsStore) GetTranscodeSettings(ctx context.Context) (TranscodeSettings, error) {
	if s == nil || s.kv == nil {
		return DefaultTranscodeSettings(), nil
	}

	raw, err := s.kv.GetSettingValue(ctx, transcodeSettingsKey)
	if err != nil {
		//nolint:nilerr // missing row is the normal first-run case
		return DefaultTranscodeSettings(), nil
	}

	var settings TranscodeSettings
	err = json.Unmarshal(raw, &settings)
	if err != nil {
		return TranscodeSettings{}, fmt.Errorf("decode transcode settings: %w", err)
	}

	normalized, err := NormalizeTranscodeSettings(settings)
	if err != nil {
		//nolint:nilerr // corrupt value falls back to safe defaults
		return DefaultTranscodeSettings(), nil
	}

	return normalized, nil
}

// SaveTranscodeSettings validates and upserts settings.
func (s *KVSettingsStore) SaveTranscodeSettings(
	ctx context.Context,
	settings TranscodeSettings,
) error {
	if s == nil || s.kv == nil {
		return ErrSettingsUnavailable
	}

	normalized, err := NormalizeTranscodeSettings(settings)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode transcode settings: %w", err)
	}

	err = s.kv.SaveSettingValue(ctx, transcodeSettingsKey, raw)
	if err != nil {
		return fmt.Errorf("save transcode settings: %w", err)
	}

	return nil
}
