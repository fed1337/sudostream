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
	maxStartSkewSec     = 180 // ±3 minutes across episodes
	minSeasonAgreement  = 0.50
	msPerSecond         = 1000

	// Chromaprint hop for ffmpeg -algorithm 1 (AcoustID default / TEST2).
	// frame 4096 with 2/3 overlap at 11025 Hz.
	chromaprintFrameSamples    = 4096.0
	chromaprintOverlapDiv      = 3.0
	chromaprintSampleHz        = 11025.0
	chromaprintItemDurationSec = chromaprintFrameSamples / (chromaprintOverlapDiv * chromaprintSampleHz)

	// bitMatchThreshold is the minimum local bit-agreement for a match run.
	// Random unrelated uint32 pairs average ~0.5; keep clear margin above that.
	bitMatchThreshold = 0.70
	// localWindowItems smooths per-item scores before run detection.
	localWindowItems = 5
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
//
//nolint:cyclop // single-pass scan with explicit guards; splitting would obscure rules
func IntroFromChapters(chapters []ChapterCue, durationSeconds float64) *Intro {
	if len(chapters) == 0 || durationSeconds <= 0 {
		return nil
	}

	scanWindow := introScanWindowSec(durationSeconds)
	prevTitleWasIntro := false

	for _, chapter := range chapters {
		if !titleMatchesIntro(chapter.Title) || titleMatchesRecap(chapter.Title) {
			prevTitleWasIntro = false

			continue
		}
		if prevTitleWasIntro {
			continue
		}

		start := chapter.StartSeconds
		end := chapter.EndSeconds
		if end <= start {
			prevTitleWasIntro = true

			continue
		}
		duration := end - start
		if duration < minIntroSec || duration > maxIntroSec {
			prevTitleWasIntro = true

			continue
		}
		if start < 0 || start > scanWindow {
			prevTitleWasIntro = true

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

// IntroScanWindowSec returns the audio/chapter scan cap for one episode.
func IntroScanWindowSec(durationSeconds float64) float64 {
	if durationSeconds <= 0 {
		return 0
	}
	windowSec := durationSeconds * maxScanRuntimeRatio
	if windowSec > maxScanWindowSec {
		windowSec = maxScanWindowSec
	}

	return windowSec
}

func introScanWindowSec(durationSeconds float64) float64 {
	return IntroScanWindowSec(durationSeconds)
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

	return int64(math.Round(seconds * msPerSecond))
}
