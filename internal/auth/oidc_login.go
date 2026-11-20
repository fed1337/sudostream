package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sudoStream/internal/observability"
	"time"
)

var errOIDCIdentityIncomplete = errors.New("oidc identity: missing issuer or subject")

// CompleteOIDCLogin maps/provisions a user from verified IdP claims and issues our session.
// Local 2FA is skipped (T1) — MFA belongs at the IdP.
func (s *Service) CompleteOIDCLogin(
	ctx context.Context,
	input OIDCLoginInput,
	meta SessionMeta,
) (LoginOutcome, error) {
	issuer := strings.TrimSpace(input.Issuer)
	subject := strings.TrimSpace(input.Subject)
	if issuer == "" || subject == "" {
		return LoginOutcome{}, errOIDCIdentityIncomplete
	}

	user, err := s.resolveOIDCUser(ctx, input)
	if err != nil {
		observability.RecordAuthLoginAttempt("oidc_denied")

		return LoginOutcome{}, err
	}

	if !AllowsWebLogin(user.Role) {
		observability.RecordAuthLoginAttempt("tv_web_denied")

		return LoginOutcome{}, ErrTVWebAuth
	}
	if !user.Enabled {
		observability.RecordAuthLoginAttempt("disabled")

		return LoginOutcome{}, ErrUserDisabled
	}

	err = s.applyOIDCAdminGroup(ctx, user, input)
	if err != nil {
		return LoginOutcome{}, err
	}

	identity := ExternalIdentity{
		UserID:  user.ID,
		Issuer:  issuer,
		Subject: subject,
		IDToken: input.RawIDToken,
	}
	err = s.store.UpsertExternalIdentity(ctx, identity)
	if err != nil {
		return LoginOutcome{}, fmt.Errorf("upsert external identity: %w", err)
	}

	return s.issueLoginOutcome(ctx, user, meta)
}

// LogoutWithHint revokes the refresh session and returns an IdP id_token_hint when available.
func (s *Service) LogoutWithHint(ctx context.Context, rawRefreshToken string) (OIDCLogoutHint, error) {
	hint := OIDCLogoutHint{}
	if rawRefreshToken == "" {
		return hint, nil
	}

	tokenHash := HashToken(rawRefreshToken)
	session, err := s.store.GetSessionByTokenHash(ctx, tokenHash)
	if err == nil && session != nil {
		identity, idErr := s.store.GetExternalIdentityByUserID(ctx, session.UserID)
		if idErr == nil && identity != nil {
			hint.IDTokenHint = identity.IDToken
		}
	}

	err = s.Logout(ctx, rawRefreshToken)
	if err != nil {
		return OIDCLogoutHint{}, err
	}

	return hint, nil
}

func (s *Service) resolveOIDCUser(ctx context.Context, input OIDCLoginInput) (*User, error) {
	user, err := s.userFromExternalSubject(ctx, input.Issuer, input.Subject)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, ErrExternalIdentityNotFound) {
		return nil, err
	}

	return s.provisionOIDCUser(ctx, input)
}

func (s *Service) userFromExternalSubject(ctx context.Context, issuer, subject string) (*User, error) {
	existing, err := s.store.GetExternalIdentityBySubject(ctx, issuer, subject)
	if err != nil {
		return nil, fmt.Errorf("lookup external identity: %w", err)
	}
	user, getErr := s.store.GetUserByID(ctx, existing.UserID)
	if getErr != nil {
		return nil, fmt.Errorf("load linked user: %w", getErr)
	}

	return user, nil
}

func (s *Service) provisionOIDCUser(ctx context.Context, input OIDCLoginInput) (*User, error) {
	err := s.ensureOIDCEmailPolicy(input)
	if err != nil {
		return nil, err
	}

	if input.MergeUsers {
		user, mergeErr := s.mergeOIDCUserByEmail(ctx, input)
		if mergeErr == nil {
			return user, nil
		}
		if !errors.Is(mergeErr, ErrExternalIdentityNotFound) {
			return nil, mergeErr
		}
	}

	if input.AutoRegister {
		return s.registerOIDCUser(ctx, input)
	}

	return nil, ErrOIDCUnknownIdentity
}

func (s *Service) ensureOIDCEmailPolicy(input OIDCLoginInput) error {
	if input.RequireEmailVerification && !input.EmailVerified {
		return ErrOIDCEmailNotVerified
	}
	if strings.TrimSpace(input.Email) == "" {
		return ErrOIDCEmailRequired
	}

	return nil
}

func (s *Service) mergeOIDCUserByEmail(ctx context.Context, input OIDCLoginInput) (*User, error) {
	user, _, err := s.store.GetUserByEmail(ctx, input.Email)
	if err != nil {
		return nil, ErrExternalIdentityNotFound
	}

	return user, nil
}

func (s *Service) registerOIDCUser(ctx context.Context, input OIDCLoginInput) (*User, error) {
	_, _, err := s.store.GetUserByEmail(ctx, input.Email)
	if err == nil {
		// Email already taken but merge was off / failed — do not hijack.
		return nil, ErrUserExists
	}

	hash, err := unusablePasswordHash()
	if err != nil {
		return nil, err
	}

	role := RoleUser
	if oidcHasAdminGroup(input.Groups, input.AdminGroup) {
		role = RoleAdmin
	}

	now := time.Now().UTC()
	var verifiedAt *time.Time
	if input.EmailVerified || !input.RequireEmailVerification {
		verifiedAt = &now
	}

	user := User{
		ID:                 "",
		Email:              input.Email,
		Role:               role,
		Enabled:            true,
		EmailVerifiedAt:    verifiedAt,
		MustChangePassword: false,
	}

	err = s.store.CreateUser(ctx, user, hash)
	if err != nil {
		return nil, fmt.Errorf("create oidc user: %w", err)
	}

	created, _, err := s.store.GetUserByEmail(ctx, input.Email)
	if err != nil {
		return nil, fmt.Errorf("load oidc user: %w", err)
	}

	if verifiedAt != nil {
		err = s.store.EnableUser(ctx, created.ID, true)
		if err != nil {
			return nil, fmt.Errorf("enable oidc user: %w", err)
		}
		created.Enabled = true
		created.EmailVerifiedAt = verifiedAt
	}

	return created, nil
}

func (s *Service) applyOIDCAdminGroup(ctx context.Context, user *User, input OIDCLoginInput) error {
	if !oidcHasAdminGroup(input.Groups, input.AdminGroup) {
		return nil
	}
	if user.Role == RoleAdmin {
		return nil
	}

	role := RoleAdmin
	err := s.store.UpdateUser(ctx, user.ID, UserPatch{Role: &role})
	if err != nil {
		return fmt.Errorf("elevate oidc admin: %w", err)
	}
	user.Role = RoleAdmin

	return nil
}

func oidcHasAdminGroup(groups []string, adminGroup string) bool {
	want := strings.TrimSpace(adminGroup)
	if want == "" {
		return false
	}
	for _, group := range groups {
		if strings.EqualFold(strings.TrimSpace(group), want) {
			return true
		}
	}

	return false
}
