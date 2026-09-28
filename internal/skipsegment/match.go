package skipsegment

import (
	"math"
	"math/bits"
)

const (
	bitsPerFingerprintItem = 32
)

// MatchRange is a contiguous fingerprint alignment between two episodes.
type MatchRange struct {
	StartA      int // inclusive item index in A
	EndA        int // exclusive item index in A
	StartB      int
	EndB        int
	Offset      int // B index - A index at alignment
	Score       float64
	DurationSec float64
}

// LongestMatch finds the best contiguous high-similarity run between two
// chromaprint raw fingerprints, searching start offsets within ±maxSkewSec.
// Returns nil when no run meets minIntroSec / score policy.
func LongestMatch(left, right []uint32, maxSkewSec float64) *MatchRange {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}

	maxSkewItems := max(int(math.Ceil(maxSkewSec/chromaprintItemDurationSec)), 0)

	minItems := int(math.Ceil(float64(minIntroSec) / chromaprintItemDurationSec))
	maxItems := int(math.Floor(float64(maxIntroSec) / chromaprintItemDurationSec))
	if minItems < 1 {
		minItems = 1
	}
	if maxItems < minItems {
		maxItems = minItems
	}

	var best *MatchRange
	for offset := -maxSkewItems; offset <= maxSkewItems; offset++ {
		candidate := bestRunAtOffset(left, right, offset, minItems, maxItems)
		if candidate == nil {
			continue
		}
		if betterMatch(candidate, best) {
			copyRange := *candidate
			best = &copyRange
		}
	}

	return best
}

//nolint:cyclop // run scanner with max-window trim
func bestRunAtOffset(left, right []uint32, offset, minItems, maxItems int) *MatchRange {
	startA := 0
	if offset < 0 {
		startA = -offset
	}
	endA := min(len(left), len(right)-offset)
	if endA-startA < minItems {
		return nil
	}

	scores := make([]float64, endA-startA)
	for index := startA; index < endA; index++ {
		scores[index-startA] = bitAgreement(left[index], right[index+offset])
	}
	smoothed := smoothScores(scores, localWindowItems)

	var best *MatchRange
	runStart := -1
	for index := 0; index <= len(smoothed); index++ {
		above := index < len(smoothed) && smoothed[index] >= bitMatchThreshold
		if above {
			if runStart < 0 {
				runStart = index
			}

			continue
		}
		if runStart < 0 {
			continue
		}
		runEnd := index
		if runEnd-runStart > maxItems {
			runStart, runEnd = bestSubwindow(smoothed, runStart, runEnd, maxItems)
		}
		if runEnd-runStart >= minItems {
			candidate := matchFromRun(offset, startA+runStart, startA+runEnd, smoothed[runStart:runEnd])
			if betterMatch(candidate, best) {
				best = candidate
			}
		}
		runStart = -1
	}

	return best
}

func betterMatch(candidate, best *MatchRange) bool {
	if candidate == nil {
		return false
	}
	if best == nil {
		return true
	}
	if candidate.Score > best.Score {
		return true
	}

	return candidate.Score == best.Score && candidate.DurationSec > best.DurationSec
}

func matchFromRun(offset, startA, endA int, windowScores []float64) *MatchRange {
	sum := 0.0
	for _, score := range windowScores {
		sum += score
	}
	avg := sum / float64(len(windowScores))
	duration := float64(endA-startA) * chromaprintItemDurationSec

	return &MatchRange{
		StartA:      startA,
		EndA:        endA,
		StartB:      startA + offset,
		EndB:        endA + offset,
		Offset:      offset,
		Score:       avg,
		DurationSec: duration,
	}
}

func bestSubwindow(scores []float64, runStart, runEnd, window int) (int, int) {
	if runEnd-runStart <= window {
		return runStart, runEnd
	}
	bestStart := runStart
	bestSum := 0.0
	for index := runStart; index < runStart+window; index++ {
		bestSum += scores[index]
	}
	sum := bestSum
	for start := runStart + 1; start+window <= runEnd; start++ {
		sum += scores[start+window-1] - scores[start-1]
		if sum > bestSum {
			bestSum = sum
			bestStart = start
		}
	}

	return bestStart, bestStart + window
}

func smoothScores(scores []float64, window int) []float64 {
	if window <= 1 || len(scores) == 0 {
		out := make([]float64, len(scores))
		copy(out, scores)

		return out
	}
	half := window / 2 //nolint:mnd // centered moving average
	out := make([]float64, len(scores))
	for index := range scores {
		start := max(index-half, 0)
		end := min(index+half+1, len(scores))
		sum := 0.0
		for cursor := start; cursor < end; cursor++ {
			sum += scores[cursor]
		}
		out[index] = sum / float64(end-start)
	}

	return out
}

func bitAgreement(left, right uint32) float64 {
	return float64(bitsPerFingerprintItem-bits.OnesCount32(left^right)) / float64(bitsPerFingerprintItem)
}

// ItemIndexToMs converts a chromaprint item index to content milliseconds
// within a scan window that starts at t=0.
func ItemIndexToMs(index int) int64 {
	if index <= 0 {
		return 0
	}

	return int64(math.Round(float64(index) * chromaprintItemDurationSec * msPerSecond))
}
