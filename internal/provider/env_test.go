package provider

import (
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type stubEnvRequirer struct {
	keys []string
}

func (s stubEnvRequirer) RequiredEnv() []string { return s.keys }

func TestEnvHelpers(t *testing.T) {
	keyPresent := "SUDOSTREAM_TEST_ENV_PRESENT"
	keyMissing := "SUDOSTREAM_TEST_ENV_MISSING"

	for _, tc := range []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "Env trims values",
			run: func(t *testing.T) {
				t.Helper()
				t.Setenv(keyPresent, " value ")
				if got := Env(keyPresent); got != "value" {
					t.Fatalf("Env trim: got %q", got)
				}
			},
		},
		{
			name: "RequireEnv ok when present",
			run: func(t *testing.T) {
				t.Helper()
				t.Setenv(keyPresent, "value")
				t.Setenv(keyMissing, "")
				if err := RequireEnv(keyPresent); err != nil {
					t.Fatalf("RequireEnv present: %v", err)
				}
			},
		},
		{
			name: "RequireEnv reports missing",
			run: func(t *testing.T) {
				t.Helper()
				t.Setenv(keyPresent, "value")
				t.Setenv(keyMissing, "")
				err := RequireEnv(keyPresent, keyMissing)
				if !errors.Is(err, ErrMissingEnv) {
					t.Fatalf("want ErrMissingEnv, got %v", err)
				}
			},
		},
		{
			name: "MissingEnvFor lists unset keys",
			run: func(t *testing.T) {
				t.Helper()
				t.Setenv(keyPresent, "value")
				t.Setenv(keyMissing, "")
				adapter := stubEnvRequirer{keys: []string{keyPresent, keyMissing}}
				if got := RequiredEnvFor(adapter); len(got) != 2 {
					t.Fatalf("RequiredEnvFor: %v", got)
				}
				missing := MissingEnvFor(adapter)
				if len(missing) != 1 || missing[0] != keyMissing {
					t.Fatalf("MissingEnvFor: %v", missing)
				}
				if RequiredEnvFor("not-requirer") != nil {
					t.Fatal("non-requirer should yield nil")
				}
			},
		},
	} {
		allure.Test(t, "env helpers — "+tc.name, func(a *allure.Context) {
			tc.run(a.T())
		})
	}
}
