package httpapi

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestIsVideoFilename_DetectsCommonExtensions(t *testing.T) {
	t.Parallel()

	allure.Test(t, "video extensions are recognized", func(a *allure.Context) {
		t := a.T()
		for _, name := range []string{"a.mp4", "b.MKV", "clip.webm"} {
			if !isVideoFilename(name) {
				t.Fatalf("expected %q to be video", name)
			}
		}
		if isVideoFilename("notes.txt") {
			t.Fatal("expected txt to be non-video")
		}
	})
}

func TestFirstMediaPathSegment_ReturnsTopLevelFolder(t *testing.T) {
	t.Parallel()

	allure.Test(t, "first segment is library folder", func(a *allure.Context) {
		t := a.T()
		if got := firstMediaPathSegment("series/show/s01e01.mp4"); got != "series" {
			t.Fatalf("got %q want series", got)
		}
	})
}
