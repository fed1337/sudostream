package maintenance

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestValidateCron(t *testing.T) {
	t.Parallel()

	allure.Test(t, "validates 5-field cron expressions", func(a *allure.Context) {
		t := a.T()
		err := ValidateCron("")
		if err != nil {
			t.Fatalf("empty cron: %v", err)
		}
		err = ValidateCron("0 3 * * *")
		if err != nil {
			t.Fatalf("valid cron: %v", err)
		}
		err = ValidateCron("not a cron")
		if err == nil {
			t.Fatal("expected invalid cron")
		}
	})
}

//nolint:cyclop // straight-line action catalog assertions
func TestLockKeyAndActionHelpers(t *testing.T) {
	t.Parallel()

	allure.Test(t, "action catalog helpers", func(a *allure.Context) {
		t := a.T()
		if !IsGlobalAction(ActionTrashPurge) || IsLibraryAction(ActionTrashPurge) {
			t.Fatal("trash.purge scope")
		}
		if !IsLibraryAction(ActionMetadataScan) || IsGlobalAction(ActionMetadataScan) {
			t.Fatal("metadata.scan scope")
		}
		for _, action := range []string{
			ActionProvidersMetadata, ActionProvidersPosters, ActionProvidersSubtitles,
		} {
			if !IsLibraryAction(action) || IsGlobalAction(action) {
				t.Fatalf("%s scope", action)
			}
			if !ValidAction(action) {
				t.Fatalf("%s should be a valid action", action)
			}
		}
		if LockKey(ActionMetadataScan, "abc") != "metadata.scan|abc" {
			t.Fatal("lock key")
		}
		if PurgeRetentionHours != 6 {
			t.Fatalf("purge retention want 6, got %d", PurgeRetentionHours)
		}
	})
}
