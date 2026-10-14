package skipsegment

import (
	"math/rand/v2"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestLongestMatch_FindsSharedBlock(t *testing.T) {
	t.Parallel()

	allure.Test(t, "longest match finds shared fingerprint block", func(a *allure.Context) {
		t := a.T()

		rng := rand.New(rand.NewPCG(1, 2))
		theme := randomFingerprint(rng, 100)
		leftLead := randomFingerprint(rng, 40)
		leftTail := randomFingerprint(rng, 20)
		rightLead := randomFingerprint(rng, 55)
		rightTail := randomFingerprint(rng, 10)

		left := append(append(leftLead, theme...), leftTail...)
		right := append(append(rightLead, theme...), rightTail...)

		match := LongestMatch(left, right, maxStartSkewSec)
		if match == nil {
			t.Fatal("expected match")
		}
		if match.DurationSec < float64(minIntroSec) {
			t.Fatalf("duration too short: %.2f", match.DurationSec)
		}
		if match.StartA < 35 || match.StartA > 45 {
			t.Fatalf("startA=%d want ~40", match.StartA)
		}
	})
}

func TestLongestMatch_NoMatchOnNoise(t *testing.T) {
	t.Parallel()

	allure.Test(t, "longest match returns nil for unrelated fingerprints", func(a *allure.Context) {
		t := a.T()

		rng := rand.New(rand.NewPCG(3, 4))
		left := randomFingerprint(rng, 200)
		right := randomFingerprint(rng, 200)
		if match := LongestMatch(left, right, maxStartSkewSec); match != nil {
			t.Fatalf("unexpected match: %+v", match)
		}
	})
}

func TestConsensusIntros_RequiresAgreement(t *testing.T) {
	t.Parallel()

	allure.Test(t, "consensus intros — shared theme across three episodes", func(a *allure.Context) {
		t := a.T()

		rng := rand.New(rand.NewPCG(5, 6))
		theme := randomFingerprint(rng, 120)
		mk := func(path string, lead int) EpisodeAudio {
			fp := append(randomFingerprint(rng, lead), theme...)

			return EpisodeAudio{
				RelPath:       path,
				Fingerprint:   fp,
				ScanWindowSec: 600,
			}
		}
		hits := ConsensusIntros([]EpisodeAudio{
			mk("a.mkv", 20),
			mk("b.mkv", 30),
			mk("c.mkv", 25),
		})
		if len(hits) != 3 {
			t.Fatalf("hits=%d want 3: %+v", len(hits), hits)
		}
	})

	allure.Test(t, "consensus intros — abstains with single episode", func(a *allure.Context) {
		t := a.T()

		if hits := ConsensusIntros([]EpisodeAudio{{RelPath: "only.mkv", Fingerprint: []uint32{1, 2, 3}}}); hits != nil {
			t.Fatalf("expected nil, got %+v", hits)
		}
	})
}

func TestResolveIntro_PrefersChapter(t *testing.T) {
	t.Parallel()

	allure.Test(t, "resolve intro prefers chapter over stored audio", func(a *allure.Context) {
		t := a.T()

		chapter := &Intro{StartMs: 10_000, EndMs: 70_000, Source: SourceChapter}
		audio := &Intro{StartMs: 5_000, EndMs: 80_000, Source: SourceAudio}
		got := ResolveIntro(chapter, audio)
		if got == nil || got.Source != SourceChapter || got.StartMs != 10_000 {
			t.Fatalf("got %+v", got)
		}
	})

	allure.Test(t, "resolve intro falls back to stored audio", func(a *allure.Context) {
		t := a.T()

		audio := &Intro{StartMs: 5_000, EndMs: 80_000, Source: SourceAudio}
		got := ResolveIntro(nil, audio)
		if got == nil || got.Source != SourceAudio {
			t.Fatalf("got %+v", got)
		}
	})
}

func randomFingerprint(rng *rand.Rand, n int) []uint32 {
	out := make([]uint32, n)
	for index := range out {
		out[index] = rng.Uint32()
	}

	return out
}
