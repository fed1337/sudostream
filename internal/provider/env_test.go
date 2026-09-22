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
	allure.Test(t, "RequireEnv and MissingEnvFor report unset SUDOSTREAM keys", func(a *allure.Context) {
		t := a.T()
		keyPresent := "SUDOSTREAM_TEST_ENV_PRESENT"
		keyMissing := "SUDOSTREAM_TEST_ENV_MISSING"
		t.Setenv(keyPresent, " value ")
		t.Setenv(keyMissing, "")

		if got := Env(keyPresent); got != "value" {
			t.Fatalf("Env trim: got %q", got)
		}
		err := RequireEnv(keyPresent)
		if err != nil {
			t.Fatalf("RequireEnv present: %v", err)
		}
		err = RequireEnv(keyPresent, keyMissing)
		if !errors.Is(err, ErrMissingEnv) {
			t.Fatalf("want ErrMissingEnv, got %v", err)
		}

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
	})
}
