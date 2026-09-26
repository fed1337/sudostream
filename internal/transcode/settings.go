package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// HwAccel is the admin-selected hardware encoder preference.
type HwAccel string

const (
	// HwAccelOff forces software libx264.
	HwAccelOff HwAccel = "off"
	// HwAccelQSV prefers Intel Quick Sync (Intel GPU only).
	HwAccelQSV HwAccel = "qsv"
	// HwAccelVAAPI prefers VA-API (Intel or AMD GPU via /dev/dri).
	HwAccelVAAPI HwAccel = "vaapi"
	// HwAccelNVENC prefers NVIDIA NVENC.
	HwAccelNVENC HwAccel = "nvenc"
	// HwAccelRockchip prefers Rockchip RKMPP (arm64 hosts with /dev/mpp_service).
	HwAccelRockchip HwAccel = "rockchip"
)

// ToneMappingAlgorithm is the HDR→SDR tone-mapping operator.
type ToneMappingAlgorithm string

const (
	// ToneMappingBT2390 is ITU-R BT.2390 (default; Jellyfin-aligned).
	ToneMappingBT2390 ToneMappingAlgorithm = "bt2390"
	// ToneMappingHable is the Hable filmic operator.
	ToneMappingHable ToneMappingAlgorithm = "hable"
	// ToneMappingReinhard is the Reinhard operator.
	ToneMappingReinhard ToneMappingAlgorithm = "reinhard"
	// ToneMappingMobius is the Mobius operator.
	ToneMappingMobius ToneMappingAlgorithm = "mobius"
)

var (
	// ErrInvalidHwAccel is returned when a settings patch has an unknown hwAccel value.
	ErrInvalidHwAccel = errors.New("invalid hwAccel value")
	// ErrInvalidDownmix is returned when a settings patch has an unknown downmixAlgorithm.
	ErrInvalidDownmix = errors.New("invalid downmixAlgorithm value")
	// ErrInvalidDownmixBoost is returned when downmixBoost is outside 0.5–3.0.
	ErrInvalidDownmixBoost = errors.New("invalid downmixBoost value")
	// ErrInvalidToneMappingAlgorithm is returned for an unknown toneMappingAlgorithm.
	ErrInvalidToneMappingAlgorithm = errors.New("invalid toneMappingAlgorithm value")
	// ErrSettingsUnavailable is returned when the settings store is not configured.
	ErrSettingsUnavailable = errors.New("transcode settings store unavailable")
)

// TranscodeSettings is the admin-editable transcode preference document
// stored under settings.key = "transcode".
//
//nolint:revive // API/OpenAPI name; matches resource path /api/admin/transcode/settings.
type TranscodeSettings struct {
	HwAccel              HwAccel              `json:"hwAccel"`
	DownmixAlgorithm     DownmixAlgorithm     `json:"downmixAlgorithm"`
	DownmixBoost         float64              `json:"downmixBoost,omitempty"`
	ToneMappingEnabled   bool                 `json:"toneMappingEnabled"`
	ToneMappingAlgorithm ToneMappingAlgorithm `json:"toneMappingAlgorithm"`
}

// UnmarshalJSON defaults missing toneMappingEnabled to true (pre-FI-5 rows).
func (s *TranscodeSettings) UnmarshalJSON(data []byte) error {
	type rawSettings struct {
		HwAccel              HwAccel              `json:"hwAccel"`
		DownmixAlgorithm     DownmixAlgorithm     `json:"downmixAlgorithm"`
		DownmixBoost         float64              `json:"downmixBoost"`
		ToneMappingEnabled   *bool                `json:"toneMappingEnabled"`
		ToneMappingAlgorithm ToneMappingAlgorithm `json:"toneMappingAlgorithm"`
	}

	var raw rawSettings
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err //nolint:wrapcheck // passthrough decode error
	}

	s.HwAccel = raw.HwAccel
	s.DownmixAlgorithm = raw.DownmixAlgorithm
	s.DownmixBoost = raw.DownmixBoost
	s.ToneMappingAlgorithm = raw.ToneMappingAlgorithm
	if raw.ToneMappingEnabled == nil {
		s.ToneMappingEnabled = true
	} else {
		s.ToneMappingEnabled = *raw.ToneMappingEnabled
	}

	return nil
}

