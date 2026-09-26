package version

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestIdentity(t *testing.T) {
	cases := []struct {
		name     string
		version  string
		build    string
		ref      string
		revision string
		want     string
	}{
		{
			name:    "defaults",
			version: "0.1.0",
			want:    "v0.1.0",
		},
		{
			name:     "dev image",
			version:  "0.1.0",
			build:    "dev",
			ref:      "dev",
			revision: "574a8e1deadbeef",
			want:     "v0.1.0 · dev · 574a8e1",
		},
		{
			name:     "release ref deduped",
			version:  "0.2.0",
			build:    "0.2.0",
			ref:      "v0.2.0",
			revision: "abc1234",
			want:     "v0.2.0 · abc1234",
		},
		{
			name:    "local ref uses build",
			version: "0.1.0",
			ref:     "local",
			build:   "dev",
			want:    "v0.1.0 · dev",
		},
	}

	for _, tc := range cases {
		allure.Test(t, tc.name, func(a *allure.Context) {
			t := a.T()

			prevVersion, prevBuild, prevRef, prevRevision := Version, Build, Ref, Revision
			t.Cleanup(func() {
				Version, Build, Ref, Revision = prevVersion, prevBuild, prevRef, prevRevision
			})

			Version = tc.version
			Build = tc.build
			Ref = tc.ref
			Revision = tc.revision

			if got := Identity(); got != tc.want {
				t.Fatalf("Identity() = %q, want %q", got, tc.want)
			}
		})
	}
}
