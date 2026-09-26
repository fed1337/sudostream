package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// RequestPasswordReset emails a reset link when the account exists.
// When email confirmation is required and the account is unverified, a confirm
// link is (re)sent instead of a reset link.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil
	}

	user, _, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return nil //nolint:nilerr // do not reveal whether the account exists
	}

	settings := s.loadSettings(ctx)
	if settings.EmailConfirmationRequired && user.EmailVerifiedAt == nil {
		s.ensureConfirmEmailSent(ctx, user)

		return nil
	}

	rawToken, tokenHash, err := newSessionToken()
	if err != nil {
		return fmt.Errorf("create reset token: %w", err)
	}

	resetTTL := s.resetPasswordTTL()

	err = s.store.InvalidateUnusedTokens(ctx, email, TokenPurposeResetPassword)
	if err != nil {
		return fmt.Errorf("invalidate reset tokens: %w", err)
	}

	err = s.store.CreateAuthToken(ctx, Token{
		ID:        "",
		UserID:    user.ID,
		Email:     email,
		Purpose:   TokenPurposeResetPassword,
		TokenHash: tokenHash,
		Role:      "",
		InvitedBy: "",
		ExpiresAt: time.Now().UTC().Add(resetTTL),
		UsedAt:    nil,
	})
	if err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	link := s.appBaseURL() + "/reset-password?token=" + rawToken
	subject := "Reset your sudoStream password"
	body := fmt.Sprintf(
		"Use this link to reset your password (expires in %s):\n\n%s",
		resetTTL.Round(time.Minute),
		link,
	)

	err = s.mail.Send(ctx, email, subject, body)
	if err != nil {
		return fmt.Errorf("send reset email: %w", err)
	}

	return nil
}

// ResetPassword consumes a reset token and sets a new password.
func (s *Service) ResetPassword(ctx context.Context, rawToken, password string) error {
	token, err := s.validateToken(ctx, rawToken, TokenPurposeResetPassword)
	if err != nil {
		return err
	}

	_, currentHash, err := s.store.GetUserWithPasswordByID(ctx, token.UserID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	err = bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(password))
	if err == nil {
		return ErrPasswordUnchanged
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	err = s.store.MarkAuthTokenUsed(ctx, token.ID)
	if err != nil {
		return fmt.Errorf("mark token used: %w", err)
	}

	err = s.store.UpdatePasswordHash(ctx, token.UserID, hash, false)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	err = s.store.DeleteAllSessionsByUserID(ctx, token.UserID)
	if err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}

	return nil
}

// ResendConfirmEmail resends a pending invite, change-email, or confirm link.
func (s *Service) ResendConfirmEmail(ctx context.Context, userID string) error {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	pending, err := s.store.ListPendingInvites(ctx, user.Email)
	if err != nil {
		return fmt.Errorf("list pending invites: %w", err)
	}
	if len(pending) > 0 {
		return s.ResendInvite(ctx, pending[0].ID, pending[0].InvitedBy, 0)
	}

	changeToken, changeErr := s.store.GetUnusedAuthTokenByUser(
		ctx,
		user.ID,
		TokenPurposeChangeEmail,
	)
	if changeErr == nil {
		return s.resendChangeEmailToken(ctx, user.ID, changeToken.Email)
	}

	if user.EmailVerifiedAt != nil {
		return ErrResendConfirmUnavailable
	}

	err = s.store.InvalidateUnusedTokens(ctx, user.Email, TokenPurposeConfirmEmail)
	if err != nil {
		return fmt.Errorf("invalidate confirm tokens: %w", err)
	}

	return s.sendConfirmEmail(ctx, user.ID, user.Email)
}

func (s *Service) resendChangeEmailToken(ctx context.Context, userID, newEmail string) error {
	err := s.store.InvalidateUnusedTokens(ctx, newEmail, TokenPurposeChangeEmail)
	if err != nil {
		return fmt.Errorf("invalidate change-email tokens: %w", err)
	}

	return s.issueChangeEmailToken(ctx, userID, newEmail)
}

// ConfirmEmail verifies an email address or applies a pending email change from a token link.
// Already-used tokens succeed idempotently so double-clicks / remounts do not surface errors.
func (s *Service) ConfirmEmail(ctx context.Context, rawToken string) error {
	err := s.confirmEmailPurpose(ctx, rawToken, TokenPurposeConfirmEmail)
	if err == nil || errors.Is(err, ErrTokenUsed) {
		return nil
	}
	if !errors.Is(err, ErrInvalidToken) {
		return err
	}

	changeErr := s.confirmEmailPurpose(ctx, rawToken, TokenPurposeChangeEmail)
	if changeErr == nil || errors.Is(changeErr, ErrTokenUsed) {
		return nil
	}

	return changeErr
}

