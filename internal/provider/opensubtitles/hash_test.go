package opensubtitles_test

import (
	"os"
	"path/filepath"
	"sudoStream/internal/provider/opensubtitles"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestMovieHash_Deterministic(t *testing.T) {
	t.Parallel()

	allure.Test(t, "moviehash is stable for a fixed file", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		path := filepath.Join(root, "sample.bin")

		// 128KiB of patterned bytes so head and tail differ.
		data := make([]byte, 128*1024)
		for i := range data {
			data[i] = byte(i % 251)
		}
		err := os.WriteFile(path, data, 0o600)
		if err != nil {
			t.Fatal(err)
		}

		hash1, size1, err := opensubtitles.MovieHash(path)
		if err != nil {
			t.Fatal(err)
		}
		hash2, size2, err := opensubtitles.MovieHash(path)
		if err != nil {
			t.Fatal(err)
		}
		if size1 != int64(len(data)) || size2 != size1 {
			t.Fatalf("size: %d %d want %d", size1, size2, len(data))
		}
		if hash1 == "" || hash1 != hash2 {
			t.Fatalf("hash unstable: %q %q", hash1, hash2)
		}
		if len(hash1) != 16 {
			t.Fatalf("want 16 hex chars, got %q", hash1)
		}
	})
}
