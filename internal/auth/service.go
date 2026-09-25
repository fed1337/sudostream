// Package auth provides user authentication and session management.
//
// The package is transport-agnostic: HTTP adapters live in internal/httpapi,
// persistence behind Store, and token signing behind TokenIssuer. Host applications
// wire concrete adapters at the composition root (cmd/server).
//
// Host-owned extension points:
//   - Store — users, sessions, MFA, auth tokens, policy settings
//   - TokenIssuer — JWT access + opaque refresh issuance (ginjwt.Issuer in sudoStream)
//   - email.Mailer — invite, confirm, reset notifications (injected via Service deps)
//   - ExternalIdentityMapper — future OIDC relying-party hook; absent means deny federation
//   - Observability — metrics and structured logs wired at the httpapi boundary
//
// Library ACL (internal/access) stays outside this package.
//
//nolint:funcorder // login helpers stay near Login for readability.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"sudoStream/internal/email"
	"sudoStream/internal/observability"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	// RoleAdmin grants full administrative access.
	RoleAdmin = "admin"
	// RoleUser is the standard authenticated role.
	RoleUser = "user"
	// RoleTV is defined in roles.go (DLNA principal; no web login).

	bcryptCost        = 12
	bcryptCostTest    = 4
	sessionTokenBytes = 32
)

var (
	// ErrInvalidCredentials is returned when login credentials do not match.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserDisabled is returned when a disabled user attempts to authenticate.
	ErrUserDisabled = errors.New("user disabled")
	// ErrInvitePending is returned when a stub invite account tries to log in
	// before accepting the invite (no usable password yet).
	ErrInvitePending = errors.New("invite pending")
	// ErrSessionNotFound is returned when no session matches the provided token.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionExpired is returned when a session has expired.
	ErrSessionExpired = errors.New("session expired")
	// ErrIssuerRequired is returned when auth service wiring omits a token issuer.
	ErrIssuerRequired = errors.New("token issuer is required")
)

// User is an authenticated account.
type User struct {
	ID                 string
	Email              string
	Role               string
	Enabled            bool
	EmailVerifiedAt    *time.Time
	MustChangePassword bool
}

// PublicUser is returned by the API.
type PublicUser struct {
	ID                 string `json:"id"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"mustChangePassword"`
	Has2FA             bool   `json:"has2FA,omitempty"` //nolint:tagliatelle // matches frontend API contract
}

// SessionMeta captures client metadata for audit fields.
type SessionMeta struct {
	IP        string
	UserAgent string
}

// Store persists users and sessions.
//
//nolint:interfacebloat // single postgres implementation; splitting would obscure the boundary.
type Store interface {
	CountUsers(ctx context.Context) (int, error)
	CountSessions(ctx context.Context) (int64, error)
	CountAdmins(ctx context.Context) (int, error)
	CreateUser(ctx context.Context, user User, passwordHash string) error
	GetUserByEmail(ctx context.Context, email string) (*User, string, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserWithPasswordByID(ctx context.Context, id string) (*User, string, error)
	ListUsers(ctx context.Context) ([]AdminUser, error)
	UpdateUser(ctx context.Context, userID string, patch UserPatch) error
	UpdateLastLogin(ctx context.Context, userID string, at time.Time) error
	UpdatePasswordHash(
		ctx context.Context,
		userID, passwordHash string,
		mustChangePassword bool,
	) error
	UpdateUserEmail(ctx context.Context, userID, email string, clearVerified bool) error
	EnableUser(ctx context.Context, userID string, markEmailVerified bool) error
	ClearEmailVerified(ctx context.Context, userID string) error
	DeleteUser(ctx context.Context, userID string) error
	HasUnusedAuthToken(ctx context.Context, email, purpose string) (bool, error)
	GetUnusedAuthTokenByUser(
		ctx context.Context,
		userID, purpose string,
	) (*Token, error)
	GetSettings(ctx context.Context) (Settings, error)
	SaveSettings(ctx context.Context, settings Settings) error
	CreateAuthToken(ctx context.Context, token Token) error
	GetAuthTokenByHash(ctx context.Context, tokenHash, purpose string) (*Token, error)
	MarkAuthTokenUsed(ctx context.Context, tokenID string) error
	InvalidateUnusedTokens(ctx context.Context, email, purpose string) error
	CreateSession(
		ctx context.Context,
		sessionID, userID, tokenHash string,
		expiresAt time.Time,
		meta SessionMeta,
	) error
	UpdateSessionRefreshToken(
		ctx context.Context,
		sessionID, tokenHash string,
		expiresAt time.Time,
	) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*SessionRecord, error)
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
	DeleteAllSessionsByUserID(ctx context.Context, userID string) error
	DeleteSessionsExcept(ctx context.Context, userID, keepSessionID string) error
	ListSessionsByUserID(ctx context.Context, userID string) ([]SessionRecord, error)
	GetSessionByID(ctx context.Context, sessionID string) (*SessionRecord, error)
	DeleteSessionByID(ctx context.Context, sessionID string) error
	ListPendingInvites(ctx context.Context, email string) ([]Token, error)
	GetAuthTokenByID(ctx context.Context, id string) (*Token, error)
	UserHasTOTP(ctx context.Context, userID string) (bool, error)
	GetUserTOTP(ctx context.Context, userID string) (*TOTPRecord, error)
	SavePendingTOTPSecret(ctx context.Context, userID, secret string) error
	GetPendingTOTPSecret(ctx context.Context, userID string) (string, error)
	EnableUserTOTP(ctx context.Context, userID, secretEncrypted string, backupHashes []string) error
	DeleteUserTOTP(ctx context.Context, userID string) error
	ConsumeBackupCode(ctx context.Context, userID string, index int) error
}

