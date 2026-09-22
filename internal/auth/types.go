package auth

import "time"

const (
	// TokenPurposeInvite creates a new user account.
	TokenPurposeInvite = "invite"
	// TokenPurposeResetPassword resets a forgotten password.
	TokenPurposeResetPassword = "reset_password"
	// TokenPurposeConfirmEmail verifies an email address.
	TokenPurposeConfirmEmail = "confirm_email"
	// TokenPurposeLogin2FA completes login after password verification.
	TokenPurposeLogin2FA = "login_2fa"
	// TokenPurposeChangeEmail confirms a self-service email change.
	TokenPurposeChangeEmail = "change_email"

	// UserStatusInvited is a stub account awaiting invite acceptance.
	UserStatusInvited = "invited"
	// UserStatusUnconfirmed is an account that still needs email confirmation.
	UserStatusUnconfirmed = "unconfirmed"
	// UserStatusActive is an enabled, verified account.
	UserStatusActive = "active"
	// UserStatusDisabled is a disabled account (not pending invite/confirm).
	UserStatusDisabled = "disabled"

	defaultLogin2FATTL        = 5 * time.Minute
	defaultTwoFactorLeeway    = 30
	defaultTokenTTLHours      = 24
	defaultAppBaseURL         = "http://localhost:8080"
	maxTwoFactorLeewaySeconds = 120
	minTwoFactorLeewaySeconds = 0
	backupCodeCount           = 8
	backupCodeLength          = 8
	backupCodeAlphabet        = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	totpPeriodSeconds         = 30
)

// Settings holds admin-editable auth policy flags (DB). TTLs and base URL come from env/runtime Config.
type Settings struct {
	EmailConfirmationRequired bool `json:"emailConfirmationRequired"`
	TwoFactorRequired         bool `json:"twoFactorRequired"`
}

// AdminUser is a user row exposed to administrators.
type AdminUser struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	Role               string     `json:"role"`
	Enabled            bool       `json:"enabled"`
	EmailVerified      bool       `json:"emailVerified"`
	Status             string     `json:"status"`
	MustChangePassword bool       `json:"mustChangePassword"`
	Has2FA             bool       `json:"has2FA"` //nolint:tagliatelle // matches frontend API contract
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

// UserPatch updates mutable user fields.
type UserPatch struct {
	Enabled *bool
	Role    *string
	Email   *string
}

// Token is a single-use emailed or login-pending token.
type Token struct {
	ID        string
	UserID    string
	Email     string
	Purpose   string
	TokenHash string
	Role      string
	InvitedBy string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// InviteView is a pending invite exposed to administrators.
type InviteView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
	Expired   bool      `json:"expired"`
}

// SessionView is a session row exposed to users and admins.
type SessionView struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"userAgent"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Current   bool      `json:"current"`
}

// LoginOutcome describes the result of a password login attempt.
type LoginOutcome struct {
	Status       string
	User         PublicUser
	AccessToken  string
	RefreshToken string
	PendingToken string
}

// TwoFactorSetup holds enrollment data before confirmation.
type TwoFactorSetup struct {
	Secret     string `json:"secret"`
	OtpauthURL string `json:"otpauthURL"` //nolint:tagliatelle // matches frontend API contract
}

// TwoFactorConfirmResult returns backup codes after enrollment.
type TwoFactorConfirmResult struct {
	BackupCodes []string `json:"backupCodes"`
}

// CreateUserInput configures a direct admin-created user.
type CreateUserInput struct {
	Email    string
	Password string
	Role     string
}

// TOTPRecord is stored 2FA state for a user.
type TOTPRecord struct {
	UserID          string
	SecretEncrypted string
	EnabledAt       time.Time
	BackupCodesHash []string
}
