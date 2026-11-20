//nolint:funcorder // helpers stay near Exchange for readability
package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var (
	// ErrDisabled is returned when OIDC is not configured/enabled.
	ErrDisabled = errors.New("oidc disabled")
	// ErrInvalidState is returned when the OAuth state cookie does not match.
	ErrInvalidState = errors.New("oidc invalid state")
	// ErrMissingCode is returned when the callback lacks an authorization code.
	ErrMissingCode = errors.New("oidc missing authorization code")
	// ErrMissingIDToken is returned when the token response has no id_token.
	ErrMissingIDToken = errors.New("oidc missing id_token")
	// ErrNonceMismatch is returned when the ID token nonce does not match.
	ErrNonceMismatch = errors.New("oidc nonce mismatch")
)

const (
	pkceVerifierBytes = 32
	stateNonceBytes   = 24
)

// Provider is an OIDC relying-party client (discovery + code exchange + verify).
type Provider struct {
	cfg      Config
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
}

// NewProvider discovers the issuer and builds the RP client.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	err := cfg.Validate()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, ErrDisabled
	}

	issuer := strings.TrimRight(strings.TrimSpace(cfg.IssuerURL), "/")
	ctx = oidcHTTPContext(ctx, cfg.InsecureSkipVerify)
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discover %s: %w", issuer, err)
	}

	oauthCfg := oauth2.Config{
		ClientID:     strings.TrimSpace(cfg.ClientID),
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURI(),
		Scopes:       cfg.Scopes(),
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: oauthCfg.ClientID})

	return &Provider{
		cfg:      cfg,
		provider: provider,
		verifier: verifier,
		oauth:    oauthCfg,
	}, nil
}

// Config returns the loaded RP config.
func (p *Provider) Config() Config {
	if p == nil {
		return Config{}
	}

	return p.cfg
}

// PublicStatus returns FE-safe flags.
func (p *Provider) PublicStatus() PublicStatus {
	if p == nil {
		return PublicStatus{}
	}

	return p.cfg.Public()
}

// AuthCodeURL builds the authorize URL for the given state, nonce, and PKCE challenge.
func (p *Provider) AuthCodeURL(state, nonce, codeChallenge string) string {
	return p.oauth.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

// ExchangeResult is the verified identity after code exchange.
type ExchangeResult struct {
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
	Username      string
	Groups        []string
	RawIDToken    string
	Claims        map[string]any
}

// Exchange verifies state, exchanges the code with PKCE, and validates the ID token.
func (p *Provider) Exchange(
	ctx context.Context,
	code, expectedState, gotState, nonce, codeVerifier string,
) (ExchangeResult, error) {
	err := p.validateCallback(code, expectedState, gotState)
	if err != nil {
		return ExchangeResult{}, err
	}

	ctx = oidcHTTPContext(ctx, p.cfg.InsecureSkipVerify)
	token, err := p.oauth.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("oidc token exchange: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return ExchangeResult{}, ErrMissingIDToken
	}

	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("verify id_token: %w", err)
	}
	if nonce != "" && idToken.Nonce != nonce {
		return ExchangeResult{}, ErrNonceMismatch
	}

	result, err := p.exchangeResultFromIDToken(idToken, rawIDToken)
	if err != nil {
		return ExchangeResult{}, err
	}

	// Authelia 4.39+ (and many IdPs) keep email/profile/groups on UserInfo, not the ID token.
	err = p.mergeUserInfoClaims(ctx, token, &result)
	if err != nil {
		return ExchangeResult{}, err
	}

	return result, nil
}

func (p *Provider) validateCallback(code, expectedState, gotState string) error {
	if p == nil || !p.cfg.Enabled {
		return ErrDisabled
	}
	if strings.TrimSpace(code) == "" {
		return ErrMissingCode
	}
	if expectedState == "" || gotState == "" || !secureEqual(expectedState, gotState) {
		return ErrInvalidState
	}

	return nil
}