// SessionRecord is a stored session row.
type SessionRecord struct {
	ID        string
	UserID    string
	TokenHash string
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Config configures the auth service (JWT/TOTP secrets + operator runtime from env).
type Config struct {
	TOTPEncryptionKey []byte
	JWTSecret         []byte
	AccessTTL         time.Duration
	RefreshTTL        time.Duration

	BaseURL               string
	InviteTTLHours        int
	ConfirmEmailTTLHours  int
	ResetPasswordTTLHours int
	TOTPLeewaySeconds     int
}

// Service handles login, logout, and session validation.
type Service struct {
	store  Store
	config Config
	mail   email.Sender
	issuer TokenIssuer
}

// NewService constructs an auth service.
func NewService(
	store Store,
	config Config,
	mail email.Sender,
	issuer TokenIssuer,
) (*Service, error) {
	if issuer == nil {
		return nil, ErrIssuerRequired
	}
	if len(config.TOTPEncryptionKey) == 0 {
		config.TOTPEncryptionKey = devTOTPEncryptionKey()
	}
	if mail == nil {
		mail = email.LogSender{}
	}
	config = normalizeConfig(config)

	return &Service{
		store:  store,
		config: config,
		mail:   mail,
		issuer: issuer,
	}, nil
}

// Issuer returns the token issuer used for session JWTs.
//
//nolint:ireturn // callers type-assert to ginjwt.Issuer for HTTP middleware.
func (s *Service) Issuer() TokenIssuer {
	return s.issuer
}

// RefreshTTL returns configured refresh token lifetime.
func (s *Service) RefreshTTL() time.Duration {
	return s.issuer.RefreshTTL()
}

// AccessTTL returns configured access token lifetime.
func (s *Service) AccessTTL() time.Duration {
	return s.issuer.AccessTTL()
}

// CountSessions returns how many refresh sessions are stored.
func (s *Service) CountSessions(ctx context.Context) (int64, error) {
	n, err := s.store.CountSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}

	return n, nil
}

// Login validates credentials and either creates a session or requests 2FA.
func (s *Service) Login(
	ctx context.Context,
	email, password string,
	meta SessionMeta,
) (LoginOutcome, error) {
	user, hash, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		observability.RecordAuthLoginAttempt("invalid_credentials")

		return LoginOutcome{}, ErrInvalidCredentials
	}

	// Invite stubs have no usable password — never compare, never allow login.
	inviteErr := s.rejectIfInvitePending(ctx, user)
	if inviteErr != nil {
		return LoginOutcome{}, inviteErr
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		observability.RecordAuthLoginAttempt("invalid_credentials")

		return LoginOutcome{}, ErrInvalidCredentials
	}

	if !AllowsWebLogin(user.Role) {
		observability.RecordAuthLoginAttempt("tv_web_denied")

		return LoginOutcome{}, ErrTVWebAuth
	}

	settings := s.loadSettings(ctx)
	needsConfirm := settings.EmailConfirmationRequired && user.EmailVerifiedAt == nil
	if !user.Enabled || needsConfirm {
		s.ensureConfirmEmailSent(ctx, user)
		observability.RecordAuthLoginAttempt("disabled")

		return LoginOutcome{}, ErrUserDisabled
	}

	return s.loginAfterPassword(ctx, user, meta)
}

