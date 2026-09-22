package transcode_test

import (
	"context"
	"os"
	"path/filepath"
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestLocalSubtitleLanguages_SidecarCodes(t *testing.T) {
	t.Parallel()

	allure.Test(t, "LocalSubtitleLanguages reads en/ru from series sidecars", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		video := filepath.Join(root, "Show S01E01 Title.mkv")
		err := os.WriteFile(video, []byte("fake"), 0o600)
		if err != nil {
			t.Fatalf("write video: %v", err)
		}
		err = os.WriteFile(filepath.Join(root, "Show S01E01 Title.en.srt"), []byte("1\n"), 0o600)
		if err != nil {
			t.Fatalf("write en: %v", err)
		}
		err = os.MkdirAll(filepath.Join(root, "Subs"), 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(root, "Subs", "S01E01.rus.srt"), []byte("1\n"), 0o600)
		if err != nil {
			t.Fatalf("write ru: %v", err)
		}

		langs, err := transcode.LocalSubtitleLanguages(context.Background(), video)
		if err != nil {
			// Probe fails on fake bytes; sidecar langs should still return when present.
			if len(langs) == 0 {
				t.Fatalf("want sidecar langs despite probe err: %v", err)
			}
		}
		if _, ok := langs["en"]; !ok {
			t.Fatalf("missing en: %+v", langs)
		}
		if _, ok := langs["ru"]; !ok {
			t.Fatalf("missing ru: %+v", langs)
		}
	})
}
