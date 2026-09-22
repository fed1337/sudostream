package httpapi

import (
	"errors"
	"maps"
	"net/http"
	"strings"
	"sudoStream/internal/auth"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	refreshCookieName   = "refresh"
	userContextKey      = "authUser"
	sessionContextKey   = "authSessionID"
	authRequiredMessage = "authentication required"
	invalidRequestBody  = "invalid request body"
	responseUserKey     = "user"
	jsonStatusKey       = "status"
	userExistsMessage   = "user already exists"
	emailInUseMessage   = "email is already used"
	accessTokenKey      = "accessToken"
)

type authHandler struct {
	auth   *auth.Service
	config RouteConfig
}

// Login authenticates with email and password.
//
//	@Summary		Login
//	@Description	Authenticates a user. May return a 2FA challenge instead of tokens.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"Credentials"
//	@Success		200		{object}	LoginResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		500		{object}	ErrorResponse
//	@Router			/api/auth/login [post]
func (h *authHandler) login(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	outcome, err := h.auth.Login(
		c.Request.Context(),
		strings.TrimSpace(body.Email),
		body.Password,
		sessionMeta(c),
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid email or password"})
		case errors.Is(err, auth.ErrTVWebAuth):
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "tv accounts cannot use web login"})
		case errors.Is(err, auth.ErrInvitePending):
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "check your email to accept the invite"})
		case errors.Is(err, auth.ErrUserDisabled):
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "account disabled"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "login failed"})
		}

		return
	}

	switch outcome.Status {
	case authStatus2FARequired, authStatus2FASetupRequired:
		c.JSON(http.StatusOK, gin.H{
			jsonStatusKey:  outcome.Status,
			"pendingToken": outcome.PendingToken,
			"user":         outcome.User,
		})
	default:
		writeAuthTokens(
			c,
			h.config.CookieSecure,
			h.auth.RefreshTTL(),
			outcome.AccessToken,
			outcome.RefreshToken,
			outcome.User,
			gin.H{jsonStatusKey: "ok"},
		)
	}
}

// Refresh exchanges a refresh cookie for a new access token.
//
//	@Summary		Refresh access token
//	@Description	Reads the httpOnly refresh cookie and returns a new access token.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	RefreshResponse
//	@Failure		401	{object}	ErrorResponse
//	@Router			/api/auth/refresh [post]
func (h *authHandler) refresh(c *gin.Context) {
	rawRefresh, err := c.Cookie(refreshCookieName)
	if err != nil || rawRefresh == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	user, accessToken, refreshToken, err := h.auth.Refresh(c.Request.Context(), rawRefresh)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	setRefreshCookie(c, refreshToken, h.config.CookieSecure, h.auth.RefreshTTL())
	c.JSON(http.StatusOK, gin.H{
		accessTokenKey:  accessToken,
		responseUserKey: user,
	})
}

// Logout revokes the refresh session and clears the refresh cookie.
//
//	@Summary		Logout
//	@Description	Revokes the current refresh session.
//	@Tags			auth
//	@Success		204
//	@Failure		401	{object}	ErrorResponse
//	@Router			/api/auth/logout [post]
func (h *authHandler) logout(c *gin.Context) {
	rawRefresh, _ := c.Cookie(refreshCookieName)
	_ = h.auth.Logout(c.Request.Context(), rawRefresh)
	clearRefreshCookie(c, h.config.CookieSecure)
	c.Status(http.StatusNoContent)
}

// Me returns the authenticated user.
//
//	@Summary		Current user
//	@Description	Returns the authenticated user profile.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	MeResponse
//	@Failure		401	{object}	ErrorResponse
//	@Router			/api/auth/me [get]
func (h *authHandler) me(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	has2FA, err := h.auth.UserHas2FA(c.Request.Context(), user.ID)
	if err == nil {
		user.Has2FA = has2FA
	}

	c.JSON(http.StatusOK, gin.H{responseUserKey: user})
}

// ForgotPassword requests a password reset email.
//
//	@Summary		Forgot password
//	@Description	Always returns 204 to avoid email enumeration.
//	@Tags			auth
//	@Accept			json
//	@Param			body	body	ForgotPasswordRequest	true	"Email"
//	@Success		204
//	@Failure		400	{object}	ErrorResponse
//	@Router			/api/auth/forgot-password [post]
func (h *authHandler) forgotPassword(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	_ = h.auth.RequestPasswordReset(c.Request.Context(), body.Email)
	c.Status(http.StatusNoContent)
}

