// Package version holds release semver and optional CI build metadata (ldflags).
package version

import "strings"

const (
	shortRevisionLen = 7
	buildMetaUnknown = "unknown"
	buildMetaLocal   = "local"
)

// Version is the application release version (single source of truth; bumped by commitizen).
//
//nolint:gochecknoglobals // compile-time semver baked into the binary from this var.
var Version = "0.1.0"

// Build identifies the CI/build channel (e.g. dev or release semver from BUILD_VERSION).
//
//nolint:gochecknoglobals // injected at link time via -ldflags -X.
var Build = ""

// Revision is the VCS revision baked into the binary (short SHA when built in Docker/CI).
//
//nolint:gochecknoglobals // injected at link time via -ldflags -X.
var Revision = ""

// Ref is the branch or image ref name (IMAGE_REF_NAME, e.g. dev or v0.2.0).
//
//nolint:gochecknoglobals // injected at link time via -ldflags -X.
var Ref = ""

// Identity returns a human-readable build line: v{semver} · {ref} · {revision}.
func Identity() string {
	parts := identityParts()
	if len(parts) == 0 {
		return "v" + Version
	}

	return "v" + Version + " · " + strings.Join(parts, " · ")
}

func identityParts() []string {
	ref := strings.TrimSpace(Ref)
	if ref == "" || ref == buildMetaLocal || ref == buildMetaUnknown {
		ref = strings.TrimSpace(Build)
	}

	rev := shortRevision(Revision)

	var parts []string
	if ref != "" && ref != buildMetaLocal && ref != buildMetaUnknown && ref != Version && ref != "v"+Version {
		parts = append(parts, ref)
	}
	if rev != "" {
		parts = append(parts, rev)
	}

	return parts
}

func shortRevision(revision string) string {
	rev := strings.TrimSpace(revision)
	if rev == "" || rev == buildMetaUnknown {
		return ""
	}
	if len(rev) > shortRevisionLen {
		return rev[:shortRevisionLen]
	}

	return rev
}
