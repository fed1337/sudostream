package transcode_test

import (
	"sudoStream/internal/transcode"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestFastStartRung_UsesHighestApplicableRung(t *testing.T) {
	t.Parallel()

	allure.Test(t, "1080p source picks highest rung for faststart", func(a *allure.Context) {
		t := a.T()
		rung := transcode.FastStartRung(1080)
		if rung.Height != 1080 {
			t.Fatalf("expected 1080p faststart, got %d", rung.Height)
		}
	})

	allure.Test(t, "480p source picks 480p for faststart", func(a *allure.Context) {
		t := a.T()
		rung := transcode.FastStartRung(480)
		if rung.Height != 480 {
			t.Fatalf("expected 480p faststart, got %d", rung.Height)
		}
	})
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
