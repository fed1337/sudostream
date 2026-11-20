package auth

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrExternalIdentityNotFound is returned when no IdP subject mapping exists.
	ErrExternalIdentityNotFound = errors.New("external identity not found")
	// ErrOIDCUnknownIdentity is returned when merge/register cannot resolve a user.
	ErrOIDCUnknownIdentity = errors.New("unknown oidc identity")
	// ErrOIDCEmailRequired is returned when claims lack an email needed to proceed.
	ErrOIDCEmailRequired = errors.New("oidc email required")
	// ErrOIDCEmailNotVerified is returned when email verification is required but missing.
	ErrOIDCEmailNotVerified = errors.New("oidc email not verified")
	// ErrOIDCDisabled is returned when SSO is not enabled.
	ErrOIDCDisabled = errors.New("oidc disabled")
)

// ExternalIdentity links an IdP (issuer, subject) to a local user.
type ExternalIdentity struct {
	ID        string
	UserID    string
	Issuer    string
	Subject   string
	IDToken   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OIDCLoginInput is the verified IdP identity used to complete SSO login.
type OIDCLoginInput struct {
	Issuer                   string
	Subject                  string
	Email                    string
	EmailVerified            bool
	Username                 string
	Groups                   []string
	RawIDToken               string
	AdminGroup               string
	AutoRegister             bool
	MergeUsers               bool
	RequireEmailVerification bool
}

// OIDCLogoutHint carries data needed for RP-initiated logout after local revoke.
type OIDCLogoutHint struct {
	IDTokenHint string
}

// ExternalIdentityStore persists IdP subject mappings.
type ExternalIdentityStore interface {
	GetExternalIdentityBySubject(
		ctx context.Context,
		issuer, subject string,
	) (*ExternalIdentity, error)
	GetExternalIdentityByUserID(ctx context.Context, userID string) (*ExternalIdentity, error)
	UpsertExternalIdentity(ctx context.Context, identity ExternalIdentity) error
	UpdateExternalIdentityIDToken(ctx context.Context, issuer, subject, idToken string) error
}
