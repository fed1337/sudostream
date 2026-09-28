// Package skipsegment detects skippable playback ranges (E-23 skip intro).
package skipsegment

import "time"

// Segment kind and detection source identifiers.
const (
	KindIntro = "intro"

	SourceChapter = "chapter"
	SourceAudio   = "audio"

	// EngineVersion bumps invalidate or force re-detect of stored audio hits.
	EngineVersion = 1
)

// Intro is an opening segment the player may offer to skip.
type Intro struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Source  string `json:"source,omitempty"`
}

// ChapterCue is one navigable chapter title and time range.
type ChapterCue struct {
	StartSeconds float64
	EndSeconds   float64
	Title        string
}

// StoredSegment is one persisted skip range for a media file.
type StoredSegment struct {
	LibraryID     string
	RelPath       string
	Kind          string
	StartMs       int64
	EndMs         int64
	Source        string
	Confidence    float64
	EngineVersion int
	ShowKey       string
	Season        int
	DetectedAt    time.Time
}

// ToIntro maps a stored row to the playback DTO.
func (s StoredSegment) ToIntro() *Intro {
	if s.EndMs <= s.StartMs {
		return nil
	}

	return &Intro{
		StartMs: s.StartMs,
		EndMs:   s.EndMs,
		Source:  s.Source,
	}
}

// EpisodeAudio is one episode's chromaprint within the intro scan window.
type EpisodeAudio struct {
	RelPath         string
	DurationSeconds float64
	Fingerprint     []uint32
	ScanWindowSec   float64
}
