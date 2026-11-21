package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/auth/oidc"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	oidcStateCookie          = "oidc_state"
	oidcNonceCookie          = "oidc_nonce"
	oidcVerifierCookie       = "oidc_verifier"
	oidcCookieTTL            = 10 * time.Minute
	oidcFrontendCallbackPath = "/login/oidc/callback"
	oidcFrontendLoginPath    = "/login"
	oidcStartFailedMessage   = "oidc start failed"
)

// OIDCStatus returns non-secret SSO flags for the login UI.
//
//	@Summary		OIDC status
//	@Description	Returns whether external OIDC SSO is enabled and related UI flags.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	oidc.PublicStatus
//	@Router			/api/auth/oidc/status [get]
func (h *authHandler) oidcStatus(c *gin.Context) {
	if h.config.OIDC == nil {
		c.JSON(http.StatusOK, oidc.PublicStatus{})

		return
	}
	c.JSON(http.StatusOK, h.config.OIDC.PublicStatus())
}

// OIDCStart begins the authorization-code + PKCE flow.
//
//	@Summary		Start OIDC login
//	@Description	Redirects the browser to the configured OpenID Provider authorize URL.
//	@Tags			auth
//	@Success		302
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/api/auth/oidc/start [get]
func (h *authHandler) oidcStart(c *gin.Context) {
	provider := h.config.OIDC
	if provider == nil || !provider.Config().Enabled {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "oidc not enabled"})

		return
	}

	state, err := oidc.NewState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: oidcStartFailedMessage})

		return
	}
	nonce, err := oidc.NewState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: oidcStartFailedMessage})

		return
	}
	verifier, err := oidc.NewPKCEVerifier()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: oidcStartFailedMessage})

		return
	}

	secure := h.config.CookieSecure
	maxAge := int(oidcCookieTTL.Seconds())
	c.SetCookie(oidcStateCookie, state, maxAge, "/", "", secure, true)
	c.SetCookie(oidcNonceCookie, nonce, maxAge, "/", "", secure, true)
	c.SetCookie(oidcVerifierCookie, verifier, maxAge, "/", "", secure, true)

	challenge := oidc.PKCEChallengeS256(verifier)
	c.Redirect(http.StatusFound, provider.AuthCodeURL(state, nonce, challenge))
}

// OIDCCallback completes the OIDC code exchange and issues a sudoStream session.
//
//	@Summary		OIDC callback
//	@Description	Exchanges the authorization code, maps/provisions the user, sets refresh cookie, redirects to SPA.
//	@Tags			auth
//	@Param			code	query	string	false	"Authorization code"
//	@Param			state	query	string	false	"OAuth state"
//	@Param			error	query	string	false	"IdP error"
//	@Success		302
//	@Failure		302
//	@Router			/api/auth/oidc/callback [get]
func (h *authHandler) oidcCallback(c *gin.Context) {
	provider := h.config.OIDC
	baseURL := h.oidcAppBaseURL()
	if provider == nil || !provider.Config().Enabled || h.auth == nil {
		c.Redirect(http.StatusFound, baseURL+oidcFrontendLoginPath+"?oidc_error="+url.QueryEscape("oidc not enabled"))

		return
	}

	if idpErr := strings.TrimSpace(c.Query("error")); idpErr != "" {
		desc := strings.TrimSpace(c.Query("error_description"))
		msg := idpErr
		if desc != "" {
			msg = desc
		}
		h.clearOIDCCookies(c)
		c.Redirect(http.StatusFound, baseURL+oidcFrontendLoginPath+"?oidc_error="+url.QueryEscape(msg))

		return
	}

	expectedState, _ := c.Cookie(oidcStateCookie)
	nonce, _ := c.Cookie(oidcNonceCookie)
	verifier, _ := c.Cookie(oidcVerifierCookie)
	gotState := c.Query("state")
	code := c.Query("code")

	result, err := provider.Exchange(c.Request.Context(), code, expectedState, gotState, nonce, verifier)
	h.clearOIDCCookies(c)
	if err != nil {
		c.Redirect(http.StatusFound, baseURL+oidcFrontendLoginPath+"?oidc_error="+url.QueryEscape(mapOIDCError(err)))

		return
	}

	cfg := provider.Config()
	outcome, err := h.auth.CompleteOIDCLogin(c.Request.Context(), auth.OIDCLoginInput{
		Issuer:                   result.Issuer,
		Subject:                  result.Subject,
		Email:                    result.Email,
		EmailVerified:            result.EmailVerified,
		Username:                 result.Username,
		Groups:                   result.Groups,
		RawIDToken:               result.RawIDToken,
		AdminGroup:               cfg.AdminGroup,
		AutoRegister:             cfg.AutoRegister,
		MergeUsers:               cfg.MergeUsers,
		RequireEmailVerification: cfg.RequireEmailVerification,
	}, sessionMeta(c))
	if err != nil {
		c.Redirect(http.StatusFound, baseURL+oidcFrontendLoginPath+"?oidc_error="+url.QueryEscape(mapOIDCError(err)))

		return
	}

	setRefreshCookie(c, outcome.RefreshToken, h.config.CookieSecure, h.auth.RefreshTTL())
	// Access token is picked up by the SPA via /api/auth/refresh on the callback page.
	_ = outcome.AccessToken
	c.Redirect(http.StatusFound, baseURL+oidcFrontendCallbackPath)
}

func (h *authHandler) clearOIDCCookies(c *gin.Context) {
	secure := h.config.CookieSecure
	c.SetCookie(oidcStateCookie, "", -1, "/", "", secure, true)
	c.SetCookie(oidcNonceCookie, "", -1, "/", "", secure, true)
	c.SetCookie(oidcVerifierCookie, "", -1, "/", "", secure, true)
}

func (h *authHandler) oidcAppBaseURL() string {
	if h.auth != nil {
		return strings.TrimRight(h.auth.BaseURL(), "/")
	}
	if h.config.OIDC != nil {
		return strings.TrimRight(h.config.OIDC.Config().BaseURL, "/")
	}

	return ""
}

func mapOIDCError(err error) string {
	switch {
	case errors.Is(err, oidc.ErrInvalidState), errors.Is(err, oidc.ErrMissingCode):
		return "oidc login failed"
	case errors.Is(err, auth.ErrOIDCUnknownIdentity):
		return "oidc account not linked"
	case errors.Is(err, auth.ErrOIDCEmailNotVerified):
		return "oidc email not verified"
	case errors.Is(err, auth.ErrOIDCEmailRequired):
		return "oidc email required"
	case errors.Is(err, auth.ErrUserExists):
		return "oidc email already registered"
	case errors.Is(err, auth.ErrUserDisabled):
		return "account disabled"
	case errors.Is(err, auth.ErrTVWebAuth):
		return "tv accounts cannot use web login"
	default:
		return "oidc login failed"
	}
}
