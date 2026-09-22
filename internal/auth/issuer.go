package auth

import "time"

// TokenPair holds issued access and refresh tokens.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// TokenIssuer signs access JWTs and manages opaque refresh tokens.
type TokenIssuer interface {
	IssueTokens(identity SessionIdentity) (*TokenPair, error)
	RefreshWithRotation(oldRefresh string) (SessionIdentity, *TokenPair, error)
	ValidateRefresh(rawRefresh string) (SessionIdentity, error)
	RevokeRefresh(rawRefresh string) error
	AccessTTL() time.Duration
	RefreshTTL() time.Duration
}
