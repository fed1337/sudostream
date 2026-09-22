package auth_test

import (
	"encoding/base64"
	"strings"
	"sudoStream/internal/auth"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseTOTPEncryptionKey(t *testing.T) {
	t.Parallel()

	allure.Test(t, "valid base64 key is accepted", func(a *allure.Context) {
		t := a.T()
		raw := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
		key, err := auth.ParseTOTPEncryptionKey(raw)
		if err != nil {
			t.Fatalf("parse key: %v", err)
		}
		if len(key) != 32 {
			t.Fatalf("unexpected key length: %d", len(key))
		}
	})

	allure.Test(t, "empty key is rejected", func(a *allure.Context) {
		t := a.T()
		_, err := auth.ParseTOTPEncryptionKey("")
		if err == nil {
			t.Fatal("expected error for empty key")
		}
	})

	allure.Test(t, "wrong length key is rejected", func(a *allure.Context) {
		t := a.T()
		raw := base64.StdEncoding.EncodeToString([]byte("short"))
		_, err := auth.ParseTOTPEncryptionKey(raw)
		if err == nil {
			t.Fatal("expected error for wrong key length")
		}
	})
}
