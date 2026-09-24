package skipsegment

import (
	"math"
	"regexp"
	"strings"
)

const (
	minIntroSec         = 10
	maxIntroSec         = 150
	maxScanWindowSec    = 600 // 10 minutes
	maxScanRuntimeRatio = 0.30
)

var (
	recapTitleRe = regexp.MustCompile(`(?i)\b(recap|previously(\s+on)?|summary)\b`)
	introTitleRe = regexp.MustCompile(
		`(?i)\b(intro|opening|theme|title sequence|main titles)\b|` +
			`(?i)(^|[\s\-—(])op([\s\-—)]|$)`,
	)
)

// IntroFromChapters picks the first valid opening intro from embedded chapters.
// Returns nil when no qualifying chapter exists (prefer miss over bad skip).
func IntroFromChapters(chapters []ChapterCue, durationSeconds float64) *Intro {
	if len(chapters) == 0 || durationSeconds <= 0 {
		return nil
	}

	scanWindow := introScanWindowSec(durationSeconds)

	for index, chapter := range chapters {
		if !titleMatchesIntro(chapter.Title) || titleMatchesRecap(chapter.Title) {
			continue
		}
		if index > 0 && titleMatchesIntro(chapters[index-1].Title) &&
			!titleMatchesRecap(chapters[index-1].Title) {
			continue
		}

		start := chapter.StartSeconds
		end := chapter.EndSeconds
		if end <= start {
			continue
		}
		duration := end - start
		if duration < minIntroSec || duration > maxIntroSec {
			continue
		}
		if start < 0 || start > scanWindow {
			continue
		}

		return &Intro{
			StartMs: secondsToMs(start),
			EndMs:   secondsToMs(end),
			Source:  SourceChapter,
		}
	}

	return nil
}

func introScanWindowSec(durationSeconds float64) float64 {
	cap := durationSeconds * maxScanRuntimeRatio
	if cap > maxScanWindowSec {
		cap = maxScanWindowSec
	}
	return cap
}

func titleMatchesIntro(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	return introTitleRe.MatchString(title)
}

func titleMatchesRecap(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	return recapTitleRe.MatchString(title)
}

func secondsToMs(seconds float64) int64 {
	if seconds <= 0 {
		return 0
	}
	return int64(math.Round(seconds * 1000))
}
