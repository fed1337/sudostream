package thumbnail

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestMayHaveEmbeddedCover_ByExtension(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{name: "mkv can embed", path: "/media/show.mkv", want: true},
		{name: "mp4 can embed", path: "/media/film.mp4", want: true},
		{name: "webm skips probe", path: "/media/clip.webm", want: false},
		{name: "mpeg-ts skips probe", path: "/media/broadcast.ts", want: false},
		{name: "elementary h264 skips", path: "/media/raw.h264", want: false},
	} {
		allure.Test(t, "mayHaveEmbeddedCover — "+tc.name, func(a *allure.Context) {
			t := a.T()
			if got := mayHaveEmbeddedCover(tc.path); got != tc.want {
				t.Fatalf("mayHaveEmbeddedCover(%q)=%v want %v", tc.path, got, tc.want)
			}
		})
	}
}