// rejectIfInvitePending blocks login for unused invite stubs and refreshes
// expired invite tokens so the user gets a new email.
func (s *Service) rejectIfInvitePending(ctx context.Context, user *User) error {
	if user.Enabled {
		return nil
	}

	pending, err := s.store.ListPendingInvites(ctx, user.Email)
	if err != nil {
		return fmt.Errorf("list pending invites: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	invite := pending[0]
	if time.Now().UTC().After(invite.ExpiresAt) {
		_ = s.ResendInvite(ctx, invite.ID, invite.InvitedBy, 0)
	}

	observability.RecordAuthLoginAttempt("invite_pending")

	return ErrInvitePending
}

// ensureConfirmEmailSent (re)sends a confirmation or change-email link when
// email confirmation is required and no unused token is already outstanding.
func (s *Service) ensureConfirmEmailSent(ctx context.Context, user *User) {
	settings := s.loadSettings(ctx)
	if !settings.EmailConfirmationRequired {
		return
	}
	if user.EmailVerifiedAt != nil {
		return
	}

	_, changeErr := s.store.GetUnusedAuthTokenByUser(ctx, user.ID, TokenPurposeChangeEmail)
	if changeErr == nil {
		return
	}

	hasConfirm, err := s.store.HasUnusedAuthToken(ctx, user.Email, TokenPurposeConfirmEmail)
	if err == nil && hasConfirm {
		return
	}

	pending, err := s.store.ListPendingInvites(ctx, user.Email)
	if err == nil && len(pending) > 0 {
		_ = s.ResendInvite(ctx, pending[0].ID, pending[0].InvitedBy, 0)

		return
	}

	_ = s.store.InvalidateUnusedTokens(ctx, user.Email, TokenPurposeConfirmEmail)
	_ = s.sendConfirmEmail(ctx, user.ID, user.Email)
}

func (s *Service) loginAfterPassword(
	ctx context.Context,
	user *User,
	meta SessionMeta,
) (LoginOutcome, error) {
	settings := s.loadSettings(ctx)

	hasTOTP, err := s.store.UserHasTOTP(ctx, user.ID)
	if err != nil {
		return LoginOutcome{}, fmt.Errorf("check totp: %w", err)
	}

	if hasTOTP || settings.TwoFactorRequired {
		return s.pendingTwoFactorOutcome(ctx, user, hasTOTP, settings.TwoFactorRequired)
	}

	return s.issueLoginOutcome(ctx, user, meta)
}

func (s *Service) issueLoginOutcome(
	ctx context.Context,
	user *User,
	meta SessionMeta,
) (LoginOutcome, error) {
	publicUser, accessToken, refreshToken, err := s.issueSession(ctx, user, meta)
	if err != nil {
		observability.RecordAuthLoginAttempt("session_error")

		return LoginOutcome{}, err
	}

	observability.RecordAuthLoginAttempt("success")

	return LoginOutcome{
		Status:       "ok",
		User:         publicUser,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		PendingToken: "",
	}, nil
}

func (s *Service) pendingTwoFactorOutcome(
	ctx context.Context,
	user *User,
	hasTOTP, required bool,
) (LoginOutcome, error) {
	pendingToken, err := s.createPendingLoginToken(ctx, user)
	if err != nil {
		observability.RecordAuthLoginAttempt("session_error")

		return LoginOutcome{}, err
	}

	status := "2fa_required"
	if required && !hasTOTP {
		status = "2fa_setup_required"
	}
	observability.RecordAuthLoginAttempt(status)

	return LoginOutcome{
		Status:       status,
		User:         user.Public(),
		PendingToken: pendingToken,
		AccessToken:  "",
		RefreshToken: "",
	}, nil
}

// Logout removes a refresh session by raw refresh token.
func (s *Service) Logout(_ context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}

	err := s.issuer.RevokeRefresh(rawRefreshToken)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// Refresh issues a new access token and rotated refresh token.
func (s *Service) Refresh(
	ctx context.Context,
	rawRefreshToken string,
) (PublicUser, string, string, error) {
	if rawRefreshToken == "" {
		return PublicUser{}, "", "", ErrSessionNotFound
	}

	identity, pair, err := s.issuer.RefreshWithRotation(rawRefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, ErrSessionNotFound),
			errors.Is(err, ErrSessionExpired),
			errors.Is(err, ErrUserDisabled):
			return PublicUser{}, "", "", fmt.Errorf("refresh session: %w", err)
		default:
			return PublicUser{}, "", "", fmt.Errorf("refresh session: %w", ErrSessionNotFound)
		}
	}

	_ = ctx

	return identity.User, pair.AccessToken, pair.RefreshToken, nil
}

