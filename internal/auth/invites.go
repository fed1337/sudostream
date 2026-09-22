package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ListPendingInvites returns unused invite tokens, optionally filtered by email.
func (s *Service) ListPendingInvites(ctx context.Context, email string) ([]InviteView, error) {
	invites, err := s.store.ListPendingInvites(ctx, strings.TrimSpace(strings.ToLower(email)))
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}

	now := time.Now().UTC()
	views := make([]InviteView, 0, len(invites))
	for _, invite := range invites {
		views = append(views, InviteView{
			ID:        invite.ID,
			Email:     invite.Email,
			Role:      invite.Role,
			ExpiresAt: invite.ExpiresAt,
			CreatedAt: invite.CreatedAt,
			Expired:   now.After(invite.ExpiresAt),
		})
	}

	return views, nil
}

// DefaultInviteTTL returns configured invite TTL in hours (from runtime env config).
func (s *Service) DefaultInviteTTL() int {
	return s.config.InviteTTLHours
}

// ResendInvite invalidates prior invite tokens and sends a fresh invite email.
func (s *Service) ResendInvite(
	ctx context.Context,
	tokenID, invitedBy string,
	expiresIn time.Duration,
) error {
	token, err := s.store.GetAuthTokenByID(ctx, tokenID)
	if err != nil {
		return fmt.Errorf("load invite token: %w", err)
	}
	if token.Purpose != TokenPurposeInvite {
		return ErrInvalidToken
	}

	if expiresIn <= 0 {
		expiresIn = s.inviteTTL()
	}

	err = s.store.InvalidateUnusedTokens(ctx, token.Email, TokenPurposeInvite)
	if err != nil {
		return fmt.Errorf("invalidate invite tokens: %w", err)
	}

	return s.CreateInvite(ctx, token.Email, token.Role, invitedBy, expiresIn)
}

// RevokeInvite invalidates a pending invite before it is accepted.
func (s *Service) RevokeInvite(ctx context.Context, tokenID string) error {
	token, err := s.store.GetAuthTokenByID(ctx, tokenID)
	if err != nil {
		return fmt.Errorf("load invite token: %w", err)
	}
	if token.Purpose != TokenPurposeInvite {
		return ErrInvalidToken
	}
	if token.UsedAt != nil {
		return ErrInvalidToken
	}

	err = s.store.MarkAuthTokenUsed(ctx, token.ID)
	if err != nil {
		return fmt.Errorf("revoke invite token: %w", err)
	}

	return nil
}
