// Package dlna exposes a LAN-only UPnP AV MediaServer (FI-9).
package dlna

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	settingsKey          = "dlna"
	defaultHTTPPort      = 8200
	defaultSSDPMulticast = "239.255.255.250:1900" // UPnP SSDP IANA (not admin-configurable)
	streamTokenTTLSec    = 24 * 60 * 60
	ssdpMaxAgeSec        = 1800
	browsePageSize       = 100
)

var (
	// ErrSettingsUnavailable is returned when the settings store is not configured.
	ErrSettingsUnavailable = errors.New("dlna settings store unavailable")
	// ErrTVUserRequired is returned when enabling DLNA without a TV principal.
	ErrTVUserRequired = errors.New("dlna tv userId is required when enabled")
	// ErrInvalidTVUser is returned when userId is not a tv-role account.
	ErrInvalidTVUser = errors.New("dlna userId must be a tv role account")
	// ErrNotEnabled is returned when DLNA is off.
	ErrNotEnabled = errors.New("dlna is disabled")
	// ErrInvalidToken is returned when a signed stream URL fails verification.
	ErrInvalidToken = errors.New("invalid stream token")
	// ErrUnknownObjectID is returned for malformed ContentDirectory object ids.
	ErrUnknownObjectID = errors.New("unknown object id")
	// ErrNoLANIPv4 is returned when no suitable LAN address is found for LOCATION.
	ErrNoLANIPv4 = errors.New("no LAN IPv4 found")
	// ErrReadDenied is returned when the TV principal cannot read a media path.
	ErrReadDenied = errors.New("read denied")
	// ErrUnsupportedBrowseFlag is returned for non-DirectChildren Browse.
	ErrUnsupportedBrowseFlag = errors.New("unsupported BrowseFlag")
	// ErrUnknownSOAPAction is returned for unimplemented SOAP actions.
	ErrUnknownSOAPAction = errors.New("unknown SOAP action")
)

// Settings is stored under settings.key = "dlna".
type Settings struct {
	Enabled bool   `json:"enabled"`
	UserID  string `json:"userId"`
	// UDN is a stable uuid:… for SSDP (generated on first save when empty).
	UDN string `json:"udn,omitempty"`
}

// SettingsStore loads and saves DLNA settings.
type SettingsStore interface {
	GetSettings(ctx context.Context) (Settings, error)
	SaveSettings(ctx context.Context, settings Settings) error
}

// RawSettingsKV reads and writes opaque JSON blobs in the shared settings table.
type RawSettingsKV interface {
	GetSettingValue(ctx context.Context, key string) ([]byte, error)
	SaveSettingValue(ctx context.Context, key string, value []byte) error
}

// KVSettingsStore persists Settings under settings.key = "dlna".
type KVSettingsStore struct {
	kv RawSettingsKV
}

// NewKVSettingsStore wraps a settings-table KV backend.
func NewKVSettingsStore(kv RawSettingsKV) *KVSettingsStore {
	return &KVSettingsStore{kv: kv}
}

// DefaultSettings returns DLNA off with no TV principal.
func DefaultSettings() Settings {
	return Settings{}
}

// DefaultHTTPPort returns the eng default DLNA listen port.
func DefaultHTTPPort() int {
	return defaultHTTPPort
}

// DefaultSSDPMulticast returns the UPnP SSDP multicast address (host:port).
func DefaultSSDPMulticast() string {
	return defaultSSDPMulticast
}

// NormalizeSettings validates admin input. Caller must verify UserID is RoleTV when Enabled.
func NormalizeSettings(settings Settings) (Settings, error) {
	out := Settings{
		Enabled: settings.Enabled,
		UserID:  strings.TrimSpace(settings.UserID),
		UDN:     strings.TrimSpace(settings.UDN),
	}
	if out.Enabled && out.UserID == "" {
		return Settings{}, ErrTVUserRequired
	}

	return out, nil
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
		return Settings{}, fmt.Errorf("decode dlna settings: %w", err)
	}

	return settings, nil
}

// SaveSettings validates and upserts settings.
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
		return fmt.Errorf("encode dlna settings: %w", err)
	}

	err = s.kv.SaveSettingValue(ctx, settingsKey, raw)
	if err != nil {
		return fmt.Errorf("save dlna settings: %w", err)
	}

	return nil
}
