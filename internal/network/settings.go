// Package network holds admin-editable outbound proxy settings for provider HTTP clients.
package network

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const settingsKey = "network"

var (
	// ErrInvalidProxyURL is returned when proxyUrl has a bad scheme or cannot be parsed.
	ErrInvalidProxyURL = errors.New("invalid proxyUrl")
	// ErrSettingsUnavailable is returned when the settings store is not configured.
	ErrSettingsUnavailable = errors.New("network settings store unavailable")
)

// Settings is stored under settings.key = "network".
type Settings struct {
	ProxyURL         string `json:"proxyUrl"`
	ProxyUser        string `json:"proxyUser,omitempty"`
	ProxyPassword    string `json:"proxyPassword,omitempty"`
	HasProxyPassword bool   `json:"hasProxyPassword"`
	NoProxy          string `json:"noProxy"`
}

// SettingsStore loads and saves network settings.
type SettingsStore interface {
	GetSettings(ctx context.Context) (Settings, error)
	SaveSettings(ctx context.Context, settings Settings) error
}

// DefaultSettings returns empty proxy (env / direct).
func DefaultSettings() Settings {
	return Settings{}
}

// NormalizeSettings validates and trims fields. Password may be empty (caller merges keep-on-blank).
func NormalizeSettings(settings Settings) (Settings, error) {
	out := Settings{
		ProxyURL:      strings.TrimSpace(settings.ProxyURL),
		ProxyUser:     strings.TrimSpace(settings.ProxyUser),
		ProxyPassword: settings.ProxyPassword, // do not trim passwords
		NoProxy:       strings.TrimSpace(settings.NoProxy),
	}
	if out.ProxyURL == "" {
		out.ProxyUser = ""
		out.ProxyPassword = ""
		out.HasProxyPassword = false

		return out, nil
	}

	parsed, err := parseProxyURL(out.ProxyURL)
	if err != nil {
		return Settings{}, err
	}
	out.ProxyURL, out.ProxyUser, out.ProxyPassword = absorbProxyUserinfo(
		parsed,
		out.ProxyUser,
		out.ProxyPassword,
	)
	out.HasProxyPassword = out.ProxyPassword != ""

	return out, nil
}

func parseProxyURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: must be absolute http(s) or socks5(h) URL", ErrInvalidProxyURL)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", schemeSOCKS5H:
		return parsed, nil
	default:
		return nil, fmt.Errorf("%w: unsupported scheme %q", ErrInvalidProxyURL, parsed.Scheme)
	}
}

func absorbProxyUserinfo(parsed *url.URL, user, password string) (string, string, string) {
	if parsed.User == nil {
		return parsed.String(), user, password
	}
	if user == "" {
		user = parsed.User.Username()
	}
	if password == "" {
		if pass, ok := parsed.User.Password(); ok {
			password = pass
		}
	}
	parsed.User = nil

	return parsed.String(), user, password
}

// PublicView clears the password for API responses.
func PublicView(settings Settings) Settings {
	out := settings
	out.HasProxyPassword = settings.ProxyPassword != ""
	out.ProxyPassword = ""

	return out
}

// MergePassword keeps the stored password when the patch leaves it blank.
func MergePassword(patch, existing Settings) Settings {
	if patch.ProxyPassword == "" && existing.ProxyPassword != "" {
		patch.ProxyPassword = existing.ProxyPassword
	}

	return patch
}

// Scheme returns the lowercase proxy scheme, or empty when unset.
func (s Settings) Scheme() string {
	if strings.TrimSpace(s.ProxyURL) == "" {
		return ""
	}
	parsed, err := url.Parse(s.ProxyURL)
	if err != nil {
		return ""
	}

	return strings.ToLower(parsed.Scheme)
}

// IsSOCKS reports whether the configured proxy is SOCKS5/SOCKS5h.
func (s Settings) IsSOCKS() bool {
	switch s.Scheme() {
	case "socks5", schemeSOCKS5H:
		return true
	default:
		return false
	}
}
