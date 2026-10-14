package skipsegment

import (
	"math"
	"slices"
	"sort"
)

const minEpisodesForAudio = 2

// ConsensusHit is one episode's intro range derived from season audio matching.
type ConsensusHit struct {
	RelPath    string
	StartMs    int64
	EndMs      int64
	Confidence float64
}

type introVote struct {
	relPath string
	startMs int64
	endMs   int64
	score   float64
}

// ConsensusIntros clusters pairwise fingerprint matches into per-episode intro
// ranges. Requires ≥2 episodes and ≥50% season agreement.
//

func ConsensusIntros(episodes []EpisodeAudio) []ConsensusHit {
	if len(episodes) < minEpisodesForAudio {
		return nil
	}

	votesByPath := collectPairwiseVotes(episodes)
	participating := 0
	for _, votes := range votesByPath {
		if len(votes) > 0 {
			participating++
		}
	}
	if participating < minEpisodesForAudio ||
		float64(participating) < float64(len(episodes))*minSeasonAgreement {
		return nil
	}

	partnerFloor := min(max(int(math.Ceil(float64(len(episodes))*minSeasonAgreement)), 1), len(episodes)-1)

	hits := make([]ConsensusHit, 0, len(votesByPath))
	for path, votes := range votesByPath {
		if len(votes) < partnerFloor {
			continue
		}
		startMs, endMs, conf := medianRange(votes)
		durationMs := endMs - startMs
		if durationMs < int64(minIntroSec*msPerSecond) ||
			durationMs > int64(maxIntroSec*msPerSecond) {
			continue
		}
		hits = append(hits, ConsensusHit{
			RelPath:    path,
			StartMs:    startMs,
			EndMs:      endMs,
			Confidence: conf,
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].RelPath < hits[j].RelPath
	})

	return hits
}

func collectPairwiseVotes(episodes []EpisodeAudio) map[string][]introVote {
	votesByPath := make(map[string][]introVote, len(episodes))
	for index, left := range episodes {
		for other := index + 1; other < len(episodes); other++ {
			right := episodes[other]
			if len(left.Fingerprint) == 0 || len(right.Fingerprint) == 0 {
				continue
			}
			match := LongestMatch(left.Fingerprint, right.Fingerprint, maxStartSkewSec)
			if match == nil {
				continue
			}
			if match.DurationSec < float64(minIntroSec) || match.DurationSec > float64(maxIntroSec) {
				continue
			}
			leftStart := ItemIndexToMs(match.StartA)
			leftEnd := ItemIndexToMs(match.EndA)
			rightStart := ItemIndexToMs(match.StartB)
			rightEnd := ItemIndexToMs(match.EndB)
			if !validIntroBounds(leftStart, leftEnd, left.ScanWindowSec) ||
				!validIntroBounds(rightStart, rightEnd, right.ScanWindowSec) {
				continue
			}
			votesByPath[left.RelPath] = append(votesByPath[left.RelPath], introVote{
				relPath: left.RelPath,
				startMs: leftStart,
				endMs:   leftEnd,
				score:   match.Score,
			})
			votesByPath[right.RelPath] = append(votesByPath[right.RelPath], introVote{
				relPath: right.RelPath,
				startMs: rightStart,
				endMs:   rightEnd,
				score:   match.Score,
			})
		}
	}

	return votesByPath
}

func validIntroBounds(startMs, endMs int64, scanWindowSec float64) bool {
	if endMs <= startMs {
		return false
	}
	durationMs := endMs - startMs
	if durationMs < int64(minIntroSec*msPerSecond) || durationMs > int64(maxIntroSec*msPerSecond) {
		return false
	}
	if startMs < 0 {
		return false
	}
	if scanWindowSec > 0 && float64(startMs) > scanWindowSec*msPerSecond {
		return false
	}

	return true
}

func medianRange(votes []introVote) (int64, int64, float64) {
	starts := make([]int64, len(votes))
	ends := make([]int64, len(votes))
	sum := 0.0
	for index, item := range votes {
		starts[index] = item.startMs
		ends[index] = item.endMs
		sum += item.score
	}
	slices.Sort(starts)
	slices.Sort(ends)
	mid := len(votes) / 2 //nolint:mnd // median index
	confidence := sum / float64(len(votes))
	if confidence > 1 {
		confidence = 1
	}

	return starts[mid], ends[mid], confidence
}
