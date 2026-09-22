package transcode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestPurgeStaleCache_DeletesOldDirs(t *testing.T) {
	t.Parallel()

	allure.Test(t, "PurgeStaleCache removes only old cache directories", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		svc := &Service{cacheDir: root}

		oldDir := filepath.Join(root, "oldjob")
		newDir := filepath.Join(root, "newjob")
		err := os.MkdirAll(oldDir, 0o750)
		if err != nil {
			t.Fatal(err)
		}
		err = os.MkdirAll(newDir, 0o750)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(oldDir, "a.bin"), []byte("hello"), 0o600)
		if err != nil {
			t.Fatal(err)
		}
		oldTime := time.Now().Add(-48 * time.Hour)
		err = os.Chtimes(oldDir, oldTime, oldTime)
		if err != nil {
			t.Fatal(err)
		}

		if svc.CacheDirBytes() < 5 {
			t.Fatalf("expected cache bytes from file, got %d", svc.CacheDirBytes())
		}

		deleted, _, err := svc.PurgeStaleCache(24 * time.Hour)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if deleted != 1 {
			t.Fatalf("deleted=%d want 1", deleted)
		}
		_, oldStatErr := os.Stat(oldDir)
		if !os.IsNotExist(oldStatErr) {
			t.Fatal("old dir should be gone")
		}
		_, newStatErr := os.Stat(newDir)
		if newStatErr != nil {
			t.Fatal("new dir should remain")
		}
	})
}
