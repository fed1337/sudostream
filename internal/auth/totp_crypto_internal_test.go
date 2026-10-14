package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"github.com/pquerna/otp/totp"
)

const testTOTPSecret = "JBSWY3DPEHPK3PXP" //nolint:gosec // test-only fixed TOTP secret

func TestTOTPSecret_EncryptDecryptRoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "encrypt then decrypt returns the original secret", func(a *allure.Context) {
		t := a.T()
		key := []byte(strings.Repeat("k", totpKeySize))

		encoded, err := encryptTOTPSecret(key, testTOTPSecret)
		if err != nil {
			t.Fatalf("encrypt: %v", err)
		}
		if encoded == testTOTPSecret {
			t.Fatal("ciphertext should not equal plaintext")
		}

		plain, err := decryptTOTPSecret(key, encoded)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}
		if plain != testTOTPSecret {
			t.Fatalf("round trip mismatch: %q", plain)
		}
	})
}

func TestDecryptTOTPSecret_Errors(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"decrypt rejects bad base64, short ciphertext, and wrong key",
		func(a *allure.Context) {
			t := a.T()
			key := []byte(strings.Repeat("k", totpKeySize))
			wrongKey := []byte(strings.Repeat("z", totpKeySize))

			_, err := decryptTOTPSecret(key, "not*base64*")
			if err == nil {
				t.Fatal("expected base64 decode error")
			}

			short := base64.StdEncoding.EncodeToString([]byte("abc"))
			_, err = decryptTOTPSecret(key, short)
			if !errors.Is(err, errInvalidCiphertext) {
				t.Fatalf("expected errInvalidCiphertext, got %v", err)
			}

			encoded, err := encryptTOTPSecret(key, testTOTPSecret)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			_, err = decryptTOTPSecret(wrongKey, encoded)
			if err == nil {
				t.Fatal("expected GCM open error for wrong key")
			}
		},
	)
}

func TestValidateTOTPCode_LeewayClampAndRejection(t *testing.T) {
	t.Parallel()

	allure.Test(t, "leeway clamps to bounds and invalid codes fail", func(a *allure.Context) {
		t := a.T()
		service := &Service{}

		code, err := totp.GenerateCode(testTOTPSecret, time.Now())
		if err != nil {
			t.Fatalf("generate totp: %v", err)
		}

		if !service.validateTOTPCode(testTOTPSecret, code, -10) {
			t.Fatal("sub-minimum leeway should clamp to default and accept a valid code")
		}
		if !service.validateTOTPCode(testTOTPSecret, code, 100000) {
			t.Fatal("over-maximum leeway should clamp and accept a valid code")
		}
		if service.validateTOTPCode(testTOTPSecret, "abcdef", defaultTwoFactorLeeway) {
			t.Fatal("non-numeric code should be rejected")
		}
	})
}

func TestLooksLikeBackupCode(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"looksLikeBackupCode accepts only valid backup code shape",
		func(a *allure.Context) {
			t := a.T()
			cases := []struct {
				code string
				want bool
			}{
				{code: "000000", want: false},
				{code: "123456", want: false},
				{code: "ABCD2345", want: true},
				{code: "ABCDEFG0", want: false},
				{code: "ABCD", want: false},
				{code: " abcd2345 ", want: true},
			}

			for _, tc := range cases {
				got := looksLikeBackupCode(tc.code)
				if got != tc.want {
					t.Fatalf("looksLikeBackupCode(%q): got %v want %v", tc.code, got, tc.want)
				}
			}
		},
	)
}
