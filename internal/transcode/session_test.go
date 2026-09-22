package transcode

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func writeSegmentFiles(t *testing.T, variantDir string, indexes ...int) {
	t.Helper()

	err := os.MkdirAll(variantDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir variant: %v", err)
	}

	for _, index := range indexes {
		err = os.WriteFile(filepath.Join(variantDir, SegmentName(index)), []byte("ts"), 0o600)
		if err != nil {
			t.Fatalf("write segment %d: %v", index, err)
		}
	}
}

func TestSegmentComplete_RequiresASuccessorWhileEncoding(t *testing.T) {
	t.Parallel()

	allure.Test(t, "the segment ffmpeg is still writing is not complete", func(a *allure.Context) {
		t := a.T()
		variantDir := filepath.Join(t.TempDir(), "v720")
		writeSegmentFiles(t, variantDir, 0, 1)

		live := &segmentSession{done: make(chan struct{})}

		if !segmentComplete(variantDir, 0, live) {
			t.Fatal("segment 0 is closed once segment 1 exists")
		}
		if segmentComplete(variantDir, 1, live) {
			t.Fatal("the newest segment is still open while the session runs")
		}

		close(live.done)

		if !segmentComplete(variantDir, 1, live) {
			t.Fatal("a finished session closes its last segment")
		}
		if segmentComplete(variantDir, 5, nil) {
			t.Fatal("missing segments are never complete")
		}
	})
}

func TestNeedsRestart_SeeksInsteadOfWaitingOnLargeGaps(t *testing.T) {
	t.Parallel()

	allure.Test(t, "restart only when the request is behind or far ahead", func(a *allure.Context) {
		t := a.T()
		variantDir := filepath.Join(t.TempDir(), "v720")
		writeSegmentFiles(t, variantDir, 10, 11)

		live := &segmentSession{startIndex: 10, done: make(chan struct{})}

		if !needsRestart(nil, variantDir, 12) {
			t.Fatal("no session means a fresh encoder is required")
		}
		if !needsRestart(live, variantDir, 4) {
			t.Fatal("seeking backwards must restart")
		}
		if needsRestart(live, variantDir, 13) {
			t.Fatal("a request just ahead of the encoder should wait, not restart")
		}
		if !needsRestart(live, variantDir, 40) {
			t.Fatal("a far-ahead seek must restart")
		}

		close(live.done)

		if !needsRestart(live, variantDir, 12) {
			t.Fatal("a dead session must be replaced")
		}
	})
}

func TestRemoveSegmentsFrom_TruncatesTail(t *testing.T) {
	t.Parallel()

	allure.Test(t, "restarting drops segments the new run will rewrite", func(a *allure.Context) {
		t := a.T()
		variantDir := filepath.Join(t.TempDir(), "v720")
		writeSegmentFiles(t, variantDir, 0, 1, 2, 3)

		removeSegmentsFrom(variantDir, 2)

		highest, ok := highestSegmentIndex(variantDir)
		if !ok || highest != 1 {
			t.Fatalf("highest: got %d ok=%v", highest, ok)
		}
	})
}

var errSessionFFmpegFailed = errors.New("ffmpeg failed")

func TestSegmentSession_IdleFailureAndTailBuffer(t *testing.T) {
	t.Parallel()

	allure.Test(t, "idle/failure helpers and stderr tail buffer", func(a *allure.Context) {
		t := a.T()
		now := time.Now()
		session := &segmentSession{
			lastAccess: now.Add(-2 * time.Minute),
			done:       make(chan struct{}),
		}

		if session.idleFor(now) < time.Minute {
			t.Fatalf("idleFor too small: %s", session.idleFor(now))
		}
		if msg, failed := session.failure(); failed || msg != "" {
			t.Fatalf("unexpected failure: %q %v", msg, failed)
		}

		session.err = errSessionFFmpegFailed
		session.stderr = testEncoderBoomStderr
		if msg, failed := session.failure(); !failed || msg != testEncoderBoomStderr {
			t.Fatalf("failure: %q %v", msg, failed)
		}

		buf := &tailBuffer{limit: 8}
		_, err := buf.Write([]byte("abcdefghijklmnop"))
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := buf.String(); got != "ijklmnop" {
			t.Fatalf("tail: got %q", got)
		}
	})
}

