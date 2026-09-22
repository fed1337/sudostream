package auth

import (
	"context"
	"fmt"
	"time"
)

// ListUserSessions returns sessions for the authenticated user.
func (s *Service) ListUserSessions(
	ctx context.Context,
	userID, currentSessionID string,
) ([]SessionView, error) {
	sessions, err := s.store.ListSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	views := make([]SessionView, 0, len(sessions))
	now := time.Now().UTC()
	for _, session := range sessions {
		if now.After(session.ExpiresAt) {
			continue
		}

		views = append(views, SessionView{
			ID:        session.ID,
			IP:        session.IP,
			UserAgent: session.UserAgent,
			CreatedAt: session.CreatedAt,
			ExpiresAt: session.ExpiresAt,
			Current:   currentSessionID != "" && session.ID == currentSessionID,
		})
	}

	return views, nil
}

// RevokeUserSession removes one of the user's sessions.
func (s *Service) RevokeUserSession(
	ctx context.Context,
	userID, sessionID, currentSessionID string,
) (bool, error) {
	session, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("load session: %w", err)
	}
	if session.UserID != userID {
		return false, ErrForbidden
	}

	isCurrent := currentSessionID != "" && session.ID == currentSessionID

	err = s.store.DeleteSessionByID(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("delete session: %w", err)
	}

	return isCurrent, nil
}

// AdminListUserSessions returns sessions for admin management.
func (s *Service) AdminListUserSessions(ctx context.Context, userID string) ([]SessionView, error) {
	sessions, err := s.store.ListSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	views := make([]SessionView, 0, len(sessions))
	now := time.Now().UTC()
	for _, session := range sessions {
		if now.After(session.ExpiresAt) {
			continue
		}

		views = append(views, SessionView{
			ID:        session.ID,
			IP:        session.IP,
			UserAgent: session.UserAgent,
			CreatedAt: session.CreatedAt,
			ExpiresAt: session.ExpiresAt,
			Current:   false,
		})
	}

	return views, nil
}

// AdminRevokeSession removes a session by ID.
func (s *Service) AdminRevokeSession(ctx context.Context, sessionID string) error {
	err := s.store.DeleteSessionByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// RevokeAllUserSessions removes every session for the authenticated user.
func (s *Service) RevokeAllUserSessions(ctx context.Context, userID string) error {
	err := s.store.DeleteAllSessionsByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}

	return nil
}

// AdminRevokeAllSessions removes every session for a user.
func (s *Service) AdminRevokeAllSessions(ctx context.Context, userID string) error {
	err := s.store.DeleteAllSessionsByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("delete sessions: %w", err)
	}

	return nil
}
