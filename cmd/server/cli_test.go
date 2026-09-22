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

func TestParseCLI_VersionFlags(t *testing.T) {
	t.Parallel()

	allure.Test(t, "-v and --version request version output", func(a *allure.Context) {
		t := a.T()
		for _, args := range [][]string{
			{"-v"},
			{"--version"},
			{"-version"},
		} {
			showVersion, err := parseCLI(args)
			if err != nil {
				t.Fatalf("parse %v: %v", args, err)
			}
			if !showVersion {
				t.Fatalf("expected version flag for %v", args)
			}
		}

		showVersion, err := parseCLI(nil)
		if err != nil || showVersion {
			t.Fatalf("expected no version for empty args: show=%v err=%v", showVersion, err)
		}
	})

	allure.Test(t, "invalid flags return parse error", func(a *allure.Context) {
		t := a.T()
		_, err := parseCLI([]string{"-not-a-real-flag"})
		if err == nil {
			t.Fatal("expected parse error")
		}
	})
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

func TestExitIfVersionRequested_InvalidFlagExits(t *testing.T) {
	t.Parallel()

	if os.Getenv("SUDOSTREAM_CLI_REEXEC") == "invalid" {
		os.Args = []string{"sudostream", "-not-a-real-flag"}
		exitIfVersionRequested()
		t.Fatal("expected exit")
	}

	//nolint:gosec // re-execs the current test binary with a fixed argument list
	cmd := exec.CommandContext(
		context.Background(),
		os.Args[0],
		"-test.run=^TestExitIfVersionRequested_InvalidFlagExits$",
		"-test.count=1",
	)
	cmd.Env = append(os.Environ(), "SUDOSTREAM_CLI_REEXEC=invalid")
	err := cmd.Run()
	exitErr := &exec.ExitError{}
	ok := errors.As(err, &exitErr)
	if !ok || !exitErr.Exited() || exitErr.ExitCode() != exitCodeUsage {
		t.Fatalf("expected exit code %d, got %v", exitCodeUsage, err)
	}
}

func TestExitIfVersionRequested_VersionFlagExitsZero(t *testing.T) {
	t.Parallel()

	if os.Getenv("SUDOSTREAM_CLI_REEXEC") == "version" {
		os.Args = []string{"sudostream", "-version"}
		exitIfVersionRequested()
		t.Fatal("expected exit")
	}

	//nolint:gosec // re-execs the current test binary with a fixed argument list
	cmd := exec.CommandContext(
		context.Background(),
		os.Args[0],
		"-test.run=^TestExitIfVersionRequested_VersionFlagExitsZero$",
		"-test.count=1",
	)
	cmd.Env = append(os.Environ(), "SUDOSTREAM_CLI_REEXEC=version")
	err := cmd.Run()
	if err != nil {
		t.Fatalf("expected version exit 0, got %v", err)
	}
}
