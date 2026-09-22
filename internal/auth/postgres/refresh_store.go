package postgres

import (
	"context"
	"fmt"
	"sudoStream/internal/auth"
	"time"

	"github.com/appleboy/gin-jwt/v3/core"
)

// RefreshTokenStore persists opaque refresh tokens in the sessions table.
type RefreshTokenStore struct {
	store auth.Store
}

// NewRefreshTokenStore wraps an auth store for gin-jwt refresh token storage.
func NewRefreshTokenStore(store auth.Store) *RefreshTokenStore {
	return &RefreshTokenStore{store: store}
}

// Set stores a refresh token hash with session metadata.
func (s *RefreshTokenStore) Set(
	ctx context.Context,
	token string,
	userData any,
	expiry time.Time,
) error {
	identity, ok := userData.(auth.SessionIdentity)
	if !ok {
		return auth.ErrInvalidSessionIdentity
	}

	tokenHash := auth.HashToken(token)
	_, err := s.store.GetSessionByID(ctx, identity.SessionID)
	if err == nil {
		err = s.store.UpdateSessionRefreshToken(ctx, identity.SessionID, tokenHash, expiry.UTC())
		if err != nil {
			return fmt.Errorf("update session refresh token: %w", err)
		}

		return nil
	}

	err = s.store.CreateSession(
		ctx,
		identity.SessionID,
		identity.UserID,
		tokenHash,
		expiry.UTC(),
		identity.Meta,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

// Get loads session identity for a refresh token.
func (s *RefreshTokenStore) Get(ctx context.Context, token string) (any, error) {
	session, err := s.store.GetSessionByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return nil, core.ErrRefreshTokenNotFound
	}

	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.store.DeleteSessionByTokenHash(ctx, auth.HashToken(token))

		return nil, core.ErrRefreshTokenExpired
	}

	user, err := s.store.GetUserByID(ctx, session.UserID)
	if err != nil {
		return nil, core.ErrRefreshTokenNotFound
	}

	return auth.SessionIdentity{
		SessionID: session.ID,
		UserID:    user.ID,
		User:      user.Public(),
		Meta: auth.SessionMeta{
			IP:        session.IP,
			UserAgent: session.UserAgent,
		},
	}, nil
}

// Delete removes a refresh token session.
func (s *RefreshTokenStore) Delete(ctx context.Context, token string) error {
	err := s.store.DeleteSessionByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return fmt.Errorf("delete session by token hash: %w", err)
	}

	return nil
}

// Cleanup is a no-op; session expiry is enforced on Get.
func (s *RefreshTokenStore) Cleanup(context.Context) (int, error) {
	return 0, nil
}

// Count returns zero; session totals are not tracked for refresh storage.
func (s *RefreshTokenStore) Count(context.Context) (int, error) {
	return 0, nil
}