// ResetPassword sets a new password using a reset token.
//
//	@Summary	Reset password
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	ResetPasswordRequest	true	"Reset token"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Router		/api/auth/reset-password [post]
func (h *authHandler) resetPassword(c *gin.Context) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	err = h.auth.ResetPassword(c.Request.Context(), body.Token, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrPasswordUnchanged) {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: errPasswordUnchanged})

			return
		}

		writeTokenError(c, err)

		return
	}

	c.Status(http.StatusNoContent)
}

// ConfirmEmail verifies an email address from a token query parameter.
//
//	@Summary	Confirm email
//	@Tags		auth
//	@Produce	json
//	@Param		token	query		string	true	"Confirmation token"
//	@Success	200		{object}	StatusResponse
//	@Failure	400		{object}	ErrorResponse
//	@Router		/api/auth/confirm-email [get]
func (h *authHandler) confirmEmail(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	err := h.auth.ConfirmEmail(c.Request.Context(), token)
	if err != nil {
		writeTokenError(c, err)

		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "confirmed"})
}

// AcceptInvite creates an account from an invite token.
//
//	@Summary	Accept invite
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	AcceptInviteRequest	true	"Invite token"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	409	{object}	ErrorResponse
//	@Router		/api/auth/accept-invite [post]
func (h *authHandler) acceptInvite(c *gin.Context) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	err = h.auth.AcceptInvite(c.Request.Context(), body.Token, body.Password)
	if err != nil {
		writeTokenError(c, err)

		return
	}

	c.Status(http.StatusNoContent)
}

// ChangePassword updates the current user's password.
//
//	@Summary	Change password
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		ChangePasswordRequest	true	"Password change"
//	@Success	200		{object}	LoginResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	401		{object}	ErrorResponse
//	@Failure	500		{object}	ErrorResponse
//	@Router		/api/auth/change-password [post]
func (h *authHandler) changePassword(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		TOTP            string `json:"totp"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	publicUser, accessToken, refreshToken, err := h.auth.ChangePassword(
		c.Request.Context(),
		user.ID,
		body.CurrentPassword,
		body.NewPassword,
		body.TOTP,
		sessionMeta(c),
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errInvalidCurrentPassword})
		case errors.Is(err, auth.ErrTwoFactorRequired):
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errTwoFactorRequired})
		case errors.Is(err, auth.ErrInvalidTwoFactorCode):
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errInvalidTwoFactorCode})
		case errors.Is(err, auth.ErrPasswordUnchanged):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: errPasswordUnchanged})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errChangePasswordFailed})
		}

		return
	}

	writeAuthTokens(
		c,
		h.config.CookieSecure,
		h.auth.RefreshTTL(),
		accessToken,
		refreshToken,
		publicUser,
		gin.H{jsonStatusKey: "ok"},
	)
}

// ChangeEmail starts a self-service email change (confirm link sent to the new address).
//
//	@Summary	Request email change
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	ChangeEmailRequest	true	"Email change"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	401	{object}	ErrorResponse
//	@Failure	409	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/auth/change-email [post]
func (h *authHandler) changeEmail(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	var body struct {
		NewEmail string `json:"newEmail"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	err = h.auth.RequestChangeEmail(
		c.Request.Context(),
		user.ID,
		body.NewEmail,
		body.Password,
		body.TOTP,
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errInvalidCurrentPassword})
		case errors.Is(err, auth.ErrTwoFactorRequired):
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errTwoFactorRequired})
		case errors.Is(err, auth.ErrInvalidTwoFactorCode):
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errInvalidTwoFactorCode})
		case errors.Is(err, auth.ErrUserExists):
			c.JSON(http.StatusConflict, ErrorResponse{Error: emailInUseMessage})
		case errors.Is(err, auth.ErrMissingEmail):
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "email is required"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "change email failed"})
		}

		return
	}

	c.Status(http.StatusNoContent)
}

// ListSessions returns sessions for the current user.
//
//	@Summary	List sessions
//	@Tags		auth
//	@Produce	json
//	@Success	200	{object}	SessionsResponse
//	@Failure	401	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/auth/sessions [get]
func (h *authHandler) listSessions(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	sessionID := currentSessionID(c)
	sessions, err := h.auth.ListUserSessions(
		c.Request.Context(),
		user.ID,
		sessionID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errListSessionsFailed})

		return
	}

	c.JSON(http.StatusOK, gin.H{jsonKeySessions: sessions})
}

