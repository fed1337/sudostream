package provider

// TaskKind selects which of the three FI-1 maintenance actions to run (L14).
type TaskKind string

const (
	// TaskMetadata is the providers.metadata action.
	TaskMetadata TaskKind = "metadata"
	// TaskPoster is the providers.posters action.
	TaskPoster TaskKind = "poster"
	// TaskSubtitle is the providers.subtitles action.
	TaskSubtitle TaskKind = "subtitle"
)

// RunSummary is the per-run report surfaced in the maintenance run row (FI-1 § Maintenance
// actions). A run with no configured provider is a successful no-op with
// ProviderConfigured=false.
type RunSummary struct {
	ProviderConfigured bool
	ProviderKey        string
	Applied            int
	SkippedUser        int
	Unmatched          int
	SkippedUncertain   int
	Pruned             int
	Errors             int
}

// Map renders the summary as the JSON result stored on a maintenance run.
func (s RunSummary) Map() map[string]any {
	return map[string]any{
		"providerConfigured": s.ProviderConfigured,
		"providerKey":        s.ProviderKey,
		"applied":            s.Applied,
		"skippedUser":        s.SkippedUser,
		"unmatched":          s.Unmatched,
		"skippedUncertain":   s.SkippedUncertain,
		"pruned":             s.Pruned,
		"errors":             s.Errors,
	}
}

// countMatch records a non-applying match outcome and reports whether the caller may proceed.
func countMatch(summary *RunSummary, status MatchStatus) bool {
	switch status {
	case MatchOK:
		return true
	case MatchUncertain:
		summary.SkippedUncertain++
	case MatchNone:
		summary.Unmatched++
	default:
		summary.Unmatched++
	}

	return false
}
