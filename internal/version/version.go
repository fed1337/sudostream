// Package version holds the application release version.
package version

// Version is the application release version (single source of truth; bumped by commitizen).
//
//nolint:gochecknoglobals // compile-time semver baked into the binary from this var.
var Version = "0.1.0"
