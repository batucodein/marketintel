package domain

// Sentiment is modelled as a continuous score in [-1.0, 1.0] (the source of
// truth, produced by the classifier) plus a derived 5-level label. The label is
// always computed from the score via LabelForScore — it is never re-classified,
// so re-thresholding is a code change + a single UPDATE, not a new AI call.

// 5-level sentiment labels (derived from the score).
const (
	SentimentVeryNegative = "very_negative"
	SentimentVeryPositive = "very_positive"
	// SentimentNegative / SentimentNeutral / SentimentPositive are defined in
	// outreach.go and reused here as the middle three levels.
)

// SentimentLevels is the ordered set of labels, coldest → warmest. Used by the
// UI facet order and to validate inbound label values.
var SentimentLevels = []string{
	SentimentVeryNegative,
	SentimentNegative,
	SentimentNeutral,
	SentimentPositive,
	SentimentVeryPositive,
}

// LabelForScore maps a score in [-1, 1] to one of the five labels. Thresholds
// live here and nowhere else — change them and re-derive labels with one UPDATE.
func LabelForScore(score float64) string {
	switch {
	case score <= -0.6:
		return SentimentVeryNegative
	case score < -0.2:
		return SentimentNegative
	case score <= 0.2:
		return SentimentNeutral
	case score < 0.6:
		return SentimentPositive
	default:
		return SentimentVeryPositive
	}
}

// ScoreForLegacy maps the old 3-value sentiment string to a representative
// score, so historical rows (classified before scoring existed) and the
// back-compat parse path still produce a sane number.
func ScoreForLegacy(legacy string) float64 {
	switch legacy {
	case SentimentPositive:
		return 0.5
	case SentimentNegative:
		return -0.5
	default:
		return 0.0
	}
}

// LegacyForScore collapses a score back to the 3-value bucket that the existing
// reply branch (ResolveReplyBranch) still keys off. The boundaries are strict
// (>0.2 / <-0.2) so they agree EXACTLY with LabelForScore's neutral band
// [-0.2, 0.2] — the UI label and the engine's branch must never tell two
// different stories about the same reply.
func LegacyForScore(score float64) string {
	switch {
	case score > 0.2:
		return SentimentPositive
	case score < -0.2:
		return SentimentNegative
	default:
		return SentimentNeutral
	}
}
