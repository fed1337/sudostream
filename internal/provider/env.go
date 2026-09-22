package provider

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrMissingEnv is returned when one or more required SUDOSTREAM_* vars are unset.
var ErrMissingEnv = errors.New("missing required env")

// Env looks up a SUDOSTREAM_* (or arbitrary) environment variable, trimmed.
func Env(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// RequireEnv returns an error listing every missing key.
func RequireEnv(keys ...string) error {
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if Env(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrMissingEnv, strings.Join(missing, ", "))
}

// EnvRequirer is implemented by adapters that need credentials from the environment (FI-1 L13).
type EnvRequirer interface {
	RequiredEnv() []string
}

// RequiredEnvFor returns RequiredEnv() when adapter implements EnvRequirer.
func RequiredEnvFor(adapter any) []string {
	requirer, ok := adapter.(EnvRequirer)
	if !ok || requirer == nil {
		return nil
	}

	return requirer.RequiredEnv()
}

// MissingEnvFor returns RequiredEnv keys that are unset for adapter.
func MissingEnvFor(adapter any) []string {
	keys := RequiredEnvFor(adapter)
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if Env(key) == "" {
			missing = append(missing, key)
		}
	}

	return missing
}
