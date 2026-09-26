package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ListUsers returns all accounts for admin management with derived status.
func (s *Service) ListUsers(ctx context.Context) ([]AdminUser, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	pendingEmails, err := s.pendingInviteEmails(ctx)
	if err != nil {
		return nil, err
	}

	for i := range users {
		users[i].Status = deriveUserStatus(users[i], pendingEmails)
	}

	return users, nil
}

func (s *Service) pendingInviteEmails(ctx context.Context) (map[string]struct{}, error) {
	invites, err := s.store.ListPendingInvites(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("list pending invites: %w", err)
	}

	emails := make(map[string]struct{}, len(invites))
	for _, invite := range invites {
		emails[strings.ToLower(invite.Email)] = struct{}{}
	}

	return emails, nil
}

func deriveUserStatus(user AdminUser, pendingInvites map[string]struct{}) string {
	if _, ok := pendingInvites[strings.ToLower(user.Email)]; ok {
		return UserStatusInvited
	}
	if !user.EmailVerified {
		return UserStatusUnconfirmed
	}
	if user.Enabled {
		return UserStatusActive
	}

	return UserStatusDisabled
}

// GetUserByID loads a user account by primary key.
func (s *Service) GetUserByID(ctx context.Context, userID string) (*User, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	return user, nil
}

// CreateUser creates an account with a password (admin only).
// The account is active immediately with mustChangePassword=true; email is marked verified.
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (AdminUser, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	if email == "" || input.Password == "" {
		return AdminUser{}, ErrMissingCredentials
	}

	role, err := NormalizeRole(input.Role)
	if err != nil {
		return AdminUser{}, err
	}

	_, _, err = s.store.GetUserByEmail(ctx, email)
	if err == nil {
		return AdminUser{}, ErrUserExists
	}

	hash, err := HashPassword(input.Password)
	if err != nil {
		return AdminUser{}, fmt.Errorf("hash password: %w", err)
	}

	user := User{
		ID:                 "",
		Email:              email,
		Role:               role,
		Enabled:            true,
		EmailVerifiedAt:    nil,
		MustChangePassword: true,
	}

	err = s.store.CreateUser(ctx, user, hash)
	if err != nil {
		return AdminUser{}, fmt.Errorf("create user: %w", err)
	}

	created, _, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return AdminUser{}, fmt.Errorf("load created user: %w", err)
	}

	err = s.store.EnableUser(ctx, created.ID, true)
	if err != nil {
		return AdminUser{}, fmt.Errorf("enable created user: %w", err)
	}

	return s.adminUserByID(ctx, created.ID)
}

// UpdateUser applies admin patches to an account.
func (s *Service) UpdateUser( //nolint:cyclop // email/enabled/role patches
	ctx context.Context,
	userID string,
	patch UserPatch,
) (AdminUser, error) {
	target, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return AdminUser{}, fmt.Errorf("get user: %w", err)
	}

	err = s.guardLastAdminMutation(ctx, *target, patch)
	if err != nil {
		return AdminUser{}, err
	}

	if patch.Email != nil {
		err = s.adminChangeEmail(ctx, *target, *patch.Email)
		if err != nil {
			return AdminUser{}, err
		}
	}

	if patch.Enabled != nil {
		err = s.applyEnabledPatch(ctx, userID, *patch.Enabled)
		if err != nil {
			return AdminUser{}, err
		}

		patch.Enabled = nil
	}

	if patch.Role != nil {
		role, roleErr := NormalizeRole(*patch.Role)
		if roleErr != nil {
			return AdminUser{}, roleErr
		}
		patch.Role = &role

		err = s.store.UpdateUser(
			ctx,
			userID,
			UserPatch{Role: patch.Role},
		)
		if err != nil {
			return AdminUser{}, fmt.Errorf("update user: %w", err)
		}

		if !AllowsWebLogin(role) {
			_ = s.AdminRevokeAllSessions(ctx, userID)
		}
	}

	return s.adminUserByID(ctx, userID)
}

// adminChangeEmail updates email, clears verification, and emails a confirmation link.
func (s *Service) adminChangeEmail(ctx context.Context, target User, rawEmail string) error {
	email := strings.TrimSpace(strings.ToLower(rawEmail))
	if email == "" {
		return ErrMissingEmail
	}
	if email == strings.ToLower(target.Email) {
		return nil
	}

	_, _, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		return ErrUserExists
	}

	oldEmail := target.Email
	err = s.store.UpdateUserEmail(ctx, target.ID, email, true)
	if err != nil {
		return fmt.Errorf("update email: %w", err)
	}

	_ = s.store.InvalidateUnusedTokens(ctx, oldEmail, TokenPurposeInvite)
	_ = s.store.InvalidateUnusedTokens(ctx, oldEmail, TokenPurposeConfirmEmail)
	_ = s.store.InvalidateUnusedTokens(ctx, oldEmail, TokenPurposeChangeEmail)
	_ = s.store.InvalidateUnusedTokens(ctx, email, TokenPurposeInvite)
	_ = s.store.InvalidateUnusedTokens(ctx, email, TokenPurposeConfirmEmail)
	_ = s.store.InvalidateUnusedTokens(ctx, email, TokenPurposeChangeEmail)

	err = s.sendConfirmEmail(ctx, target.ID, email)
	if err != nil {
		return fmt.Errorf("send confirm email: %w", err)
	}

	return nil
}

// DeleteUser permanently removes a disabled account so the email can be reused.
func (s *Service) DeleteUser(ctx context.Context, userID string) error {
	target, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrManagedUserNotFound) {
			return ErrManagedUserNotFound
		}

		return fmt.Errorf("get user: %w", err)
	}
	if target.Enabled {
		return ErrUserNotDisabled
	}

	err = s.guardLastAdminMutation(
		ctx,
		*target,
		UserPatch{Enabled: new(false)},
	)
	if err != nil {
		return err
	}

	err = s.store.DeleteUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}

func (s *Service) adminUserByID(ctx context.Context, userID string) (AdminUser, error) {
	users, err := s.ListUsers(ctx)
	if err != nil {
		return AdminUser{}, err
	}

	for _, listed := range users {
		if listed.ID == userID {
			return listed, nil
		}
	}

	return AdminUser{}, ErrManagedUserNotFound
}

func (s *Service) applyEnabledPatch(ctx context.Context, userID string, enabled bool) error {
	if enabled {
		err := s.store.EnableUser(ctx, userID, true)
		if err != nil {
			return fmt.Errorf("enable user: %w", err)
		}

		return nil
	}

	err := s.store.DeleteAllSessionsByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}

	disabled := false
	err = s.store.UpdateUser(
		ctx,
		userID,
		UserPatch{Enabled: &disabled},
	)
	if err != nil {
		return fmt.Errorf("disable user: %w", err)
	}

	return nil
}

func (s *Service) guardLastAdminMutation(ctx context.Context, target User, patch UserPatch) error {
	if target.Role != RoleAdmin {
		return nil
	}

	disable := patch.Enabled != nil && !*patch.Enabled
	demote := patch.Role != nil && *patch.Role != RoleAdmin
	if !disable && !demote {
		return nil
	}

	admins, err := s.store.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if admins <= 1 {
		return ErrForbidden
	}

	return nil
}

func unusablePasswordHash() (string, error) {
	const unusableSecretBytes = 16
	buf := make([]byte, unusableSecretBytes)
	_, err := rand.Read(buf)
	if err != nil {
		return "", fmt.Errorf("generate unusable password: %w", err)
	}

	// bcrypt rejects inputs longer than 72 bytes; keep the marker short.
	return HashPassword("x" + hex.EncodeToString(buf))
}
