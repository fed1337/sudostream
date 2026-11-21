package httpapi

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestIsVideoFilename_DetectsCommonExtensions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		file string
		want bool
	}{
		{name: "mp4", file: "a.mp4", want: true},
		{name: "mkv uppercase", file: "b.MKV", want: true},
		{name: "webm", file: "clip.webm", want: true},
		{name: "txt", file: "notes.txt", want: false},
	} {
		allure.Test(t, "video extensions are recognized — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if got := isVideoFilename(tc.file); got != tc.want {
				t.Fatalf("isVideoFilename(%q)=%v want %v", tc.file, got, tc.want)
			}
		})
	}
}

func TestFirstMediaPathSegment_ReturnsTopLevelFolder(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "nested file", path: "series/show/s01e01.mp4", want: "series"},
	} {
		allure.Test(t, "first segment is library folder — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if got := firstMediaPathSegment(tc.path); got != tc.want {
				t.Fatalf("firstMediaPathSegment(%q)=%q want %q", tc.path, got, tc.want)
			}
		})
	}
}
