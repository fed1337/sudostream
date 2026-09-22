package auth

import "errors"

const (
	// RoleTV is a non-interactive DLNA principal (no web login).
	RoleTV = "tv"
)

var (
	// ErrInvalidRole is returned when a role string is not recognized.
	ErrInvalidRole = errors.New("invalid role")
	// ErrTVWebAuth is returned when a TV principal attempts web authentication.
	ErrTVWebAuth = errors.New("tv accounts cannot use web authentication")
	// ErrTVInviteForbidden is returned when inviting a TV role (admin-create only).
	ErrTVInviteForbidden = errors.New("tv accounts cannot be invited")
)

// ValidRole reports whether role is a known account role.
func ValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleUser, RoleTV:
		return true
	default:
		return false
	}
}

// AllowsWebLogin reports whether the role may obtain web JWT sessions.
func AllowsWebLogin(role string) bool {
	return role != RoleTV
}

// NormalizeRole trims and defaults empty role to user; returns ErrInvalidRole when unknown.
func NormalizeRole(role string) (string, error) {
	if role == "" {
		return RoleUser, nil
	}
	if !ValidRole(role) {
		return "", ErrInvalidRole
	}

	return role, nil
}