func (p *Provider) exchangeResultFromIDToken(
	idToken *oidc.IDToken,
	rawIDToken string,
) (ExchangeResult, error) {
	var claims map[string]any
	err := idToken.Claims(&claims)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("parse id_token claims: %w", err)
	}

	email := claimString(claims, "email")

	return ExchangeResult{
		Issuer:        idToken.Issuer,
		Subject:       idToken.Subject,
		Email:         strings.TrimSpace(strings.ToLower(email)),
		EmailVerified: claimBool(claims, "email_verified"),
		Username:      resolveUsername(claims, p.cfg.UsernameClaimName(), email, idToken.Subject),
		Groups:        claimStringSlice(claims, "groups"),
		RawIDToken:    rawIDToken,
		Claims:        claims,
	}, nil
}

func (p *Provider) mergeUserInfoClaims(
	ctx context.Context,
	token *oauth2.Token,
	result *ExchangeResult,
) error {
	userInfo, err := p.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
	if err != nil {
		// Some IdPs omit UserInfo; keep ID-token claims when already complete.
		if result.Email != "" {
			return nil
		}

		return fmt.Errorf("oidc userinfo: %w", err)
	}

	var claims map[string]any
	err = userInfo.Claims(&claims)
	if err != nil {
		return fmt.Errorf("parse userinfo claims: %w", err)
	}
	if result.Claims == nil {
		result.Claims = map[string]any{}
	}
	maps.Copy(result.Claims, claims)

	if email := claimString(claims, "email"); email != "" {
		result.Email = strings.TrimSpace(strings.ToLower(email))
	}
	if _, ok := claims["email_verified"]; ok {
		result.EmailVerified = claimBool(claims, "email_verified")
	}
	if groups := claimStringSlice(claims, "groups"); len(groups) > 0 {
		result.Groups = groups
	}
	result.Username = resolveUsername(
		result.Claims,
		p.cfg.UsernameClaimName(),
		result.Email,
		result.Subject,
	)

	return nil
}

func resolveUsername(claims map[string]any, usernameClaim, email, subject string) string {
	username := claimString(claims, usernameClaim)
	if username == "" {
		username = claimString(claims, "preferred_username")
	}
	if username == "" {
		username = email
	}
	if username == "" {
		username = subject
	}

	return username
}

// EndSessionURL builds an RP-initiated logout URL when the IdP advertises one.
func (p *Provider) EndSessionURL(idTokenHint, postLogoutRedirectURI string) (string, bool) {
	if p == nil {
		return "", false
	}

	//nolint:tagliatelle // OIDC discovery document uses snake_case keys.
	var discovery struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	err := p.provider.Claims(&discovery)
	if err != nil || discovery.EndSessionEndpoint == "" {
		return "", false
	}

	endpoint, err := url.Parse(discovery.EndSessionEndpoint)
	if err != nil {
		return "", false
	}
	query := endpoint.Query()
	if idTokenHint != "" {
		query.Set("id_token_hint", idTokenHint)
	}
	if postLogoutRedirectURI != "" {
		query.Set("post_logout_redirect_uri", postLogoutRedirectURI)
	}
	if p.cfg.ClientID != "" {
		query.Set("client_id", p.cfg.ClientID)
	}
	endpoint.RawQuery = query.Encode()

	return endpoint.String(), true
}

func oidcHTTPContext(ctx context.Context, insecureSkipVerify bool) context.Context {
	if !insecureSkipVerify {
		return ctx
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // local Authelia smoke / self-signed IdP only
		},
	}

	return oidc.ClientContext(ctx, &http.Client{Transport: transport})
}

// NewPKCEVerifier returns a high-entropy PKCE code_verifier.
func NewPKCEVerifier() (string, error) {
	buf := make([]byte, pkceVerifierBytes)
	_, err := rand.Read(buf)
	if err != nil {
		return "", fmt.Errorf("pkce verifier: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// PKCEChallengeS256 returns the S256 code_challenge for verifier.
func PKCEChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// NewState returns a random OAuth state/nonce value.
func NewState() (string, error) {
	buf := make([]byte, stateNonceBytes)
	_, err := rand.Read(buf)
	if err != nil {
		return "", fmt.Errorf("oidc state: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func claimString(claims map[string]any, key string) string {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func claimBool(claims map[string]any, key string) bool {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func claimStringSlice(claims map[string]any, key string) []string {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				out = append(out, s)
			}
		}

		return out
	case string:
		s := strings.TrimSpace(typed)
		if s == "" {
			return nil
		}

		return []string{s}
	default:
		return nil
	}
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}

	return diff == 0
}
