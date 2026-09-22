package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/observability"

	"github.com/gin-gonic/gin"
)

const (
	oauthGrantPassword     = "password"
	oauthGrantRefreshToken = "refresh_token"
	oauthErrorInvalidGrant = "invalid_grant"
	oauthErrorInvalidReq   = "invalid_request"
	oauthErrorUnsupported  = "unsupported_grant_type"
	oauthErrorMFARequired  = "mfa_required"
)

// OAuthTokenResponse is returned on successful token issuance (RFC 6749).
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`            //nolint:tagliatelle // OAuth2 spec
	TokenType    string `json:"token_type"`              //nolint:tagliatelle // OAuth2 spec
	ExpiresIn    int    `json:"expires_in"`              //nolint:tagliatelle // OAuth2 spec
	RefreshToken string `json:"refresh_token,omitempty"` //nolint:tagliatelle // OAuth2 spec
}

// OAuthErrorResponse is returned for OAuth token endpoint errors (RFC 6749 field names).
type OAuthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"` //nolint:tagliatelle // OAuth2 spec
	PendingToken     string `json:"pending_token,omitempty"`     //nolint:tagliatelle // OAuth2 spec
}

type oauthHandler struct {
	auth   *auth.Service
	config RouteConfig
}

// OAuthToken issues access tokens via OAuth2 password or refresh_token grants.
//
//	@Summary	OAuth2 token
//	@Tags		oauth
//	@Accept		x-www-form-urlencoded
//	@Produce	json
//	@Success	200	{object}	OAuthTokenResponse
//	@Failure	400	{object}	OAuthErrorResponse
//	@Failure	401	{object}	OAuthErrorResponse
//	@Router		/api/oauth/token [post]
func (h *oauthHandler) token(c *gin.Context) {
	grantType := strings.TrimSpace(c.PostForm("grant_type"))
	if grantType == "" {
		h.oauthError(
			c,
			grantType,
			oauthErrorInvalidReq,
			"grant_type is required",
			http.StatusBadRequest,
		)

		return
	}

	switch grantType {
	case oauthGrantPassword:
		h.oauthPasswordGrant(c, grantType)
	case oauthGrantRefreshToken:
		h.oauthRefreshGrant(c, grantType)
	default:
		h.oauthError(
			c,
			grantType,
			oauthErrorUnsupported,
			"unsupported grant_type",
			http.StatusBadRequest,
		)
	}
}

func (h *oauthHandler) oauthPasswordGrant( //nolint:cyclop // credential + MFA branches
	c *gin.Context,
	grantType string,
) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")
	if username == "" || password == "" {
		h.oauthError(
			c,
			grantType,
			oauthErrorInvalidReq,
			"username and password are required",
			http.StatusBadRequest,
		)

		return
	}

	totpCode := strings.TrimSpace(c.PostForm("totp"))
	outcome, err := h.auth.LoginWithTOTP(
		c.Request.Context(),
		username,
		password,
		totpCode,
		sessionMeta(c),
	)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			h.oauthError(
				c,
				grantType,
				oauthErrorInvalidGrant,
				"invalid username or password",
				http.StatusUnauthorized,
			)
		case errors.Is(err, auth.ErrTVWebAuth):
			h.oauthError(
				c,
				grantType,
				oauthErrorInvalidGrant,
				"tv accounts cannot use web login",
				http.StatusForbidden,
			)
		case errors.Is(err, auth.ErrInvitePending):
			h.oauthError(
				c,
				grantType,
				oauthErrorInvalidGrant,
				"check your email to accept the invite",
				http.StatusForbidden,
			)
		case errors.Is(err, auth.ErrUserDisabled):
			h.oauthError(
				c,
				grantType,
				oauthErrorInvalidGrant,
				"account disabled",
				http.StatusUnauthorized,
			)
		case errors.Is(err, auth.ErrInvalidTwoFactorCode):
			h.oauthMFARequired(c, grantType, "invalid two-factor code", "")
		default:
			h.oauthError(
				c,
				grantType,
				oauthErrorInvalidGrant,
				"login failed",
				http.StatusBadRequest,
			)
		}

		return
	}

	switch outcome.Status {
	case authStatus2FARequired, authStatus2FASetupRequired:
		description := "two-factor authentication required"
		if outcome.Status == authStatus2FASetupRequired {
			description = "two-factor authentication setup required"
		}

		h.oauthMFARequired(c, grantType, description, outcome.PendingToken)

		return
	}

	h.writeOAuthTokens(c, grantType, "issue", outcome.AccessToken, outcome.RefreshToken)
}

func (h *oauthHandler) oauthMFARequired(
	c *gin.Context,
	grantType, description, pendingToken string,
) {
	observability.LogAction(
		c,
		"oauth.token.error",
		"grant_type", grantType,
		"error", oauthErrorMFARequired,
	)
	c.JSON(http.StatusBadRequest, OAuthErrorResponse{
		Error:            oauthErrorMFARequired,
		ErrorDescription: description,
		PendingToken:     pendingToken,
	})
}

func (h *oauthHandler) oauthRefreshGrant(c *gin.Context, grantType string) {
	rawRefresh := strings.TrimSpace(c.PostForm("refresh_token"))
	if rawRefresh == "" {
		h.oauthError(
			c,
			grantType,
			oauthErrorInvalidReq,
			"refresh_token is required",
			http.StatusBadRequest,
		)

		return
	}

	_, accessToken, refreshToken, err := h.auth.Refresh(c.Request.Context(), rawRefresh)
	if err != nil {
		h.oauthError(
			c,
			grantType,
			oauthErrorInvalidGrant,
			"invalid or expired refresh token",
			http.StatusUnauthorized,
		)

		return
	}

	h.writeOAuthTokens(c, grantType, "refresh", accessToken, refreshToken)
}

func (h *oauthHandler) writeOAuthTokens(
	c *gin.Context,
	grantType, action, accessToken, refreshToken string,
) {
	expiresIn := int(h.auth.AccessTTL().Seconds())
	if expiresIn <= 0 {
		expiresIn = 900
	}

	observability.LogAction(c, "oauth.token."+action, "grant_type", grantType)

	c.JSON(http.StatusOK, OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    oauthTokenTypeBearer,
		ExpiresIn:    expiresIn,
		RefreshToken: refreshToken,
	})
}

func (h *oauthHandler) oauthError(
	c *gin.Context,
	grantType, code, description string,
	status int,
) {
	if grantType == "" {
		grantType = oauthGrantTypeUnknown
	}

	observability.LogAction(
		c,
		"oauth.token.error",
		"grant_type", grantType,
		"error", code,
	)

	c.JSON(status, OAuthErrorResponse{
		Error:            code,
		ErrorDescription: description,
	})
}
