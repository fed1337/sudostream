package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sudoStream/internal/version"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseCLI(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		args        []string
		wantVersion bool
		wantErr     bool
	}{
		{name: "short -v", args: []string{"-v"}, wantVersion: true},
		{name: "long --version", args: []string{"--version"}, wantVersion: true},
		{name: "dash -version", args: []string{"-version"}, wantVersion: true},
		{name: "empty args", args: nil, wantVersion: false},
		{name: "invalid flag", args: []string{"-not-a-real-flag"}, wantErr: true},
	} {
		allure.Test(t, "parse CLI — "+tc.name, func(a *allure.Context) {
			t := a.T()
			showVersion, err := parseCLI(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected parse error")
				}

				return
			}
			if err != nil {
				t.Fatalf("parse %v: %v", tc.args, err)
			}
			if showVersion != tc.wantVersion {
				t.Fatalf("showVersion=%v want %v", showVersion, tc.wantVersion)
			}
		})
	}
}

func TestPrintVersion_WritesVersionToStdout(t *testing.T) {
	t.Parallel()

	allure.Test(t, "printVersion writes internal/version to stdout", func(a *allure.Context) {
		t := a.T()
		oldStdout := os.Stdout
		readPipe, writePipe, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}
		os.Stdout = writePipe

		printVersion()

		err = writePipe.Close()
		if err != nil {
			t.Fatalf("close write pipe: %v", err)
		}
		os.Stdout = oldStdout

		var buf bytes.Buffer
		_, err = io.Copy(&buf, readPipe)
		if err != nil {
			t.Fatalf("read stdout: %v", err)
		}
		err = readPipe.Close()
		if err != nil {
			t.Fatalf("close read pipe: %v", err)
		}

		got := bytes.TrimSpace(buf.Bytes())
		if string(got) != version.Version {
			t.Fatalf("stdout: got %q want %q", got, version.Version)
		}
	})
}

func TestExitIfVersionRequested(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		reexec   string
		args     []string
		wantCode int
		wantZero bool
	}{
		{
			name:     "invalid flag exits usage",
			reexec:   "invalid",
			args:     []string{"sudostream", "-not-a-real-flag"},
			wantCode: exitCodeUsage,
		},
		{
			name:     "version flag exits zero",
			reexec:   "version",
			args:     []string{"sudostream", "-version"},
			wantZero: true,
		},
	} {
		allure.Test(t, "exit if version requested — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if os.Getenv("SUDOSTREAM_CLI_REEXEC") == tc.reexec {
				os.Args = tc.args
				exitIfVersionRequested()
				t.Fatal("expected exit")
			}

			cmd := exec.CommandContext( //nolint:gosec // re-execs current test binary
				context.Background(),
				os.Args[0],
				"-test.run=^TestExitIfVersionRequested$",
				"-test.count=1",
			)
			cmd.Env = append(os.Environ(), "SUDOSTREAM_CLI_REEXEC="+tc.reexec)
			err := cmd.Run()
			if tc.wantZero {
				if err != nil {
					t.Fatalf("expected exit 0, got %v", err)
				}

				return
			}
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || !exitErr.Exited() || exitErr.ExitCode() != tc.wantCode {
				t.Fatalf("expected exit code %d, got %v", tc.wantCode, err)
			}
		})
	}
}
