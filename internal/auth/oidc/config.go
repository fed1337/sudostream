// Package oidc implements the OpenID Connect relying-party adapter for sudoStream.
package oidc

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	// CallbackPath is the fixed RP callback route under the API.
	CallbackPath = "/api/auth/oidc/callback"
	// StatusPath is the public non-secret OIDC status route.
	StatusPath = "/api/auth/oidc/status"
	// StartPath begins the authorization-code + PKCE flow.
	StartPath = "/api/auth/oidc/start"

	defaultScope         = "openid email profile groups"
	defaultUsernameClaim = "preferred_username"
)

var (
	errIssuerRequired       = errors.New("SUDOSTREAM_OIDC_ISSUER_URL is required when OIDC is enabled")
	errClientIDRequired     = errors.New("SUDOSTREAM_OIDC_CLIENT_ID is required when OIDC is enabled")
	errClientSecretRequired = errors.New("SUDOSTREAM_OIDC_CLIENT_SECRET is required when OIDC is enabled")
	errBaseURLRequired      = errors.New("SUDOSTREAM_BASE_URL is required when OIDC is enabled")
)

// Config holds env-only OIDC relying-party settings (SUDOSTREAM_OIDC_*).
type Config struct {
	Enabled                  bool
	IssuerURL                string
	IssuerName               string
	ClientID                 string
	ClientSecret             string
	Only                     bool
	AutoRedirect             bool
	AdminGroup               string
	UsernameClaim            string
	Scope                    string
	AutoRegister             bool
	MergeUsers               bool
	RequireEmailVerification bool
	InsecureSkipVerify       bool
	BaseURL                  string
}

// PublicStatus is the non-secret subset exposed to the frontend.
type PublicStatus struct {
	Enabled      bool   `json:"enabled"`
	IssuerName   string `json:"issuerName,omitempty"`
	Only         bool   `json:"only"`
	AutoRedirect bool   `json:"autoRedirect"`
}

// RedirectURI returns the absolute callback URL derived from BaseURL.
func (c Config) RedirectURI() string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")

	return base + CallbackPath
}

// Scopes returns the configured OAuth scopes (space-split).
func (c Config) Scopes() []string {
	raw := strings.TrimSpace(c.Scope)
	if raw == "" {
		raw = defaultScope
	}
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}

	return out
}

// UsernameClaimName returns the claim used for display/username fallbacks.
func (c Config) UsernameClaimName() string {
	claim := strings.TrimSpace(c.UsernameClaim)
	if claim == "" {
		return defaultUsernameClaim
	}

	return claim
}

// Public returns FE-safe status flags.
func (c Config) Public() PublicStatus {
	if !c.Enabled {
		return PublicStatus{}
	}

	name := strings.TrimSpace(c.IssuerName)
	if name == "" {
		name = "SSO"
	}

	return PublicStatus{
		Enabled:      true,
		IssuerName:   name,
		Only:         c.Only,
		AutoRedirect: c.AutoRedirect,
	}
}

// Validate checks required fields when Enabled.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.IssuerURL) == "" {
		return errIssuerRequired
	}
	_, err := url.ParseRequestURI(strings.TrimSpace(c.IssuerURL))
	if err != nil {
		return fmt.Errorf("SUDOSTREAM_OIDC_ISSUER_URL is invalid: %w", err)
	}
	if strings.TrimSpace(c.ClientID) == "" {
		return errClientIDRequired
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		return errClientSecretRequired
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return errBaseURLRequired
	}
	_, err = url.ParseRequestURI(c.RedirectURI())
	if err != nil {
		return fmt.Errorf("OIDC redirect URI is invalid: %w", err)
	}

	return nil
}