func (s *Service) confirmEmailPurpose(ctx context.Context, rawToken, purpose string) error {
	token, err := s.consumeToken(ctx, rawToken, purpose)
	if err != nil {
		return err
	}

	if purpose == TokenPurposeChangeEmail {
		err = s.store.UpdateUserEmail(ctx, token.UserID, token.Email, false)
		if err != nil {
			return fmt.Errorf("apply email change: %w", err)
		}
		_ = s.store.InvalidateUnusedTokens(ctx, token.Email, TokenPurposeChangeEmail)
	}

	err = s.store.EnableUser(ctx, token.UserID, true)
	if err != nil {
		return fmt.Errorf("verify email after confirm: %w", err)
	}

	return nil
}

// RequestChangeEmail verifies the password (and optional TOTP) then emails a confirmation link
// to the new address.
func (s *Service) RequestChangeEmail( //nolint:cyclop // password + optional 2FA + token email steps
	ctx context.Context,
	userID, newEmail, password, totpCode string,
) error {
	newEmail = strings.TrimSpace(strings.ToLower(newEmail))
	if newEmail == "" {
		return ErrMissingEmail
	}

	user, hash, err := s.store.GetUserWithPasswordByID(ctx, userID)
	if err != nil {
		return ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return ErrInvalidCredentials
	}

	if newEmail == strings.ToLower(user.Email) {
		return ErrUserExists
	}

	_, _, err = s.store.GetUserByEmail(ctx, newEmail)
	if err == nil {
		return ErrUserExists
	}

	has2FA, err := s.store.UserHasTOTP(ctx, userID)
	if err != nil {
		return fmt.Errorf("check 2fa: %w", err)
	}
	if has2FA {
		err = s.requireUserTOTP(ctx, userID, totpCode)
		if err != nil {
			return err
		}
	}

	err = s.store.InvalidateUnusedTokens(ctx, user.Email, TokenPurposeChangeEmail)
	if err != nil {
		return fmt.Errorf("invalidate change-email tokens: %w", err)
	}
	err = s.store.InvalidateUnusedTokens(ctx, newEmail, TokenPurposeChangeEmail)
	if err != nil {
		return fmt.Errorf("invalidate change-email tokens: %w", err)
	}
	err = s.store.InvalidateUnusedTokens(ctx, user.Email, TokenPurposeConfirmEmail)
	if err != nil {
		return fmt.Errorf("invalidate confirm tokens: %w", err)
	}

	err = s.store.ClearEmailVerified(ctx, userID)
	if err != nil {
		return fmt.Errorf("clear email verified: %w", err)
	}

	return s.issueChangeEmailToken(ctx, userID, newEmail)
}

func (s *Service) requireUserTOTP(ctx context.Context, userID, totpCode string) error {
	if strings.TrimSpace(totpCode) == "" {
		return ErrTwoFactorRequired
	}

	record, err := s.store.GetUserTOTP(ctx, userID)
	if err != nil {
		return fmt.Errorf("load 2fa: %w", err)
	}

	verified, err := s.verifyTwoFactorCode(
		ctx,
		userID,
		record,
		totpCode,
		s.totpLeewaySeconds(),
	)
	if err != nil {
		return err
	}
	if !verified {
		return ErrInvalidTwoFactorCode
	}

	return nil
}

//nolint:dupl // mirrors sendConfirmEmail with change_email purpose and copy
func (s *Service) issueChangeEmailToken(
	ctx context.Context,
	userID, newEmail string,
) error {
	rawToken, tokenHash, err := newSessionToken()
	if err != nil {
		return fmt.Errorf("create change-email token: %w", err)
	}

	ttl := s.confirmEmailTTL()

	err = s.store.CreateAuthToken(ctx, Token{
		ID:        "",
		UserID:    userID,
		Email:     newEmail,
		Purpose:   TokenPurposeChangeEmail,
		TokenHash: tokenHash,
		Role:      "",
		InvitedBy: "",
		ExpiresAt: time.Now().UTC().Add(ttl),
		UsedAt:    nil,
	})
	if err != nil {
		return fmt.Errorf("store change-email token: %w", err)
	}

	link := s.appBaseURL() + "/confirm-email?token=" + rawToken
	subject := "Confirm your new sudoStream email"
	body := fmt.Sprintf(
		"Confirm your new email address (expires in %s):\n\n%s",
		ttl.Round(time.Minute),
		link,
	)

	err = s.mail.Send(ctx, newEmail, subject, body)
	if err != nil {
		return fmt.Errorf("send change-email: %w", err)
	}

	return nil
}

