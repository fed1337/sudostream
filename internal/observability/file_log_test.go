package observability

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestConfigureFileLogging_WritesToDir(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ConfigureFileLogging appends to sudostream.log", func(a *allure.Context) {
		t := a.T()
		dir := t.TempDir()
		prev := logDirOverride
		logDirOverride = dir
		t.Cleanup(func() {
			logDirOverride = prev
		})

		closeLog, err := ConfigureFileLogging()
		if err != nil {
			t.Fatalf("configure: %v", err)
		}
		t.Cleanup(func() {
			_ = closeLog()
		})

		slog.Info("file-log-smoke", "ok", true)

		// #nosec G304 -- temp dir from t.TempDir
		raw, err := os.ReadFile(filepath.Join(dir, "sudostream.log"))
		if err != nil {
			t.Fatalf("read log: %v", err)
		}
		if !strings.Contains(string(raw), "file-log-smoke") {
			t.Fatalf("log missing message: %s", raw)
		}
	})
}
