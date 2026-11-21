package transcode_test

import (
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestFastStartRung_UsesHighestApplicableRung(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		sourceHeight int
		want         int
	}{
		{name: "1080p source picks highest rung", sourceHeight: 1080, want: 1080},
		{name: "480p source picks 480p", sourceHeight: 480, want: 480},
	} {
		allure.Test(t, "faststart rung — "+tc.name, func(a *allure.Context) {
			t := a.T()
			rung := transcode.FastStartRung(tc.sourceHeight)
			if rung.Height != tc.want {
				t.Fatalf("expected %dp faststart, got %d", tc.want, rung.Height)
			}
		})
	}
}

func TestQualityRungs_NoUpscaleBeyondSource(t *testing.T) {
	t.Parallel()

	allure.Test(t, "480p source excludes higher rungs", func(a *allure.Context) {
		t := a.T()
		rungs := transcode.QualityRungs(480)
		if len(rungs) != 3 {
			t.Fatalf("expected three rungs, got %d", len(rungs))
		}
		if rungs[len(rungs)-1].Height != 240 {
			t.Fatalf("expected lowest 240p, got %d", rungs[len(rungs)-1].Height)
		}
	})
}
