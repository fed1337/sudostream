package oidc

import (
	"os"
	"strconv"
	"strings"
)

// LoadConfigFromEnv reads SUDOSTREAM_OIDC_* (+ BaseURL) using the process environment.
func LoadConfigFromEnv(baseURL string) Config {
	return Config{
		Enabled:                  envBool("SUDOSTREAM_OIDC_ENABLED"),
		IssuerURL:                strings.TrimSpace(os.Getenv("SUDOSTREAM_OIDC_ISSUER_URL")),
		IssuerName:               strings.TrimSpace(os.Getenv("SUDOSTREAM_OIDC_ISSUER_NAME")),
		ClientID:                 strings.TrimSpace(os.Getenv("SUDOSTREAM_OIDC_CLIENT_ID")),
		ClientSecret:             os.Getenv("SUDOSTREAM_OIDC_CLIENT_SECRET"),
		Only:                     envBool("SUDOSTREAM_OIDC_ONLY"),
		AutoRedirect:             envBool("SUDOSTREAM_OIDC_AUTO_REDIRECT"),
		AdminGroup:               strings.TrimSpace(os.Getenv("SUDOSTREAM_OIDC_ADMIN_GROUP")),
		UsernameClaim:            strings.TrimSpace(os.Getenv("SUDOSTREAM_OIDC_USERNAME_CLAIM")),
		Scope:                    strings.TrimSpace(os.Getenv("SUDOSTREAM_OIDC_SCOPE")),
		AutoRegister:             envBool("SUDOSTREAM_OIDC_AUTO_REGISTER"),
		MergeUsers:               envBool("SUDOSTREAM_OIDC_MERGE_USERS"),
		RequireEmailVerification: envBool("SUDOSTREAM_OIDC_REQUIRE_EMAIL_VERIFICATION"),
		InsecureSkipVerify:       envBool("SUDOSTREAM_OIDC_INSECURE_SKIP_VERIFY"),
		BaseURL:                  strings.TrimSpace(baseURL),
	}
}

func envBool(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		switch strings.ToLower(value) {
		case "1", "yes", "on":
			return true
		default:
			return false
		}
	}

	return parsed
}