// CreateInvite creates a disabled stub user (if needed) and emails an invite link.
func (s *Service) CreateInvite(
	ctx context.Context,
	email, role, invitedBy string,
	expiresIn time.Duration,
) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return ErrMissingEmail
	}
	role, err := NormalizeRole(role)
	if err != nil {
		return err
	}
	if role == RoleTV {
		return ErrTVInviteForbidden
	}
	if expiresIn <= 0 {
		expiresIn = s.inviteTTL()
	}

	userID, err := s.ensureInviteStubUser(ctx, email, role)
	if err != nil {
		return err
	}

	return s.issueInviteToken(ctx, userID, email, role, invitedBy, expiresIn)
}

func (s *Service) ensureInviteStubUser(ctx context.Context, email, role string) (string, error) {
	existing, _, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		pending, listErr := s.store.ListPendingInvites(ctx, email)
		if listErr != nil {
			return "", fmt.Errorf("list pending invites: %w", listErr)
		}
		if len(pending) == 0 && existing.Enabled {
			return "", ErrUserExists
		}
		if len(pending) == 0 && !existing.Enabled {
			// Disabled non-invite account: allow re-invite by attaching invite token.
			return existing.ID, nil
		}

		return existing.ID, nil
	}

	hash, hashErr := unusablePasswordHash()
	if hashErr != nil {
		return "", hashErr
	}

	user := User{
		ID:                 "",
		Email:              email,
		Role:               role,
		Enabled:            false,
		EmailVerifiedAt:    nil,
		MustChangePassword: false,
	}
	createErr := s.store.CreateUser(ctx, user, hash)
	if createErr != nil {
		return "", fmt.Errorf("create invite stub user: %w", createErr)
	}

	created, _, loadErr := s.store.GetUserByEmail(ctx, email)
	if loadErr != nil {
		return "", fmt.Errorf("load invite stub user: %w", loadErr)
	}

	return created.ID, nil
}

func (s *Service) issueInviteToken(
	ctx context.Context,
	userID, email, role, invitedBy string,
	expiresIn time.Duration,
) error {
	rawToken, tokenHash, err := newSessionToken()
	if err != nil {
		return fmt.Errorf("create invite token: %w", err)
	}

	err = s.store.InvalidateUnusedTokens(ctx, email, TokenPurposeInvite)
	if err != nil {
		return fmt.Errorf("invalidate invite tokens: %w", err)
	}

	err = s.store.CreateAuthToken(ctx, Token{
		ID:        "",
		UserID:    userID,
		Email:     email,
		Purpose:   TokenPurposeInvite,
		TokenHash: tokenHash,
		Role:      role,
		InvitedBy: invitedBy,
		ExpiresAt: time.Now().UTC().Add(expiresIn),
		UsedAt:    nil,
	})
	if err != nil {
		return fmt.Errorf("store invite token: %w", err)
	}

	return s.sendInviteEmail(ctx, email, rawToken, expiresIn)
}

func (s *Service) sendInviteEmail(
	ctx context.Context,
	email, rawToken string,
	expiresIn time.Duration,
) error {
	link := s.appBaseURL() + "/accept-invite?token=" + rawToken
	subject := "You're invited to sudoStream"
	body := fmt.Sprintf(
		"Accept your invite and set a password (expires in %s):\n\n%s",
		expiresIn.Round(time.Minute),
		link,
	)

	err := s.mail.Send(ctx, email, subject, body)
	if err != nil {
		return fmt.Errorf("send invite email: %w", err)
	}

	return nil
}

// AcceptInvite sets a password on the invite stub (or creates the user) and activates
// the account. Opening the invite link already proves email ownership, so the account
// is enabled and marked verified regardless of EmailConfirmationRequired.
func (s *Service) AcceptInvite(
	ctx context.Context,
	rawToken, password string,
) error {
	token, err := s.validateToken(ctx, rawToken, TokenPurposeInvite)
	if err != nil {
		return err
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	role := token.Role
	if role == "" {
		role = RoleUser
	}

	existing, err := s.loadOrCreateInviteUser(ctx, token.Email, role, hash)
	if err != nil {
		return err
	}

	err = s.store.InvalidateUnusedTokens(ctx, token.Email, TokenPurposeInvite)
	if err != nil {
		return fmt.Errorf("invalidate invite tokens: %w", err)
	}

	err = s.store.EnableUser(ctx, existing.ID, true)
	if err != nil {
		return fmt.Errorf("enable invited user: %w", err)
	}

	return nil
}

func (s *Service) loadOrCreateInviteUser(
	ctx context.Context,
	email, role, passwordHash string,
) (*User, error) {
	existing, _, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		user := User{
			ID:                 "",
			Email:              email,
			Role:               role,
			Enabled:            true,
			EmailVerifiedAt:    nil,
			MustChangePassword: false,
		}
		err = s.store.CreateUser(ctx, user, passwordHash)
		if err != nil {
			return nil, fmt.Errorf("create invited user: %w", err)
		}
		existing, _, err = s.store.GetUserByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("load invited user: %w", err)
		}

		return existing, nil
	}

	err = s.store.UpdatePasswordHash(ctx, existing.ID, passwordHash, false)
	if err != nil {
		return nil, fmt.Errorf("set invited password: %w", err)
	}
	if role != existing.Role {
		err = s.store.UpdateUser(
			ctx,
			existing.ID,
			UserPatch{Role: &role},
		)
		if err != nil {
			return nil, fmt.Errorf("set invited role: %w", err)
		}
	}

	return existing, nil
}

