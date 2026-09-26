package transcode

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSelectDefaultAudioStreamIndex(t *testing.T) {
	t.Parallel()

	streams := []AudioStream{
		{Index: 0, Language: "jpn", Commentary: false},
		{Index: 1, Language: "eng", Commentary: false},
		{Index: 2, Language: "eng", Commentary: true},
	}

	allure.Test(t, "user language preference wins over track order", func(a *allure.Context) {
		t := a.T()
		got := SelectDefaultAudioStreamIndex(streams, []string{"eng"})
		if got != 1 {
			t.Fatalf("got %d want 1", got)
		}
	})

	allure.Test(t, "commentary skipped when non-commentary exists", func(a *allure.Context) {
		t := a.T()
		onlyCommentary := []AudioStream{
			{Language: "eng", Commentary: true},
			{Language: "jpn", Commentary: false},
		}
		got := SelectDefaultAudioStreamIndex(onlyCommentary, nil)
		if got != 1 {
			t.Fatalf("got %d want 1", got)
		}
	})
}
