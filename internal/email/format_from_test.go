package email

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestFormatFromHeader(t *testing.T) {
	t.Parallel()

	allure.Test(t, "formats display name and address for From header", func(a *allure.Context) {
		t := a.T()
		if got := formatFromHeader("", "a@b.c"); got != "a@b.c" {
			t.Fatalf("empty name: %q", got)
		}
		if got := formatFromHeader(
			`Ada "Lovelace"`,
			"ada@example.com",
		); got != `"Ada \"Lovelace\"" <ada@example.com>` {
			t.Fatalf("quoted name: %q", got)
		}
	})
}
