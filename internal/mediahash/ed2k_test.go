package mediahash_test

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"sudoStream/internal/mediahash"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
	"golang.org/x/crypto/md4" //nolint:gosec,staticcheck // G506/SA1019: small-file ed2k vector uses MD4
)

func TestEd2k_BlueMethodZeros(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		size int
		want string
	}{
		{name: "one chunk", size: mediahash.Ed2kChunkSize, want: "d7def262a127cd79096a108e7a9fc138"},
		{name: "two chunks", size: mediahash.Ed2kChunkSize * 2, want: "194ee9e4fa79b2ee9f8829284c466051"},
	} {
		allure.Test(t, "ed2k blue method zeros — "+tc.name, func(a *allure.Context) {
			t := a.T()
			path := filepath.Join(t.TempDir(), "zeros.bin")
			err := os.WriteFile(path, make([]byte, tc.size), 0o600)
			if err != nil {
				t.Fatal(err)
			}
			hash, size, err := mediahash.Ed2k(path)
			if err != nil {
				t.Fatal(err)
			}
			if size != int64(tc.size) {
				t.Fatalf("size: got %d want %d", size, tc.size)
			}
			if hash != tc.want {
				t.Fatalf("hash: got %q want %q", hash, tc.want)
			}
		})
	}
}

func TestEd2k_SmallFileIsMD4(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ed2k of a small file equals MD4 of content", func(a *allure.Context) {
		t := a.T()
		path := filepath.Join(t.TempDir(), "small.bin")
		content := []byte("sudoStream ed2k vector")
		err := os.WriteFile(path, content, 0o600)
		if err != nil {
			t.Fatal(err)
		}

		hasher := md4.New() //nolint:gosec // G406: expected MD4 of content for small files
		_, _ = hasher.Write(content)
		want := hex.EncodeToString(hasher.Sum(nil))

		hash, size, err := mediahash.Ed2kContext(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		if size != int64(len(content)) {
			t.Fatalf("size: got %d want %d", size, len(content))
		}
		if hash != want {
			t.Fatalf("hash: got %q want %q", hash, want)
		}
	})
}

func TestEd2kContext_Canceled(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ed2k context cancellation returns ctx error", func(a *allure.Context) {
		t := a.T()
		path := filepath.Join(t.TempDir(), "file.bin")
		err := os.WriteFile(path, []byte("x"), 0o600)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err = mediahash.Ed2kContext(ctx, path)
		if err == nil {
			t.Fatal("expected context error")
		}
	})
}