// RevokeSession revokes one of the current user's sessions.
//
//	@Summary	Revoke session
//	@Tags		auth
//	@Param		id	path	string	true	"Session ID"
//	@Success	204
//	@Failure	401	{object}	ErrorResponse
//	@Failure	403	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/auth/sessions/{id} [delete]
func (h *authHandler) revokeSession(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	sessionID := currentSessionID(c)
	isCurrent, err := h.auth.RevokeUserSession(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
		sessionID,
	)
	if err != nil {
		if errors.Is(err, auth.ErrForbidden) {
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden"})

			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errRevokeSessionFailed})

		return
	}

	if isCurrent {
		clearRefreshCookie(c, h.config.CookieSecure)
	}

	c.Status(http.StatusNoContent)
}

// RevokeAllSessions revokes every session for the current user.
//
//	@Summary	Revoke all sessions
//	@Tags		auth
//	@Success	204
//	@Failure	401	{object}	ErrorResponse
//	@Failure	500	{object}	ErrorResponse
//	@Router		/api/auth/sessions/revoke-all [post]
func (h *authHandler) revokeAllSessions(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	err := h.auth.RevokeAllUserSessions(c.Request.Context(), user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: errRevokeSessionFailed})

		return
	}

	clearRefreshCookie(c, h.config.CookieSecure)
	c.Status(http.StatusNoContent)
}

// SetupTwoFactor starts TOTP enrollment.
//
//	@Summary	Start 2FA setup
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		TwoFactorSetupRequest	false	"Optional pending token"
//	@Success	200		{object}	auth.TwoFactorSetup
//	@Failure	400		{object}	ErrorResponse
//	@Failure	401		{object}	ErrorResponse
//	@Failure	409		{object}	ErrorResponse
//	@Router		/api/auth/2fa/setup [post]
func (h *authHandler) setupTwoFactor(c *gin.Context) {
	var body struct {
		PendingToken string `json:"pendingToken"`
	}

	_ = c.ShouldBindJSON(&body)

	userID, email, ok := h.resolveTwoFactorUser(c, body.PendingToken)
	if !ok {
		return
	}

	setup, err := h.auth.StartTwoFactorSetup(c.Request.Context(), userID, email)
	if err != nil {
		writeTwoFactorError(c, err)

		return
	}

	c.JSON(http.StatusOK, setup)
}

// ConfirmTwoFactor completes TOTP enrollment.
//
//	@Summary	Confirm 2FA setup
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		TwoFactorConfirmRequest	true	"TOTP code"
//	@Success	200		{object}	auth.TwoFactorConfirmResult
//	@Failure	400		{object}	ErrorResponse
//	@Failure	401		{object}	ErrorResponse
//	@Router		/api/auth/2fa/confirm [post]
func (h *authHandler) confirmTwoFactor(c *gin.Context) {
	var body struct {
		Code         string `json:"code"`
		PendingToken string `json:"pendingToken"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	userID, _, ok := h.resolveTwoFactorUser(c, body.PendingToken)
	if !ok {
		return
	}

	if body.PendingToken != "" {
		result, accessToken, refreshToken, confirmResult, confirmErr := h.auth.CompletePendingTwoFactorSetup(
			c.Request.Context(),
			body.PendingToken,
			body.Code,
			sessionMeta(c),
		)
		if confirmErr != nil {
			writeTwoFactorError(c, confirmErr)

			return
		}

		writeAuthTokens(
			c,
			h.config.CookieSecure,
			h.auth.RefreshTTL(),
			accessToken,
			refreshToken,
			result,
			gin.H{"backupCodes": confirmResult.BackupCodes},
		)

		return
	}

	result, err := h.auth.ConfirmTwoFactorSetup(c.Request.Context(), userID, body.Code)
	if err != nil {
		writeTwoFactorError(c, err)

		return
	}

	c.JSON(http.StatusOK, result)
}

// VerifyTwoFactor completes login after a 2FA challenge.
//
//	@Summary	Verify 2FA login
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		VerifyTwoFactorRequest	true	"Pending token and code"
//	@Success	200		{object}	AuthTokensResponse
//	@Failure	400		{object}	ErrorResponse
//	@Failure	401		{object}	ErrorResponse
//	@Router		/api/auth/2fa/verify [post]
func (h *authHandler) verifyTwoFactor(c *gin.Context) {
	var body struct {
		PendingToken string `json:"pendingToken"`
		Code         string `json:"code"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	user, accessToken, refreshToken, err := h.auth.VerifyLoginTwoFactor(
		c.Request.Context(),
		body.PendingToken,
		body.Code,
		sessionMeta(c),
	)
	if err != nil {
		writeTwoFactorError(c, err)

		return
	}

	writeAuthTokens(
		c,
		h.config.CookieSecure,
		h.auth.RefreshTTL(),
		accessToken,
		refreshToken,
		user,
		gin.H{responseUserKey: user},
	)
}

// DisableTwoFactor removes TOTP from the current user.
//
//	@Summary	Disable 2FA
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	DisableTwoFactorRequest	true	"Password and TOTP code"
//	@Success	204
//	@Failure	400	{object}	ErrorResponse
//	@Failure	401	{object}	ErrorResponse
//	@Router		/api/auth/2fa [delete]
func (h *authHandler) disableTwoFactor(c *gin.Context) {
	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return
	}

	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: invalidRequestBody})

		return
	}

	err = h.auth.DisableTwoFactor(c.Request.Context(), user.ID, body.Password, body.Code)
	if err != nil {
		writeTwoFactorError(c, err)

		return
	}

	c.Status(http.StatusNoContent)
}

