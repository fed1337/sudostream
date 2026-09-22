package httpapi

const (
	errAccessControlUnavailable = "access control unavailable"
	errChangePasswordFailed     = "change password failed"
	errInvalidCurrentPassword   = "invalid current password"
	errPasswordUnchanged        = "new password must differ from the current password"
	errTwoFactorRequired        = "two factor required"
	errInvalidTwoFactorCode     = "invalid two factor code"
	errListLibrariesFailed      = "list libraries failed"
	errListSessionsFailed       = "list sessions failed"
	errLoadGrantsFailed         = "load grants failed"
	errRevokeSessionFailed      = "revoke session failed"

	jsonKeySessions    = "sessions"
	jsonKeyLibraryID   = "libraryId"
	jsonKeyPermissions = "permissions"
	jsonKeyRead        = "read"

	authStatus2FARequired      = "2fa_required"
	authStatus2FASetupRequired = "2fa_setup_required"

	healthStatusDegraded = "degraded"

	oauthTokenTypeBearer     = "Bearer"
	oauthGrantTypeUnknown    = "unknown"
	oauthFormKeyGrantType    = "grant_type"
	oauthFormKeyRefreshToken = "refresh_token"

	libraryTypeUnknown = "unknown"
)
