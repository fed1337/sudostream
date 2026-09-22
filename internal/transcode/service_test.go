package transcode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const testCacheKey = "abc123"

func TestStatus_ProcessingWhileJobActive(t *testing.T) {
	t.Parallel()

	allure.Test(t, "active jobs report processing even if master exists", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600)
		if err != nil {
			t.Fatalf("write master: %v", err)
		}

		service.jobs[cacheKey] = &Job{cacheKey: cacheKey, outDir: outDir}

		status := service.Status(cacheKey)
		if status.Status != StatusProcessing {
			t.Fatalf("expected processing without segments, got %q", status.Status)
		}
	})
}

func TestStatus_ReadyOnceTimelinePublished(t *testing.T) {
	t.Parallel()

	allure.Test(t, "complete marker reports ready regardless of segments", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		markTranscodeComplete(outDir)

		status := service.Status(cacheKey)
		if status.Status != StatusReady {
			t.Fatalf("expected ready, got %q", status.Status)
		}
	})
}

func TestStatus_IdleWithoutCache(t *testing.T) {
	t.Parallel()

	allure.Test(t, "missing cache dir reports idle", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		status := service.Status("missing")
		if status.Status != StatusIdle {
			t.Fatalf("expected idle, got %q", status.Status)
		}
	})
}

func TestStatus_ReadyRequiresCompleteMarker(t *testing.T) {
	t.Parallel()

	allure.Test(t, "master playlist alone is not enough for ready", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600)
		if err != nil {
			t.Fatalf("write master: %v", err)
		}

		status := service.Status(cacheKey)
		if status.Status != StatusProcessing {
			t.Fatalf("expected processing without complete marker, got %q", status.Status)
		}

		markTranscodeComplete(outDir)
		status = service.Status(cacheKey)
		if status.Status != StatusReady {
			t.Fatalf("expected ready, got %q", status.Status)
		}
	})
}

func TestStatus_ErrorMarker(t *testing.T) {
	t.Parallel()

	allure.Test(t, "error marker surfaces failed transcodes", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeTranscodeError(outDir, "ffmpeg failed")

		status := service.Status(cacheKey)
		if status.Status != StatusError {
			t.Fatalf("expected error, got %q", status.Status)
		}
		if status.Error != "ffmpeg failed" {
			t.Fatalf("unexpected error message: %q", status.Error)
		}
	})
}

func TestStartJob_CacheHitDoesNotSpawnWorker(t *testing.T) {
	t.Parallel()

	allure.Test(t, "complete cache skips job creation", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600)
		if err != nil {
			t.Fatalf("write master: %v", err)
		}
		markTranscodeComplete(outDir)

		err = service.StartJob(context.Background(), cacheKey, "/media/a.mp4")
		if err != nil {
			t.Fatalf("start job: %v", err)
		}

		service.mu.Lock()
		jobCount := len(service.jobs)
		service.mu.Unlock()

		if jobCount != 0 {
			t.Fatalf("expected no active jobs on cache hit, got %d", jobCount)
		}
	})
}

func TestStartJob_DedupWhileJobActive(t *testing.T) {
	t.Parallel()

	allure.Test(t, "second StartJob is ignored while job is active", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		service.jobs[cacheKey] = &Job{cacheKey: cacheKey, outDir: outDir}

		err = service.StartJob(context.Background(), cacheKey, "/media/a.mp4")
		if err != nil {
			t.Fatalf("start job: %v", err)
		}

		service.mu.Lock()
		jobCount := len(service.jobs)
		service.mu.Unlock()

		if jobCount != 1 {
			t.Fatalf("expected one active job, got %d", jobCount)
		}
	})
}

func TestRemoveCache_DeletesOutputDirectory(t *testing.T) {
	t.Parallel()

	allure.Test(t, "RemoveCache deletes cached HLS package", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600)
		if err != nil {
			t.Fatalf("write master: %v", err)
		}

		service.RemoveCache(cacheKey)

		_, statErr := os.Stat(outDir)
		if !os.IsNotExist(statErr) {
			t.Fatal("expected cache directory removed")
		}
	})
}

