package skipsegment

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestIntroFromChapters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		chapters  []ChapterCue
		duration  float64
		wantNil   bool
		wantStart int64
		wantEnd   int64
	}{
		{
			name: "opening theme within window",
			chapters: []ChapterCue{
				{StartSeconds: 0, EndSeconds: 45, Title: "Cold open"},
				{StartSeconds: 45, EndSeconds: 135, Title: "Opening Theme"},
			},
			duration:  3600,
			wantStart: 45000,
			wantEnd:   135000,
		},
		{
			name: "recap ignored",
			chapters: []ChapterCue{
				{StartSeconds: 0, EndSeconds: 60, Title: "Previously on"},
			},
			duration: 3600,
			wantNil:  true,
		},
		{
			name: "junk chapter ignored",
			chapters: []ChapterCue{
				{StartSeconds: 0, EndSeconds: 90, Title: "Chapter 01"},
			},
			duration: 3600,
			wantNil:  true,
		},
		{
			name: "too short",
			chapters: []ChapterCue{
				{StartSeconds: 0, EndSeconds: 5, Title: "Intro"},
			},
			duration: 3600,
			wantNil:  true,
		},
		{
			name: "too long",
			chapters: []ChapterCue{
				{StartSeconds: 0, EndSeconds: 200, Title: "Intro"},
			},
			duration: 3600,
			wantNil:  true,
		},
		{
			name: "late start outside scan window",
			chapters: []ChapterCue{
				{StartSeconds: 400, EndSeconds: 490, Title: "Opening"},
			},
			duration: 1200,
			wantNil:  true,
		},
		{
			name: "adjacent intro pair keeps first",
			chapters: []ChapterCue{
				{StartSeconds: 0, EndSeconds: 90, Title: "Opening"},
				{StartSeconds: 90, EndSeconds: 150, Title: "Main Titles"},
			},
			duration:  3600,
			wantStart: 0,
			wantEnd:   90000,
		},
		{
			name: "OP word boundary",
			chapters: []ChapterCue{
				{StartSeconds: 10, EndSeconds: 100, Title: "OP"},
			},
			duration:  1800,
			wantStart: 10000,
			wantEnd:   100000,
		},
		{
			name: "stop motion not OP",
			chapters: []ChapterCue{
				{StartSeconds: 0, EndSeconds: 90, Title: "Stop motion"},
			},
			duration: 1800,
			wantNil:  true,
		},
	} {
		allure.Test(t, "chapter intro detection — "+tc.name, func(a *allure.Context) {
			t := a.T()
			intro := IntroFromChapters(tc.chapters, tc.duration)
			if tc.wantNil {
				if intro != nil {
					t.Fatalf("expected nil, got %+v", intro)
				}

				return
			}
			if intro == nil {
				t.Fatal("expected intro segment")
			}
			if intro.StartMs != tc.wantStart || intro.EndMs != tc.wantEnd {
				t.Fatalf("range: %+v want %d-%d", intro, tc.wantStart, tc.wantEnd)
			}
			if intro.Source != SourceChapter {
				t.Fatalf("source: %q", intro.Source)
			}
		})
	}
}