// SettingsStore loads and saves transcode settings.
type SettingsStore interface {
	GetTranscodeSettings(ctx context.Context) (TranscodeSettings, error)
	SaveTranscodeSettings(ctx context.Context, settings TranscodeSettings) error
}

// DefaultTranscodeSettings returns safe defaults (software encode, AC-4 downmix, tonemap on).
func DefaultTranscodeSettings() TranscodeSettings {
	return TranscodeSettings{
		HwAccel:              HwAccelOff,
		DownmixAlgorithm:     DownmixAC4,
		ToneMappingEnabled:   true,
		ToneMappingAlgorithm: ToneMappingBT2390,
	}
}

// ParseHwAccel validates and normalizes an hwAccel string.
func ParseHwAccel(raw string) (HwAccel, error) {
	switch HwAccel(strings.ToLower(strings.TrimSpace(raw))) {
	case HwAccelOff, "":
		return HwAccelOff, nil
	case HwAccelQSV:
		return HwAccelQSV, nil
	case HwAccelVAAPI:
		return HwAccelVAAPI, nil
	case HwAccelNVENC:
		return HwAccelNVENC, nil
	case HwAccelRockchip:
		return HwAccelRockchip, nil
	default:
		return "", ErrInvalidHwAccel
	}
}

// ParseDownmixAlgorithm validates and normalizes a downmixAlgorithm string.
func ParseDownmixAlgorithm(raw string) (DownmixAlgorithm, error) {
	switch DownmixAlgorithm(strings.ToLower(strings.TrimSpace(raw))) {
	case DownmixNone:
		return DownmixNone, nil
	case DownmixAC4, "":
		return DownmixAC4, nil
	case DownmixDave750:
		return DownmixDave750, nil
	case DownmixNightmodeDialogue:
		return DownmixNightmodeDialogue, nil
	case DownmixRFC7845:
		return DownmixRFC7845, nil
	default:
		return "", ErrInvalidDownmix
	}
}

// ParseDownmixBoost validates admin downmixBoost (0 means algorithm default).
func ParseDownmixBoost(raw float64, algo DownmixAlgorithm) (float64, error) {
	if raw == 0 {
		return DefaultDownmixBoost(algo), nil
	}
	if raw < minDownmixBoost || raw > maxDownmixBoost {
		return 0, ErrInvalidDownmixBoost
	}

	return raw, nil
}

// ParseToneMappingAlgorithm validates and normalizes a toneMappingAlgorithm string.
func ParseToneMappingAlgorithm(raw string) (ToneMappingAlgorithm, error) {
	switch ToneMappingAlgorithm(strings.ToLower(strings.TrimSpace(raw))) {
	case ToneMappingBT2390, "":
		return ToneMappingBT2390, nil
	case ToneMappingHable:
		return ToneMappingHable, nil
	case ToneMappingReinhard:
		return ToneMappingReinhard, nil
	case ToneMappingMobius:
		return ToneMappingMobius, nil
	default:
		return "", ErrInvalidToneMappingAlgorithm
	}
}

// NormalizeTranscodeSettings applies defaults and validates fields.
func NormalizeTranscodeSettings(settings TranscodeSettings) (TranscodeSettings, error) {
	hwAccel, err := ParseHwAccel(string(settings.HwAccel))
	if err != nil {
		return TranscodeSettings{}, err
	}

	downmix, err := ParseDownmixAlgorithm(string(settings.DownmixAlgorithm))
	if err != nil {
		return TranscodeSettings{}, err
	}

	boost, err := ParseDownmixBoost(settings.DownmixBoost, downmix)
	if err != nil {
		return TranscodeSettings{}, err
	}

	algo, err := ParseToneMappingAlgorithm(string(settings.ToneMappingAlgorithm))
	if err != nil {
		return TranscodeSettings{}, err
	}

	return TranscodeSettings{
		HwAccel:              hwAccel,
		DownmixAlgorithm:     downmix,
		DownmixBoost:         boost,
		ToneMappingEnabled:   settings.ToneMappingEnabled,
		ToneMappingAlgorithm: algo,
	}, nil
}