func TestResourcePath_RejectsTraversal(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ResourcePath rejects parent segments", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		if service.ResourcePath("abc", "../master.m3u8") != "" {
			t.Fatal("expected empty path for traversal")
		}
		if service.ResourcePath("abc", "v720/playlist.m3u8") == "" {
			t.Fatal("expected resource path for valid resource")
		}
	})
}

func TestFinishJob_WritesErrorMarkerOnFailure(t *testing.T) {
	t.Parallel()

	allure.Test(t, "failed jobs persist error marker", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		job := &Job{cacheKey: cacheKey, outDir: outDir, Failed: true, ErrorMsg: "boom"}
		service.jobs[cacheKey] = job
		service.finishJob(job)

		status := service.Status(cacheKey)
		if status.Status != StatusError || status.Error != "boom" {
			t.Fatalf("unexpected status after finish: %+v", status)
		}
	})
}

func TestCacheKey_IncludesVersionSuffix(t *testing.T) {
	t.Parallel()

	allure.Test(t, "cache key changes with size and tonemap settings", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		key := service.CacheKey("/media/a.mp4", 1, 2)
		if key == "" {
			t.Fatal("expected non-empty cache key")
		}
		if key == service.CacheKey("/media/a.mp4", 1, 3) {
			t.Fatal("expected different key for different size")
		}

		service.SetSettingsProvider(func() TranscodeSettings {
			settings := DefaultTranscodeSettings()
			settings.ToneMappingEnabled = false

			return settings
		})
		if key == service.CacheKey("/media/a.mp4", 1, 2) {
			t.Fatal("expected different key when tonemap disabled")
		}

		service.SetSettingsProvider(func() TranscodeSettings {
			settings := DefaultTranscodeSettings()
			settings.ToneMappingAlgorithm = ToneMappingHable

			return settings
		})
		if key == service.CacheKey("/media/a.mp4", 1, 2) {
			t.Fatal("expected different key when tonemap algo changes")
		}
	})
}

func TestStartJob_ErrorMarkerSkipsRestart(t *testing.T) {
	t.Parallel()

	allure.Test(t, "existing error marker prevents new transcode job", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		outDir := filepath.Join(root, cacheKey)
		err = os.MkdirAll(outDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeTranscodeError(outDir, "previous failure")

		err = service.StartJob(context.Background(), cacheKey, "/media/a.mp4")
		if err != nil {
			t.Fatalf("start job: %v", err)
		}

		service.mu.Lock()
		jobCount := len(service.jobs)
		service.mu.Unlock()

		if jobCount != 0 {
			t.Fatalf("expected no job after error marker, got %d", jobCount)
		}
	})
}

func TestHoldProcessingJob_ReportsProcessingStatus(t *testing.T) {
	t.Parallel()

	allure.Test(t, "held job reports processing without starting ffmpeg", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}

		cacheKey := testCacheKey
		service.HoldProcessingJob(cacheKey)

		status := service.Status(cacheKey)
		if status.Status != StatusProcessing {
			t.Fatalf("status: got %q want processing", status.Status)
		}

		err = service.StartJob(context.Background(), cacheKey, "/media/a.mp4")
		if err != nil {
			t.Fatalf("start job: %v", err)
		}

		service.mu.Lock()
		jobCount := len(service.jobs)
		service.mu.Unlock()
		if jobCount != 1 {
			t.Fatalf("expected one held job, got %d", jobCount)
		}
	})
}

func TestStartJob_IncompleteCacheRestartsPublish(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"cache without a complete marker republishes the timeline",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			service, err := NewService(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			cacheKey := testCacheKey
			outDir := filepath.Join(root, cacheKey)
			err = os.MkdirAll(outDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(filepath.Join(outDir, "master.m3u8"), []byte("#EXTM3U\n"), 0o600)
			if err != nil {
				t.Fatalf("write master: %v", err)
			}

			err = service.StartJob(context.Background(), cacheKey, "/media/a.mp4")
			if err != nil {
				t.Fatalf("start job: %v", err)
			}

			service.mu.Lock()
			jobCount := len(service.jobs)
			service.mu.Unlock()
			if jobCount != 1 {
				t.Fatalf("expected one active job, got %d", jobCount)
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = service.Shutdown(shutdownCtx)
			if err != nil {
				t.Fatalf("shutdown: %v", err)
			}
		},
	)
}
