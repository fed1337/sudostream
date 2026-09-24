// Package skipsegment detects skippable playback ranges (E-23 skip intro).
package skipsegment

// SourceChapter marks a segment derived from container chapter metadata.
const SourceChapter = "chapter"

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