// SeedAdmin creates the first admin user when the database is empty.
func (s *Service) SeedAdmin(ctx context.Context, email, password string) error {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	user := User{
		ID:                 "",
		Email:              email,
		Role:               RoleAdmin,
		Enabled:            true,
		EmailVerifiedAt:    nil,
		MustChangePassword: true,
	}

	err = s.store.CreateUser(ctx, user, hash)
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	created, _, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("load seeded admin: %w", err)
	}

	err = s.store.EnableUser(ctx, created.ID, true)
	if err != nil {
		return fmt.Errorf("enable seeded admin: %w", err)
	}

	log.Printf("seeded default admin user %s (must_change_password=true)", email)

	return nil
}

// HashPassword hashes a plaintext password with bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), effectiveBcryptCost())
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}

	return string(hash), nil
}

func effectiveBcryptCost() int {
	if os.Getenv("GIN_MODE") == "test" {
		return bcryptCostTest
	}

	return bcryptCost
}

// Public converts a domain user into an API-safe representation.
func (u User) Public() PublicUser {
	return PublicUser{
		ID:                 u.ID,
		Email:              u.Email,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
	}
}

func newSessionToken() (string, string, error) {
	buf := make([]byte, sessionTokenBytes)
	_, err := rand.Read(buf)
	if err != nil {
		return "", "", fmt.Errorf("read random bytes: %w", err)
	}

	raw := base64.RawURLEncoding.EncodeToString(buf)

	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))

	return hex.EncodeToString(sum[:])
}

// HashToken returns the stored hash for a raw session or auth token.
func HashToken(raw string) string {
	return hashToken(raw)
}

func (s *Service) issueSession(
	ctx context.Context,
	user *User,
	meta SessionMeta,
) (PublicUser, string, string, error) {
	sessionID, err := newSessionID()
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("create session id: %w", err)
	}

	identity := SessionIdentity{
		SessionID: sessionID,
		UserID:    user.ID,
		User:      user.Public(),
		Meta:      meta,
	}

	pair, err := s.issuer.IssueTokens(identity)
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("issue tokens: %w", err)
	}

	err = s.store.UpdateLastLogin(ctx, user.ID, time.Now().UTC())
	if err != nil {
		return PublicUser{}, "", "", fmt.Errorf("update last login: %w", err)
	}

	return user.Public(), pair.AccessToken, pair.RefreshToken, nil
}

func newSessionID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	return id.String(), nil
}

func (s *Service) createPendingLoginToken(ctx context.Context, user *User) (string, error) {
	rawToken, tokenHash, err := newSessionToken()
	if err != nil {
		return "", fmt.Errorf("create pending login token: %w", err)
	}

	err = s.store.CreateAuthToken(ctx, Token{
		UserID:    user.ID,
		Email:     user.Email,
		Purpose:   TokenPurposeLogin2FA,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(defaultLogin2FATTL),
	})
	if err != nil {
		return "", fmt.Errorf("store pending login token: %w", err)
	}

	return rawToken, nil
}

func devTOTPEncryptionKey() []byte {
	// Dev-only fallback; production must set TOTP_ENCRYPTION_KEY.
	return []byte("01234567890123456789012345678901")
}
