package thumbnail

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestGeneratePosterWebP_ExtractsFrameFromSyntheticVideo(t *testing.T) {
	t.Parallel()

	allure.Test(t, "generatePosterWebP produces WebP from lavfi video", func(a *allure.Context) {
		t := a.T()
		_, lookErr := exec.LookPath("ffmpeg")
		if lookErr != nil {
			t.Skip("ffmpeg not installed")
		}

		dir := t.TempDir()
		source := filepath.Join(dir, "clip.mp4")
		tempPath := filepath.Join(dir, "poster.webp")

		//nolint:gosec // test fixture under t.TempDir()
		cmd := exec.CommandContext(
			context.Background(),
			"ffmpeg",
			"-v", "error",
			"-f", "lavfi",
			"-i", "color=c=red:s=128x128:d=1",
			"-c:v", "libx264",
			"-y", source,
		)
		err := cmd.Run()
		if err != nil {
			t.Fatalf("create test video: %v", err)
		}

		err = generatePosterWebP(context.Background(), source, tempPath)
		if err != nil {
			t.Fatalf("generatePosterWebP: %v", err)
		}

		info, err := os.Stat(tempPath)
		if err != nil || info.Size() == 0 {
			t.Fatal("expected non-empty poster output")
		}
	})
}

func TestExtractEmbeddedArtWebP_Integration(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"extractEmbeddedArtWebP extracts attached picture stream",
		func(a *allure.Context) {
			t := a.T()
			_, lookErr := exec.LookPath("ffmpeg")
			if lookErr != nil {
				t.Skip("ffmpeg not installed")
			}

			dir := t.TempDir()
			source := filepath.Join(dir, "cover.mp4")
			tempPath := filepath.Join(dir, "poster.webp")

			//nolint:gosec // test fixture under t.TempDir()
			cmd := exec.CommandContext(
				context.Background(),
				"ffmpeg",
				"-v", "error",
				"-f", "lavfi",
				"-i", "color=c=blue:s=64x64:d=1",
				"-f", "lavfi",
				"-i", "color=c=green:s=32x32:d=0.04",
				"-map", "0:v:0",
				"-map", "1:v:0",
				"-c:v:0", "libx264",
				"-c:v:1", "mjpeg",
				"-disposition:v:1", "attached_pic",
				"-y", source,
			)
			err := cmd.Run()
			if err != nil {
				t.Fatalf("create video with cover: %v", err)
			}

			streamIndex, hasArt := embeddedCoverStream(context.Background(), source)
			if !hasArt {
				t.Skip("ffprobe did not detect attached_pic in fixture")
			}

			err = extractEmbeddedArtWebP(context.Background(), source, tempPath, streamIndex)
			if err != nil {
				t.Fatalf("extractEmbeddedArtWebP: %v", err)
			}

			info, err := os.Stat(tempPath)
			if err != nil || info.Size() == 0 {
				t.Fatal("expected embedded art poster output")
			}
		},
	)
}
