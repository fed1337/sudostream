package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

var (
	errSettingsKVMissing = errors.New("missing")
	errSettingsKVDown    = errors.New("db down")
)

type memorySettingsKV struct {
	values map[string][]byte
	getErr error
	setErr error
}

func (m *memorySettingsKV) GetSettingValue(_ context.Context, key string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.values == nil {
		return nil, errSettingsKVMissing
	}
	raw, found := m.values[key]
	if !found {
		return nil, errSettingsKVMissing
	}

	return raw, nil
}

func (m *memorySettingsKV) SaveSettingValue(_ context.Context, key string, value []byte) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.values == nil {
		m.values = map[string][]byte{}
	}
	m.values[key] = append([]byte(nil), value...)

	return nil
}

//nolint:cyclop // store branch matrix in one scenario
func TestKVSettingsStore_GetAndSave(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"KV settings store loads defaults and persists valid values",
		func(a *allure.Context) {
			t := a.T()
			store := NewKVSettingsStore(nil)
			got, err := store.GetTranscodeSettings(context.Background())
			if err != nil || got.HwAccel != HwAccelOff {
				t.Fatalf("nil kv get: %+v err=%v", got, err)
			}
			if !errors.Is(
				store.SaveTranscodeSettings(context.Background(), got),
				ErrSettingsUnavailable,
			) {
				t.Fatal("expected ErrSettingsUnavailable for nil kv")
			}

			backend := &memorySettingsKV{}
			store = NewKVSettingsStore(backend)

			got, err = store.GetTranscodeSettings(context.Background())
			if err != nil || got.HwAccel != HwAccelOff {
				t.Fatalf("missing row defaults: %+v err=%v", got, err)
			}

			err = store.SaveTranscodeSettings(
				context.Background(),
				TranscodeSettings{HwAccel: HwAccelVAAPI, ToneMappingEnabled: true},
			)
			if err != nil {
				t.Fatalf("save: %v", err)
			}

			got, err = store.GetTranscodeSettings(context.Background())
			if err != nil || got.HwAccel != HwAccelVAAPI || !got.ToneMappingEnabled ||
				got.ToneMappingAlgorithm != ToneMappingBT2390 {
				t.Fatalf("round-trip: %+v err=%v", got, err)
			}

			backend.values[transcodeSettingsKey] = []byte(`{`)
			_, err = store.GetTranscodeSettings(context.Background())
			if err == nil {
				t.Fatal("expected decode error for truncated json")
			}

			backend.values[transcodeSettingsKey] = []byte(`{"hwAccel":"auto"}`)
			got, err = store.GetTranscodeSettings(context.Background())
			if err != nil || got.HwAccel != HwAccelOff {
				t.Fatalf("invalid hwAccel should default: %+v err=%v", got, err)
			}

			err = store.SaveTranscodeSettings(
				context.Background(),
				TranscodeSettings{HwAccel: "auto"},
			)
			if !errors.Is(err, ErrInvalidHwAccel) {
				t.Fatalf("save invalid: %v", err)
			}

			backend.setErr = errSettingsKVDown
			err = store.SaveTranscodeSettings(
				context.Background(),
				DefaultTranscodeSettings(),
			)
			if err == nil {
				t.Fatal("expected save backend error")
			}

			raw, _ := json.Marshal(DefaultTranscodeSettings())
			backend.setErr = nil
			backend.values[transcodeSettingsKey] = raw
			got, err = store.GetTranscodeSettings(context.Background())
			if err != nil || got.HwAccel != HwAccelOff || !got.ToneMappingEnabled {
				t.Fatalf("valid stored settings: %+v err=%v", got, err)
			}

			// Legacy row without tonemap fields → defaults.
			backend.values[transcodeSettingsKey] = []byte(
				`{"hwAccel":"qsv","downmixAlgorithm":"ac4"}`,
			)
			got, err = store.GetTranscodeSettings(context.Background())
			if err != nil || got.HwAccel != HwAccelQSV || !got.ToneMappingEnabled ||
				got.ToneMappingAlgorithm != ToneMappingBT2390 {
				t.Fatalf("legacy row defaults: %+v err=%v", got, err)
			}
		},
	)
}
