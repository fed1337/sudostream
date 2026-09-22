package httpapi

import "sudoStream/internal/auth"

// LoginRequest is the JSON body for POST /api/auth/login.
type LoginRequest struct {
	Email    string `example:"admin@localhost.lan" json:"email"`
	Password string `                              json:"password"`
}

// LoginResponse is returned on successful login or a 2FA challenge.
type LoginResponse struct {
	Status       string          `json:"status"`
	AccessToken  string          `json:"accessToken,omitempty"`
	PendingToken string          `json:"pendingToken,omitempty"`
	User         auth.PublicUser `json:"user"`
}

// RefreshResponse is returned by POST /api/auth/refresh.
type RefreshResponse struct {
	AccessToken string          `json:"accessToken"`
	User        auth.PublicUser `json:"user"`
}

// MeResponse is returned by GET /api/auth/me.
type MeResponse struct {
	User auth.PublicUser `json:"user"`
}

// ForgotPasswordRequest is the JSON body for POST /api/auth/forgot-password.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest is the JSON body for POST /api/auth/reset-password.
type ResetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// AcceptInviteRequest is the JSON body for POST /api/auth/accept-invite.
type AcceptInviteRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// ChangePasswordRequest is the JSON body for POST /api/auth/change-password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword,omitempty"`
	NewPassword     string `json:"newPassword"`
	TOTP            string `json:"totp,omitempty"`
}

// SessionsResponse lists active sessions for the current user.
type SessionsResponse struct {
	Sessions []auth.SessionView `json:"sessions"`
}

// TwoFactorSetupRequest optionally supplies a pending login token.
type TwoFactorSetupRequest struct {
	PendingToken string `json:"pendingToken"`
}

// TwoFactorConfirmRequest confirms TOTP enrollment.
type TwoFactorConfirmRequest struct {
	Code         string `json:"code"`
	PendingToken string `json:"pendingToken"`
}

// VerifyTwoFactorRequest completes login after password verification.
type VerifyTwoFactorRequest struct {
	PendingToken string `json:"pendingToken"`
	Code         string `json:"code"`
}

// DisableTwoFactorRequest disables TOTP for the current user.
type DisableTwoFactorRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

// StatusResponse is a simple status payload.
type StatusResponse struct {
	Status string `json:"status"`
}

// AuthTokensResponse returns access and refresh credentials.
type AuthTokensResponse struct {
	AccessToken string          `json:"accessToken"`
	User        auth.PublicUser `json:"user"`
	BackupCodes []string        `json:"backupCodes,omitempty"`
}

// UsersResponse lists admin user rows.
type UsersResponse struct {
	Users []auth.AdminUser `json:"users"`
}

// UserResponse wraps a public user row.
type UserResponse struct {
	User auth.PublicUser `json:"user"`
}

// AdminUserResponse wraps a single admin user row.
type AdminUserResponse struct {
	User auth.AdminUser `json:"user"`
}

// CreateUserRequest creates a user directly from the admin API.
type CreateUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// UpdateUserRequest patches mutable user fields.
type UpdateUserRequest struct {
	Enabled *bool   `json:"enabled"`
	Role    *string `json:"role"`
	Email   *string `json:"email"`
}

// ChangeEmailRequest starts a self-service email change.
type ChangeEmailRequest struct {
	NewEmail string `json:"newEmail"`
	Password string `json:"password"`
	TOTP     string `json:"totp"`
}

// CreateInviteRequest creates an email invite.
type CreateInviteRequest struct {
	Email          string `json:"email"`
	Role           string `json:"role"`
	ExpiresInHours *int   `json:"expiresInHours"`
}

// InvitesResponse lists pending invites.
type InvitesResponse struct {
	Invites []auth.InviteView `json:"invites"`
}

// ResendInviteRequest optionally overrides invite TTL.
type ResendInviteRequest struct {
	ExpiresInHours *int `json:"expiresInHours"`
}