func (h *authHandler) resolveTwoFactorUser(
	c *gin.Context,
	pendingToken string,
) (string, string, bool) {
	if pendingToken != "" {
		user, err := h.auth.UserFromPendingLoginToken(c.Request.Context(), pendingToken)
		if err != nil {
			writeTokenError(c, err)

			return "", "", false
		}

		return user.ID, user.Email, true
	}

	user, ok := currentUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: authRequiredMessage})

		return "", "", false
	}

	return user.ID, user.Email, true
}

func writeTwoFactorError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid password"})
	case errors.Is(err, auth.ErrInvalidTwoFactorCode):
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: errInvalidTwoFactorCode})
	case errors.Is(err, auth.ErrTwoFactorAlreadyEnabled):
		c.JSON(http.StatusConflict, ErrorResponse{Error: "two factor already enabled"})
	case errors.Is(err, auth.ErrTwoFactorNotConfigured):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "two factor not configured"})
	default:
		writeTokenError(c, err)
	}
}

func writeTokenError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidToken):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid token"})
	case errors.Is(err, auth.ErrTokenExpired):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "token expired"})
	case errors.Is(err, auth.ErrTokenUsed):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "token already used"})
	case errors.Is(err, auth.ErrUserExists):
		c.JSON(http.StatusConflict, ErrorResponse{Error: userExistsMessage})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "request failed"})
	}
}

func currentUser(c *gin.Context) (auth.PublicUser, bool) {
	value, found := c.Get(userContextKey)
	if !found {
		return auth.PublicUser{
			ID:                 "",
			Email:              "",
			Role:               "",
			MustChangePassword: false,
		}, false
	}

	user, valid := value.(auth.PublicUser)
	if !valid {
		return auth.PublicUser{
			ID:                 "",
			Email:              "",
			Role:               "",
			MustChangePassword: false,
		}, false
	}

	return user, true
}

func currentSessionID(c *gin.Context) string {
	value, found := c.Get(sessionContextKey)
	if !found {
		return ""
	}

	sessionID, valid := value.(string)
	if !valid {
		return ""
	}

	return sessionID
}

func sessionMeta(c *gin.Context) auth.SessionMeta {
	return auth.SessionMeta{
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
}

func writeAuthTokens(
	c *gin.Context,
	secure bool,
	refreshTTL time.Duration,
	accessToken, refreshToken string,
	user auth.PublicUser,
	extra gin.H,
) {
	setRefreshCookie(c, refreshToken, secure, refreshTTL)
	payload := gin.H{
		accessTokenKey:  accessToken,
		responseUserKey: user,
	}
	maps.Copy(payload, extra)

	c.JSON(http.StatusOK, payload)
}

func setRefreshCookie(c *gin.Context, rawToken string, secure bool, ttl time.Duration) {
	c.SetSameSite(http.SameSiteLaxMode)
	maxAge := int(ttl.Seconds())
	if maxAge <= 0 {
		maxAge = int((7 * 24 * time.Hour).Seconds())
	}
	c.SetCookie(refreshCookieName, rawToken, maxAge, "/", "", secure, true)
}

func clearRefreshCookie(c *gin.Context, secure bool) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, "", -1, "/", "", secure, true)
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func accessToken(c *gin.Context) string {
	if token := bearerToken(c.GetHeader("Authorization")); token != "" {
		return token
	}

	return strings.TrimSpace(c.Query("access_token"))
}
