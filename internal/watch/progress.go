package watch

const (
	// MinProgressSeconds ignores accidental starts before creating a Continue row.
	MinProgressSeconds = 10
	// CompleteRatio marks a title watched when position/duration reaches this share.
	CompleteRatio = 0.90
	// CompleteRemainingSeconds marks a title watched when this many seconds remain.
	CompleteRemainingSeconds = 30
)

// Patch is a partial watch-state update. Nil fields are left unchanged.
type Patch struct {
	Watched         *bool
	PositionSeconds *float64
	DurationSeconds *float64
}

// IsComplete reports whether playback should be treated as finished.
func IsComplete(position, duration float64) bool {
	if duration <= 0 || position < 0 {
		return false
	}
	if position/duration >= CompleteRatio {
		return true
	}

	return duration-position <= CompleteRemainingSeconds
}

// ShouldSaveProgress reports whether position is past the accidental-start floor.
func ShouldSaveProgress(position float64) bool {
	return position >= MinProgressSeconds
}
