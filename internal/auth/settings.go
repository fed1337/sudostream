package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrInvalidToken is returned when a token is unknown or malformed.
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired is returned when a token has expired.
	ErrTokenExpired = errors.New("token expired")
	// ErrTokenUsed is returned when a token was already consumed.
	ErrTokenUsed = errors.New("token already used")
	// ErrForbidden is returned for disallowed admin operations.
	ErrForbidden = errors.New("forbidden")
	// ErrUserExists is returned when creating a duplicate account.
	ErrUserExists = errors.New("user already exists")
	// ErrMissingCredentials is returned when required user fields are empty.
	ErrMissingCredentials = errors.New("email and password are required")
	// ErrManagedUserNotFound is returned when an expected user row is missing.
	ErrManagedUserNotFound = errors.New("managed user not found")
	// ErrMissingEmail is returned when an email address is required.
	ErrMissingEmail = errors.New("email is required")
	// ErrResendConfirmUnavailable is returned when confirmation resend does not apply.
	ErrResendConfirmUnavailable = errors.New("confirmation resend unavailable")
	// ErrPasswordUnchanged is returned when the new password matches the current one.
	ErrPasswordUnchanged = errors.New("password unchanged")
	// ErrUserNotDisabled is returned when deleting a user that is still enabled.
	ErrUserNotDisabled = errors.New("user is not disabled")
)

// GetSettings returns current auth policy settings.
func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	settings, err := s.store.GetSettings(ctx)
	//nolint:nilerr // defaults are intentional when settings cannot be loaded
	if err != nil {
		return defaultSettings(), nil
	}

	return normalizeSettings(settings), nil
}

// UpdateSettings persists auth policy settings.
func (s *Service) UpdateSettings(ctx context.Context, settings Settings) (Settings, error) {
	settings = normalizeSettings(settings)

	err := s.store.SaveSettings(ctx, settings)
	if err != nil {
		return Settings{}, fmt.Errorf("save settings: %w", err)
	}

	return settings, nil
}

// EnsureSettingsDefaults seeds settings when missing.
func (s *Service) EnsureSettingsDefaults(ctx context.Context) error {
	_, err := s.store.GetSettings(ctx)
	if err == nil {
		return nil
	}

	err = s.store.SaveSettings(ctx, defaultSettings())
	if err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}

	return nil
}

func (s *Service) loadSettings(ctx context.Context) Settings {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return defaultSettings()
	}

	return normalizeSettings(settings)
}

func defaultSettings() Settings {
	return Settings{
		EmailConfirmationRequired: false,
		TwoFactorRequired:         false,
	}
}

func normalizeSettings(settings Settings) Settings {
	return settings
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultAppBaseURL
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.InviteTTLHours <= 0 {
		config.InviteTTLHours = defaultTokenTTLHours
	}
	if config.ConfirmEmailTTLHours <= 0 {
		config.ConfirmEmailTTLHours = defaultTokenTTLHours
	}
	if config.ResetPasswordTTLHours <= 0 {
		config.ResetPasswordTTLHours = defaultTokenTTLHours
	}
	if config.TOTPLeewaySeconds <= minTwoFactorLeewaySeconds {
		config.TOTPLeewaySeconds = defaultTwoFactorLeeway
	}
	if config.TOTPLeewaySeconds > maxTwoFactorLeewaySeconds {
		config.TOTPLeewaySeconds = maxTwoFactorLeewaySeconds
	}

	return config
}

func (s *Service) appBaseURL() string {
	return s.config.BaseURL
}

func (s *Service) inviteTTL() time.Duration {
	return time.Duration(s.config.InviteTTLHours) * time.Hour
}

func (s *Service) resetPasswordTTL() time.Duration {
	return time.Duration(s.config.ResetPasswordTTLHours) * time.Hour
}

func (s *Service) confirmEmailTTL() time.Duration {
	return time.Duration(s.config.ConfirmEmailTTLHours) * time.Hour
}

func (s *Service) totpLeewaySeconds() int {
	return s.config.TOTPLeewaySeconds
}
