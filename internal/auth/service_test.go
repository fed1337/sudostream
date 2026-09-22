package auth_test

import (
	"sudoStream/internal/auth"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_BcryptCost(t *testing.T) {
	cases := []struct {
		name     string
		ginMode  string
		wantCost int
	}{
		{name: "default", ginMode: "", wantCost: 12},
		{name: "production release", ginMode: "release", wantCost: 12},
		{name: "debug mode", ginMode: "debug", wantCost: 12},
		{name: "test mode", ginMode: "test", wantCost: 4},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("GIN_MODE", testCase.ginMode)

			hash, err := auth.HashPassword("secret-password")
			if err != nil {
				t.Fatalf("hash password: %v", err)
			}

			err = bcrypt.CompareHashAndPassword([]byte(hash), []byte("secret-password"))
			if err != nil {
				t.Fatalf("compare hash: %v", err)
			}

			cost, err := bcrypt.Cost([]byte(hash))
			if err != nil {
				t.Fatalf("read bcrypt cost: %v", err)
			}
			if cost != testCase.wantCost {
				t.Fatalf("unexpected bcrypt cost: got %d want %d", cost, testCase.wantCost)
			}
		})
	}
}
