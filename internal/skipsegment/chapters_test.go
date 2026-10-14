package skipsegment

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestIntroFromChapters_MatchesOpeningTheme(t *testing.T) {
	t.Parallel()

	allure.Test(t, "chapter intro title within duration and scan window", func(a *allure.Context) {
		t := a.T()
		intro := IntroFromChapters([]ChapterCue{
			{StartSeconds: 0, EndSeconds: 45, Title: "Cold open"},
			{StartSeconds: 45, EndSeconds: 135, Title: "Opening Theme"},
		}, 3600)
		if intro == nil {
			t.Fatal("expected intro segment")
		}
		if intro.StartMs != 45000 || intro.EndMs != 135000 {
			t.Fatalf("range: %+v", intro)
		}
		if intro.Source != SourceChapter {
			t.Fatalf("source: %q", intro.Source)
		}
	})
}

func TestIntroFromChapters_IgnoresRecapAndJunk(t *testing.T) {
	t.Parallel()

	allure.Test(t, "recap chapters and generic titles produce no intro", func(a *allure.Context) {
		t := a.T()
		if got := IntroFromChapters([]ChapterCue{
			{StartSeconds: 0, EndSeconds: 60, Title: "Previously on"},
		}, 3600); got != nil {
			t.Fatalf("recap: %+v", got)
		}
		if got := IntroFromChapters([]ChapterCue{
			{StartSeconds: 0, EndSeconds: 90, Title: "Chapter 01"},
		}, 3600); got != nil {
			t.Fatalf("junk: %+v", got)
		}
	})
}

func TestIntroFromChapters_DurationAndWindowBounds(t *testing.T) {
	t.Parallel()

	allure.Test(t, "intro must be 10-150s and start inside scan window", func(a *allure.Context) {
		t := a.T()
		if got := IntroFromChapters([]ChapterCue{
			{StartSeconds: 0, EndSeconds: 5, Title: "Intro"},
		}, 3600); got != nil {
			t.Fatalf("too short: %+v", got)
		}
		if got := IntroFromChapters([]ChapterCue{
			{StartSeconds: 0, EndSeconds: 200, Title: "Intro"},
		}, 3600); got != nil {
			t.Fatalf("too long: %+v", got)
		}
		// 30% of 1200s = 360s window; intro starting at 400s is rejected.
		if got := IntroFromChapters([]ChapterCue{
			{StartSeconds: 400, EndSeconds: 490, Title: "Opening"},
		}, 1200); got != nil {
			t.Fatalf("late start: %+v", got)
		}
	})
}

func TestIntroFromChapters_SkipsAdjacentIntroPair(t *testing.T) {
	t.Parallel()

	allure.Test(t, "second consecutive intro chapter is ignored", func(a *allure.Context) {
		t := a.T()
		intro := IntroFromChapters([]ChapterCue{
			{StartSeconds: 0, EndSeconds: 90, Title: "Opening"},
			{StartSeconds: 90, EndSeconds: 150, Title: "Main Titles"},
		}, 3600)
		if intro == nil || intro.EndMs != 90000 {
			t.Fatalf("want first intro only: %+v", intro)
		}
	})
}

func TestIntroFromChapters_OPWordBoundary(t *testing.T) {
	t.Parallel()

	allure.Test(t, "OP token matches anime marker but not substring false positives", func(a *allure.Context) {
		t := a.T()
		intro := IntroFromChapters([]ChapterCue{
			{StartSeconds: 10, EndSeconds: 100, Title: "OP"},
		}, 1800)
		if intro == nil {
			t.Fatal("expected OP chapter")
		}
		if IntroFromChapters([]ChapterCue{
			{StartSeconds: 0, EndSeconds: 90, Title: "Stop motion"},
		}, 1800) != nil {
			t.Fatal("stop should not match op")
		}
	})
}
