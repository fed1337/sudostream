package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sudoStream/internal/version"
)

const exitCodeUsage = 2

// parseCLI returns true when the process should print version and exit.
func parseCLI(args []string) (bool, error) {
	fs := flag.NewFlagSet("sudostream", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&showVersion, "v", false, "print version and exit")

	err := fs.Parse(args)
	if err != nil {
		return false, fmt.Errorf("parse flags: %w", err)
	}

	return showVersion, nil
}

func printVersion() {
	_, _ = fmt.Fprintln(os.Stdout, version.Version)
}

func exitIfVersionRequested() {
	showVersion, err := parseCLI(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "sudostream: %v\n", err)
		os.Exit(exitCodeUsage)
	}
	if showVersion {
		printVersion()
		os.Exit(0)
	}
}
