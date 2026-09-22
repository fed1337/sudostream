package watch

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestIsComplete(t *testing.T) {
	t.Parallel()

	allure.Test(t, "complete uses 90 percent or last 30 seconds", func(a *allure.Context) {
		t := a.T()
		if IsComplete(50, 100) {
			t.Fatal("50/100 should not complete")
		}
		if !IsComplete(70, 100) {
			t.Fatal("30s remaining of 100s should complete")
		}
		if !IsComplete(90, 100) {
			t.Fatal("90/100 should complete")
		}
		if IsComplete(10, 0) {
			t.Fatal("unknown duration should not complete")
		}
	})
}

func TestShouldSaveProgress(t *testing.T) {
	t.Parallel()

	allure.Test(t, "progress under 10 seconds is ignored", func(a *allure.Context) {
		t := a.T()
		if ShouldSaveProgress(9.9) {
			t.Fatal("expected skip")
		}
		if !ShouldSaveProgress(10) {
			t.Fatal("expected save")
		}
	})
}
