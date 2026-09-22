package transcode

import (
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestNormalizeSubtitleUpload_VTTAndSRT(t *testing.T) {
	t.Parallel()

	allure.Test(t, "NormalizeSubtitleUpload accepts VTT and converts SRT", func(a *allure.Context) {
		t := a.T()
		vtt, err := NormalizeSubtitleUpload(
			[]byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nHi\n"),
			"a.vtt",
		)
		if err != nil || !strings.Contains(string(vtt), "Hi") {
			t.Fatalf("vtt: %v %q", err, vtt)
		}

		srt, err := NormalizeSubtitleUpload(
			[]byte("1\n00:00:00,000 --> 00:00:01,000\nHello\n\n"),
			"a.srt",
		)
		if err != nil || !strings.Contains(string(srt), "WEBVTT") ||
			!strings.Contains(string(srt), "Hello") {
			t.Fatalf("srt: %v %q", err, srt)
		}
	})
}
