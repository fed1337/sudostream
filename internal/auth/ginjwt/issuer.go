// Package ginjwt wires appleboy/gin-jwt to sudoStream session storage.
package ginjwt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sudoStream/internal/auth"
	"time"

	jwtmw "github.com/appleboy/gin-jwt/v3"
	"github.com/appleboy/gin-jwt/v3/core"
	"github.com/gin-gonic/gin"

	authpostgres "sudoStream/internal/auth/postgres"
)

const (
	authRequiredMessage = "authentication required"
	userIdentityKey     = "authUser"
)

// Issuer implements auth.TokenIssuer using appleboy/gin-jwt.
type Issuer struct {
	mw         *jwtmw.GinJWTMiddleware
	store      auth.Store
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// New constructs a gin-jwt backed token issuer.
func New(store auth.Store, config auth.Config) (*Issuer, error) {
	accessTTL := normalizeAccessTTL(config.AccessTTL)
	refreshTTL := normalizeRefreshTTL(config.RefreshTTL)
	secret := auth.NormalizeJWTSecret(config.JWTSecret)

	issuer := &Issuer{
		store:      store,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}

	refreshStore := authpostgres.NewRefreshTokenStore(store)
	jwtMiddleware, err := jwtmw.New(
		&jwtmw.GinJWTMiddleware{ //nolint:exhaustruct // gin-jwt defaults
			Realm:               auth.JWTRealm,
			Key:                 secret,
			Timeout:             accessTTL,
			RefreshTokenTimeout: refreshTTL,
			RefreshTokenStore:   refreshStore,
			SendCookie:          false,
			TokenLookup:         "header: Authorization, query: access_token",
			TokenHeadName:       "Bearer",
			IdentityKey:         userIdentityKey,
			PayloadFunc:         sessionClaims,
			IdentityHandler:     identityFromClaims,
			Authorizer:          issuer.authorizer,
			Unauthorized:        unauthorized,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("init gin-jwt: %w", err)
	}

	issuer.mw = jwtMiddleware

	return issuer, nil
}

// Middleware returns gin-jwt auth middleware for protected routes.
func (i *Issuer) Middleware() gin.HandlerFunc {
	return i.mw.MiddlewareFunc()
}

// TryAuthenticate validates an access token when present without aborting the request.
func (i *Issuer) TryAuthenticate(c *gin.Context) (auth.PublicUser, string, bool) {
	token, err := i.mw.ParseToken(c)
	if err != nil {
		return auth.PublicUser{}, "", false
	}

	claims := jwtmw.ExtractClaimsFromToken(token)
	c.Set("JWT_PAYLOAD", claims)
	user := publicUserFromClaims(claims)
	if !i.authorizer(c, user) {
		return auth.PublicUser{}, "", false
	}

	return user, claimString(claims, auth.ClaimSessionID), true
}

// IssueTokens creates a new access and opaque refresh token pair.
func (i *Issuer) IssueTokens(identity auth.SessionIdentity) (*auth.TokenPair, error) {
	tokenPair, err := i.mw.TokenGenerator(context.Background(), identity)
	if err != nil {
		return nil, fmt.Errorf("issue tokens: %w", err)
	}

	return &auth.TokenPair{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
	}, nil
}

// RefreshWithRotation validates a refresh token and issues a rotated pair.
func (i *Issuer) RefreshWithRotation(
	oldRefresh string,
) (auth.SessionIdentity, *auth.TokenPair, error) {
	identity, err := i.ValidateRefresh(oldRefresh)
	if err != nil {
		return auth.SessionIdentity{}, nil, err
	}

	user, err := i.store.GetUserByID(context.Background(), identity.UserID)
	if err != nil {
		return auth.SessionIdentity{}, nil, auth.ErrSessionNotFound
	}
	if !user.Enabled {
		return auth.SessionIdentity{}, nil, auth.ErrUserDisabled
	}

	identity.User = user.Public()
	tokenPair, err := i.mw.TokenGeneratorWithRevocation(
		context.Background(),
		identity,
		oldRefresh,
	)
	if err != nil {
		return auth.SessionIdentity{}, nil, fmt.Errorf("refresh tokens: %w", err)
	}

	return identity, &auth.TokenPair{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
	}, nil
}

// ValidateRefresh loads session identity for an opaque refresh token.
func (i *Issuer) ValidateRefresh(rawRefresh string) (auth.SessionIdentity, error) {
	if rawRefresh == "" {
		return auth.SessionIdentity{}, auth.ErrSessionNotFound
	}

	userData, err := i.mw.RefreshTokenStore.Get(context.Background(), rawRefresh)
	if err != nil {
		if errors.Is(err, core.ErrRefreshTokenNotFound) ||
			errors.Is(err, core.ErrRefreshTokenExpired) {
			return auth.SessionIdentity{}, auth.ErrSessionNotFound
		}

		return auth.SessionIdentity{}, fmt.Errorf("validate refresh: %w", err)
	}

	identity, ok := userData.(auth.SessionIdentity)
	if !ok {
		return auth.SessionIdentity{}, auth.ErrSessionNotFound
	}

	return identity, nil
}

// RevokeRefresh deletes the session for a refresh token.
func (i *Issuer) RevokeRefresh(rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}

	err := i.mw.RefreshTokenStore.Delete(context.Background(), rawRefresh)
	if err != nil {
		return fmt.Errorf("revoke refresh: %w", err)
	}

	return nil
}

// AccessTTL returns configured access token lifetime.
func (i *Issuer) AccessTTL() time.Duration {
	return i.accessTTL
}

// RefreshTTL returns configured refresh token lifetime.
func (i *Issuer) RefreshTTL() time.Duration {
	return i.refreshTTL
}

func (i *Issuer) authorizer(c *gin.Context, _ any) bool {
	claims := map[string]any(jwtmw.ExtractClaims(c))
	sessionID := claimString(claims, auth.ClaimSessionID)
	userID := claimString(claims, auth.ClaimUserID)
	if sessionID == "" || userID == "" {
		return false
	}

	session, err := i.store.GetSessionByID(c.Request.Context(), sessionID)
	if err != nil {
		return false
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return false
	}
	if session.UserID != userID {
		return false
	}

	user, err := i.store.GetUserByID(c.Request.Context(), userID)
	if err != nil || !user.Enabled {
		return false
	}

	return true
}

func identityFromClaims(c *gin.Context) any {
	return publicUserFromClaims(jwtmw.ExtractClaims(c))
}

func publicUserFromClaims(claims map[string]any) auth.PublicUser {
	mustChange, _ := claims[auth.ClaimMustChangePass].(bool)

	return auth.PublicUser{
		ID:                 claimString(claims, auth.ClaimUserID),
		Email:              claimString(claims, auth.ClaimEmail),
		Role:               claimString(claims, auth.ClaimRole),
		MustChangePassword: mustChange,
	}
}

func claimString(claims map[string]any, key string) string {
	value, ok := claims[key].(string)
	if !ok {
		return ""
	}

	return value
}

func unauthorized(c *gin.Context, _ int, _ string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": authRequiredMessage})
}

func normalizeAccessTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return auth.DefaultAccessTTL()
	}

	return ttl
}

func normalizeRefreshTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return auth.DefaultRefreshTTL()
	}

	return ttl
}