func TestIdleSessions_SelectsTimedOutEncoders(t *testing.T) {
	t.Parallel()

	allure.Test(t, "idleSessions returns sessions past the idle timeout", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		service, err := NewService(root)
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

		now := time.Now()
		live := &segmentSession{
			variant:    VideoVariant(480),
			lastAccess: now,
			done:       make(chan struct{}),
		}
		stale := &segmentSession{
			variant:    VideoVariant(720),
			lastAccess: now.Add(-sessionIdleTimeout - time.Second),
			done:       make(chan struct{}),
		}

		service.mu.Lock()
		service.sessions["cache"] = map[string]*segmentSession{
			live.variant.Dir():  live,
			stale.variant.Dir(): stale,
		}
		service.mu.Unlock()

		idle := service.idleSessions(now)
		if len(idle) != 1 || idle[0] != stale {
			t.Fatalf("idle sessions: %+v", idle)
		}

		service.clearSession("cache", stale)
		if service.session("cache", stale.variant) != nil {
			t.Fatal("expected clearSession to remove the stale session")
		}
	})
}

func TestResolveSegment_RejectsUnknownRenditionsAndRanges(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"segment requests are validated against the published timeline",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()

			service, err := NewService(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

			cacheKey := "abc"
			outDir := filepath.Join(root, cacheKey)

			err = os.MkdirAll(outDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			_, err = service.ResolveSegment(
				context.Background(), cacheKey, "/media/a.mkv", VideoVariant(480), 0,
			)
			if !errors.Is(err, ErrSourceMetaUnavailable) {
				t.Fatalf("expected missing metadata error, got %v", err)
			}

			err = WriteSourceMeta(outDir, SourceMeta{Height: 480, Segments: []float64{6, 6}})
			if err != nil {
				t.Fatalf("write meta: %v", err)
			}

			_, err = service.ResolveSegment(
				context.Background(), cacheKey, "/media/a.mkv", VideoVariant(2160), 0,
			)
			if !errors.Is(err, ErrInvalidVariant) {
				t.Fatalf("expected invalid variant error, got %v", err)
			}

			_, err = service.ResolveSegment(
				context.Background(), cacheKey, "/media/a.mkv", VideoVariant(480), 9,
			)
			if !errors.Is(err, ErrSegmentOutOfRange) {
				t.Fatalf("expected out-of-range error, got %v", err)
			}
		},
	)
}

func TestResolveSegment_CacheHitWithoutEncoder(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"ResolveSegment returns an on-disk segment without starting ffmpeg",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			service, err := NewService(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

			cacheKey := "hit"
			outDir := filepath.Join(root, cacheKey)
			err = os.MkdirAll(outDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			err = WriteSourceMeta(outDir, SourceMeta{Height: 480, Segments: []float64{6, 6, 6}})
			if err != nil {
				t.Fatalf("write meta: %v", err)
			}

			variant := VideoVariant(480)
			variantDir := filepath.Join(outDir, variant.Dir())
			writeSegmentFiles(t, variantDir, 0, 1)

			path, err := service.ResolveSegment(
				context.Background(), cacheKey, "/media/a.mkv", variant, 0,
			)
			if err != nil {
				t.Fatalf("resolve hit: %v", err)
			}
			if path != filepath.Join(variantDir, SegmentName(0)) {
				t.Fatalf("path: got %q", path)
			}
		},
	)
}