// ChangePassword updates the authenticated user's password and re-issues session tokens
// so claims such as mustChangePassword update without a full re-login.
// When mustChangePassword is set, currentPassword may be empty (password was just used at login).
// When 2FA is enabled, totpCode is required.
func (s *Service) ChangePassword( //nolint:cyclop // password + optional 2FA + session reissue
	ctx context.Context,
	userID, currentPassword, newPassword, totpCode string,
	meta SessionMeta,
) (PublicUser, string, string, error) {
	user, hash, err := s.store.GetUserWithPasswordByID(ctx, userID)
	if err != nil {
		return PublicUser{}, "", "", ErrInvalidCredentials
	}

	if !user.MustChangePassword {
		err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(currentPassword))
		if err != nil {
			return PublicUser{}, "", "", ErrInvalidCredentials
		}
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(newPassword))
	if err == nil {
		return PublicUser{}, "", "", ErrPasswordUnchanged
	}

	has2FA, err := s.store.UserHasTOTP(ctx, userID)
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("check 2fa: %w", err)
	}
	if has2FA {
		err = s.requireUserTOTP(ctx, userID, totpCode)
		if err != nil {
			return PublicUser{}, "", "", err
		}
	}

	newHash, err := HashPassword(newPassword)
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("hash password: %w", err)
	}

	err = s.store.UpdatePasswordHash(ctx, userID, newHash, false)
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("update password: %w", err)
	}

	err = s.store.DeleteAllSessionsByUserID(ctx, userID)
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("revoke sessions: %w", err)
	}

	user.MustChangePassword = false

	return s.issueSession(ctx, user, meta)
}

func (s *Service) consumeToken(ctx context.Context, rawToken, purpose string) (*Token, error) {
	token, err := s.validateToken(ctx, rawToken, purpose)
	if err != nil {
		return nil, err
	}

	err = s.store.MarkAuthTokenUsed(ctx, token.ID)
	if err != nil {
		return nil, fmt.Errorf("mark token used: %w", err)
	}

	return token, nil
}

//nolint:dupl // mirrors issueChangeEmailToken with confirm_email purpose and copy
func (s *Service) sendConfirmEmail(
	ctx context.Context,
	userID, email string,
) error {
	rawConfirm, confirmHash, err := newSessionToken()
	if err != nil {
		return fmt.Errorf("create confirm token: %w", err)
	}

	confirmTTL := s.confirmEmailTTL()

	err = s.store.CreateAuthToken(ctx, Token{
		ID:        "",
		UserID:    userID,
		Email:     email,
		Purpose:   TokenPurposeConfirmEmail,
		TokenHash: confirmHash,
		Role:      "",
		InvitedBy: "",
		ExpiresAt: time.Now().UTC().Add(confirmTTL),
		UsedAt:    nil,
	})
	if err != nil {
		return fmt.Errorf("store confirm token: %w", err)
	}

	link := s.appBaseURL() + "/confirm-email?token=" + rawConfirm
	subject := "Confirm your sudoStream email"
	body := fmt.Sprintf(
		"Confirm your email address (expires in %s):\n\n%s",
		confirmTTL.Round(time.Minute),
		link,
	)

	err = s.mail.Send(ctx, email, subject, body)
	if err != nil {
		return fmt.Errorf("send confirm email: %w", err)
	}

	return nil
}

func (s *Service) validateToken(ctx context.Context, rawToken, purpose string) (*Token, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, ErrInvalidToken
	}

	token, err := s.store.GetAuthTokenByHash(ctx, hashToken(rawToken), purpose)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if token.UsedAt != nil {
		return nil, ErrTokenUsed
	}
	if time.Now().UTC().After(token.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return token, nil
}
