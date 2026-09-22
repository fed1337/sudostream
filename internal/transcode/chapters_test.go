package transcode

import (
	"encoding/json"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestMapProbeChapters_TitlesAndFallback(t *testing.T) {
	t.Parallel()

	allure.Test(t, "mapProbeChapters keeps titles and fills Chapter N", func(a *allure.Context) {
		t := a.T()
		got := mapProbeChapters([]probeChapter{
			{
				StartTime: "0.000000",
				EndTime:   "60.500000",
				Tags:      map[string]string{"title": " Intro "},
			},
			{
				StartTime: "60.5",
				EndTime:   "120",
			},
		})
		if len(got) != 2 {
			t.Fatalf("len: got %d", len(got))
		}
		if got[0].Title != "Intro" || got[0].StartSeconds != 0 || got[0].EndSeconds != 60.5 {
			t.Fatalf("first: %+v", got[0])
		}
		if got[1].Title != "Chapter 2" || got[1].StartSeconds != 60.5 {
			t.Fatalf("second untitled fallback: %+v", got[1])
		}
	})
}

func TestMapProbeChapters_Empty(t *testing.T) {
	t.Parallel()

	allure.Test(t, "mapProbeChapters returns empty non-nil slice", func(a *allure.Context) {
		t := a.T()
		got := mapProbeChapters(nil)
		if got == nil || len(got) != 0 {
			t.Fatalf("got %#v", got)
		}
	})
}

func TestProbeOutput_UnmarshalsChapters(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ffprobe chapter JSON unmarshals into probeOutput", func(a *allure.Context) {
		t := a.T()
		raw := []byte(`{
			"chapters": [
				{"start_time":"1.0","end_time":"2.0","tags":{"title":"A"}}
			]
		}`)
		var parsed probeOutput
		err := json.Unmarshal(raw, &parsed)
		if err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		chapters := mapProbeChapters(parsed.Chapters)
		if len(chapters) != 1 || chapters[0].Title != "A" {
			t.Fatalf("chapters: %+v", chapters)
		}
	})
}