func TestWaitForSegment_CancelFailureAndMissing(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"waitForSegment covers cancel and finished-session errors",
		func(a *allure.Context) {
			t := a.T()
			variantDir := filepath.Join(t.TempDir(), "v480")
			err := os.MkdirAll(variantDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			segmentPath := filepath.Join(variantDir, SegmentName(0))

			live := &segmentSession{done: make(chan struct{}), lastAccess: time.Now()}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err = waitForSegment(ctx, live, variantDir, 0, segmentPath)
			if err == nil || !errors.Is(err, context.Canceled) {
				t.Fatalf("expected canceled wait, got %v", err)
			}

			failed := &segmentSession{
				done:   make(chan struct{}),
				err:    errSessionFFmpegFailed,
				stderr: testEncoderBoomStderr,
			}
			close(failed.done)
			_, err = waitForSegment(context.Background(), failed, variantDir, 0, segmentPath)
			if !errors.Is(err, ErrSegmentUnavailable) {
				t.Fatalf("expected unavailable with detail, got %v", err)
			}

			cleanExit := &segmentSession{done: make(chan struct{})}
			close(cleanExit.done)
			_, err = waitForSegment(context.Background(), cleanExit, variantDir, 0, segmentPath)
			if !errors.Is(err, ErrSegmentUnavailable) {
				t.Fatalf("expected missing segment after clean exit, got %v", err)
			}

			// Segment appears while a live session is polled.
			producing := &segmentSession{done: make(chan struct{}), lastAccess: time.Now()}
			go func() {
				time.Sleep(80 * time.Millisecond)
				_ = os.WriteFile(segmentPath, []byte("ts"), 0o600)
				_ = os.WriteFile(filepath.Join(variantDir, SegmentName(1)), []byte("ts"), 0o600)
			}()
			got, err := waitForSegment(context.Background(), producing, variantDir, 0, segmentPath)
			if err != nil {
				t.Fatalf("wait for produced segment: %v", err)
			}
			if got != segmentPath {
				t.Fatalf("path: got %q", got)
			}
		},
	)
}

func TestStopSession_CancelsRunningEncoder(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"stopSession is a no-op for missing sessions and waits for cancel",
		func(a *allure.Context) {
			t := a.T()
			root := t.TempDir()
			service, err := NewService(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			t.Cleanup(func() { _ = service.Shutdown(context.Background()) })

			variant := VideoVariant(720)
			service.stopSession("missing", variant)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			session := &segmentSession{
				variant: variant,
				cancel:  cancel,
				done:    done,
			}
			go func() {
				<-ctx.Done()
				close(done)
			}()
			service.registerSession("cache", variant, session)
			service.stopSession("cache", variant)

			if !session.finished() {
				t.Fatal("expected stopSession to finish the session")
			}
		},
	)
}

func TestNeedsRestart_EmptyDirUsesStartGap(t *testing.T) {
	t.Parallel()

	allure.Test(t, "without on-disk segments restart uses startIndex gap", func(a *allure.Context) {
		t := a.T()
		variantDir := filepath.Join(t.TempDir(), "empty")
		err := os.MkdirAll(variantDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		live := &segmentSession{startIndex: 0, done: make(chan struct{})}
		if needsRestart(live, variantDir, 1) {
			t.Fatal("small forward gap should wait")
		}
		if !needsRestart(live, variantDir, segmentGapThreshold+2) {
			t.Fatal("large gap with no segments must restart")
		}
	})
}

func TestHighestSegmentIndex_IgnoresDirsAndJunk(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"highestSegmentIndex skips directories and non-segment files",
		func(a *allure.Context) {
			t := a.T()
			variantDir := filepath.Join(t.TempDir(), "v720")
			writeSegmentFiles(t, variantDir, 2)
			err := os.MkdirAll(filepath.Join(variantDir, "nested"), 0o750)
			if err != nil {
				t.Fatalf("mkdir nested: %v", err)
			}
			err = os.WriteFile(filepath.Join(variantDir, "notes.txt"), []byte("x"), 0o600)
			if err != nil {
				t.Fatalf("write junk: %v", err)
			}

			highest, ok := highestSegmentIndex(variantDir)
			if !ok || highest != 2 {
				t.Fatalf("highest: got %d ok=%v", highest, ok)
			}

			missing := filepath.Join(t.TempDir(), "nope")
			if _, ok = highestSegmentIndex(missing); ok {
				t.Fatal("missing dir should report no highest")
			}
		},
	)
}

