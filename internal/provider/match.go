package provider

// MatchStatus is the outcome of an adapter's identification attempt (FI-1 L17).
// Only MatchOK applies a result; MatchNone and MatchUncertain log and skip.
type MatchStatus string

const (
	// MatchOK means the adapter found exactly one confident match.
	MatchOK MatchStatus = "ok"
	// MatchNone means the adapter found nothing for this title.
	MatchNone MatchStatus = "none"
	// MatchUncertain means the adapter found candidates but none confident enough to apply.
	MatchUncertain MatchStatus = "uncertain"
)
