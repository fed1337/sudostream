package network

import (
	"context"
	"encoding/json"
	"fmt"
)

// RawSettingsKV reads and writes opaque JSON blobs in the shared settings table.
type RawSettingsKV interface {
	GetSettingValue(ctx context.Context, key string) ([]byte, error)
	SaveSettingValue(ctx context.Context, key string, value []byte) error
}

// KVSettingsStore persists Settings under settings.key = "network".
type KVSettingsStore struct {
	kv RawSettingsKV
}

// NewKVSettingsStore wraps a settings-table KV backend.
func NewKVSettingsStore(kv RawSettingsKV) *KVSettingsStore {
	return &KVSettingsStore{kv: kv}
}

// GetSettings returns stored settings or defaults when missing/invalid.
func (s *KVSettingsStore) GetSettings(ctx context.Context) (Settings, error) {
	if s == nil || s.kv == nil {
		return DefaultSettings(), nil
	}

	raw, err := s.kv.GetSettingValue(ctx, settingsKey)
	if err != nil {
		//nolint:nilerr // missing row is the normal first-run case
		return DefaultSettings(), nil
	}

	var settings Settings
	err = json.Unmarshal(raw, &settings)
	if err != nil {
		return Settings{}, fmt.Errorf("decode network settings: %w", err)
	}

	normalized, err := NormalizeSettings(settings)
	if err != nil {
		//nolint:nilerr // corrupt value falls back to safe defaults
		return DefaultSettings(), nil
	}

	return normalized, nil
}

// SaveSettings validates and upserts settings (password must already be merged).
func (s *KVSettingsStore) SaveSettings(ctx context.Context, settings Settings) error {
	if s == nil || s.kv == nil {
		return ErrSettingsUnavailable
	}

	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return err
	}

	raw, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode network settings: %w", err)
	}

	err = s.kv.SaveSettingValue(ctx, settingsKey, raw)
	if err != nil {
		return fmt.Errorf("save network settings: %w", err)
	}

	return nil
}
