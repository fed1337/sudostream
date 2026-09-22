package observability

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	// DefaultLogDir is where sudostream.log is written (compose volume).
	DefaultLogDir = "/var/lib/sudostream/logs"

	defaultLogFileName = "sudostream.log"
	logDirPerm         = 0o750
	logFilePerm        = 0o640
)

// logDirOverride is set only in tests.
var logDirOverride string

func effectiveLogDir() string {
	if logDirOverride != "" {
		return logDirOverride
	}

	return DefaultLogDir
}

// ConfigureFileLogging appends slog output to DefaultLogDir/sudostream.log and stdout.
// No rotation — operators manage the volume on the host.
// Returns a closer for the log file (may be a no-op).
func ConfigureFileLogging() (func() error, error) {
	dir := effectiveLogDir()
	err := os.MkdirAll(dir, logDirPerm)
	if err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	path := filepath.Join(dir, defaultLogFileName)
	// #nosec G304 -- fixed path under /var/lib/sudostream/logs (or test override)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePerm)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	handler := slog.NewTextHandler(io.MultiWriter(os.Stdout, file), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))

	return file.Close, nil
}
