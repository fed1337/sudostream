package thumbnail

import (
	"os"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestFrameCaptureOutputOK(t *testing.T) {
	t.Parallel()

	allure.Test(t, "non-empty file is accepted", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		path := filepath.Join(root, "frame.webp")
		err := os.WriteFile(path, []byte("x"), 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		if !frameCaptureOutputOK(path) {
			t.Fatal("expected non-empty capture to be ok")
		}
	})

	allure.Test(t, "empty file is rejected", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		path := filepath.Join(root, "frame.webp")
		err := os.WriteFile(path, nil, 0o600)
		if err != nil {
			t.Fatalf("write file: %v", err)
		}

		if frameCaptureOutputOK(path) {
			t.Fatal("expected empty capture to be rejected")
		}
	})
}
