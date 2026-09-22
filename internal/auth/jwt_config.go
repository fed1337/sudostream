package auth

import (
	"errors"
	"time"
)

const (
	// JWTRealm is the issuer realm for access tokens.
	JWTRealm = "sudostream"

	// ClaimSessionID is the JWT claim holding the server session ID.
	ClaimSessionID = "sessionID"
	// ClaimUserID is the JWT claim holding the user ID.
	ClaimUserID = "userID"
	// ClaimEmail is the JWT claim holding the user email.
	ClaimEmail = "email"
	// ClaimRole is the JWT claim holding the user role.
	ClaimRole = "role"
	// ClaimMustChangePass is the JWT claim for forced password changes.
	ClaimMustChangePass = "mustChangePassword"

	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 7 * 24 * time.Hour
	minJWTSecretBytes = 32
)

var (
	errJWTSecretShort = errors.New("jwt secret too short")
	// ErrInvalidSessionIdentity indicates refresh-store payload was not SessionIdentity.
	ErrInvalidSessionIdentity = errors.New("invalid session identity")
)

// ParseJWTSecret loads a JWT signing secret from raw env bytes.
func ParseJWTSecret(raw string) ([]byte, error) {
	secret := []byte(raw)
	if len(secret) < minJWTSecretBytes {
		return nil, errJWTSecretShort
	}

	return secret, nil
}

func devJWTSecret() []byte {
	return []byte("dev-only-jwt-secret-32-bytes-min!!")
}

// NormalizeJWTSecret returns a signing key of at least 32 bytes.
func NormalizeJWTSecret(secret []byte) []byte {
	if len(secret) < minJWTSecretBytes {
		return devJWTSecret()
	}

	return secret
}

// DefaultAccessTTL returns the default access token lifetime.
func DefaultAccessTTL() time.Duration {
	return defaultAccessTTL
}

// DefaultRefreshTTL returns the default refresh token lifetime.
func DefaultRefreshTTL() time.Duration {
	return defaultRefreshTTL
}
