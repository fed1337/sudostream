package transcode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestPurgeStaleCache_DeletesUnusedNotInUse(t *testing.T) {
	t.Parallel()

	allure.Test(t, "PurgeStaleCache removes unused packages but keeps in-use and fresh ones", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		svc, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		unusedDir := filepath.Join(root, "unused")
		freshDir := filepath.Join(root, "fresh")
		busyDir := filepath.Join(root, "busy")
		for _, dir := range []string{unusedDir, freshDir, busyDir} {
			err = os.MkdirAll(dir, 0o750)
			if err != nil {
				t.Fatal(err)
			}
			err = os.WriteFile(filepath.Join(dir, "a.bin"), []byte("hello"), 0o600)
			if err != nil {
				t.Fatal(err)
			}
		}

		oldTime := time.Now().Add(-time.Hour)
		err = os.Chtimes(unusedDir, oldTime, oldTime)
		if err != nil {
			t.Fatal(err)
		}
		err = os.Chtimes(busyDir, oldTime, oldTime)
		if err != nil {
			t.Fatal(err)
		}

		svc.mu.Lock()
		svc.lastUsed["unused"] = oldTime
		svc.lastUsed["busy"] = oldTime
		svc.lastUsed["fresh"] = time.Now()
		svc.packageBytes["unused"] = 5
		svc.packageBytes["busy"] = 5
		svc.packageBytes["fresh"] = 5
		svc.cacheBytes = 15
		svc.jobs["busy"] = &Job{cacheKey: "busy"}
		svc.mu.Unlock()

		if svc.CacheDirBytes() != 15 {
			t.Fatalf("tracked bytes want 15 got %d", svc.CacheDirBytes())
		}

		deleted, bytesRemoved, err := svc.PurgeStaleCache(15 * time.Minute)
		if err != nil {
			t.Fatalf("purge: %v", err)
		}
		if deleted != 1 {
			t.Fatalf("deleted=%d want 1", deleted)
		}
		if bytesRemoved != 5 {
			t.Fatalf("bytesRemoved=%d want 5", bytesRemoved)
		}
		if _, statErr := os.Stat(unusedDir); !os.IsNotExist(statErr) {
			t.Fatal("unused dir should be gone")
		}
		if _, statErr := os.Stat(freshDir); statErr != nil {
			t.Fatal("fresh dir should remain")
		}
		if _, statErr := os.Stat(busyDir); statErr != nil {
			t.Fatal("busy (in jobs) dir should remain")
		}
	})
}

func TestCacheDirStats_UsesTrackedBytes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "CacheDirStats reports tracked package bytes without requiring a full-tree walk result", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		svc, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		pkg := filepath.Join(root, "pkg")
		err = os.MkdirAll(filepath.Join(pkg, "nested"), 0o750)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(pkg, "nested", "seg.ts"), []byte("abcdef"), 0o600)
		if err != nil {
			t.Fatal(err)
		}

		svc.recordPackageSize("pkg")
		entries, bytes := svc.CacheDirStats()
		if entries != 1 {
			t.Fatalf("entries=%d want 1", entries)
		}
		if bytes != 6 {
			t.Fatalf("bytes=%d want 6", bytes)
		}

		svc.RemoveCache("pkg")
		entries, bytes = svc.CacheDirStats()
		if entries != 0 || bytes != 0 {
			t.Fatalf("after remove entries=%d bytes=%d", entries, bytes)
		}
	})
}