func TestRemoveSegmentsFrom_IgnoresMissingAndDirs(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"removeSegmentsFrom tolerates missing dirs and nested folders",
		func(a *allure.Context) {
			t := a.T()
			removeSegmentsFrom(filepath.Join(t.TempDir(), "missing"), 0)

			variantDir := filepath.Join(t.TempDir(), "v720")
			writeSegmentFiles(t, variantDir, 0, 1)
			err := os.MkdirAll(filepath.Join(variantDir, "keep"), 0o750)
			if err != nil {
				t.Fatalf("mkdir keep: %v", err)
			}

			removeSegmentsFrom(variantDir, 1)
			_, err = os.Stat(filepath.Join(variantDir, SegmentName(0)))
			if err != nil {
				t.Fatalf("segment 0 should remain: %v", err)
			}
			_, err = os.Stat(filepath.Join(variantDir, "keep"))
			if err != nil {
				t.Fatalf("nested dir should remain: %v", err)
			}
		},
	)
}

func TestResolveSegment_ProducesAlignedSegmentsAcrossRungs(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"segment N covers the same time range on every rendition",
		func(a *allure.Context) {
			t := a.T()
			mediaPath := synthesizeFixture(t, 14)
			root := t.TempDir()

			service, err := NewService(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			cacheKey := "aligned"
			outDir := filepath.Join(root, cacheKey)

			err = os.MkdirAll(outDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			meta, err := ProbeAndPublish(context.Background(), outDir, mediaPath)
			if err != nil {
				t.Fatalf("probe and publish: %v", err)
			}

			if meta.SegmentCount() < 2 {
				t.Skipf("fixture produced only %d segments", meta.SegmentCount())
			}

			const index = 1
			wantStart := SegmentStart(meta.Segments, index)

			variants := make([]Variant, 0, len(meta.Qualities()))
			for _, quality := range meta.Qualities() {
				variants = append(variants, VideoVariant(quality.Height))
			}

			for _, variant := range variants {
				path, resolveErr := service.ResolveSegment(
					context.Background(), cacheKey, mediaPath, variant, index,
				)
				if resolveErr != nil {
					t.Fatalf("%s: resolve: %v", variant.Dir(), resolveErr)
				}

				gotStart := segmentStartPTS(t, path)
				if math.Abs(gotStart-wantStart) > 0.5 {
					t.Fatalf(
						"%s: segment %d starts at %.3f, timeline says %.3f",
						variant.Dir(), index, gotStart, wantStart,
					)
				}
			}
		},
	)
}

func TestResolveSegment_KeepsTheTimelineAcrossARestart(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"a forward seek restarts ffmpeg without rebasing timestamps",
		func(a *allure.Context) {
			t := a.T()
			mediaPath := synthesizeFixture(t, 40)
			root := t.TempDir()

			service, err := NewService(root)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			cacheKey := "restart"
			outDir := filepath.Join(root, cacheKey)

			err = os.MkdirAll(outDir, 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			meta, err := ProbeAndPublish(context.Background(), outDir, mediaPath)
			if err != nil {
				t.Fatalf("probe and publish: %v", err)
			}

			seekIndex := meta.SegmentCount() - 1
			if seekIndex <= segmentGapThreshold {
				t.Skipf("fixture produced only %d segments", meta.SegmentCount())
			}

			variant := VideoVariant(meta.Height)

			for _, index := range []int{0, seekIndex} {
				path, resolveErr := service.ResolveSegment(
					context.Background(), cacheKey, mediaPath, variant, index,
				)
				if resolveErr != nil {
					t.Fatalf("resolve segment %d: %v", index, resolveErr)
				}

				want := SegmentStart(meta.Segments, index)
				if got := segmentStartPTS(t, path); math.Abs(got-want) > 0.5 {
					t.Fatalf("segment %d starts at %.3f, timeline says %.3f", index, got, want)
				}
			}
		},
	)
}

// segmentStartPTS reads the first presentation timestamp of a produced segment.
func segmentStartPTS(t *testing.T, segmentPath string) float64 {
	t.Helper()

	cmd := exec.CommandContext( //nolint:gosec // segmentPath is under t.TempDir()
		context.Background(),
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time",
		"-of", "csv=p=0",
		"-read_intervals", "%+#1",
		segmentPath,
	)

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe segment: %v", err)
	}

	first := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(output)), "\n", 2)[0])

	seconds, err := strconv.ParseFloat(strings.TrimSuffix(first, ","), 64)
	if err != nil {
		t.Fatalf("parse pts %q: %v", first, err)
	}

	return seconds
}
