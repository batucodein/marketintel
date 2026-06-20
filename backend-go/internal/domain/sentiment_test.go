package domain

import "testing"

func TestLabelForScore(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{-1.0, SentimentVeryNegative},
		{-0.6, SentimentVeryNegative}, // boundary: <= -0.6
		{-0.59, SentimentNegative},
		{-0.2, SentimentNeutral}, // boundary: -0.2 is neutral (< -0.2 is negative)
		{-0.21, SentimentNegative},
		{0.0, SentimentNeutral},
		{0.2, SentimentNeutral},  // boundary: <= 0.2 neutral
		{0.21, SentimentPositive},
		{0.59, SentimentPositive},
		{0.6, SentimentVeryPositive}, // boundary: >= 0.6
		{1.0, SentimentVeryPositive},
	}
	for _, c := range cases {
		if got := LabelForScore(c.score); got != c.want {
			t.Errorf("LabelForScore(%v) = %s, want %s", c.score, got, c.want)
		}
	}
}

func TestLegacyForScore(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{-1.0, SentimentNegative},
		{-0.21, SentimentNegative},
		{-0.2, SentimentNeutral}, // boundary must agree with LabelForScore
		{0.0, SentimentNeutral},
		{0.2, SentimentNeutral}, // boundary must agree with LabelForScore
		{0.21, SentimentPositive},
		{1.0, SentimentPositive},
	}
	for _, c := range cases {
		if got := LegacyForScore(c.score); got != c.want {
			t.Errorf("LegacyForScore(%v) = %s, want %s", c.score, got, c.want)
		}
	}
}

// TestLabelLegacyAgreement: the 5-level label and the 3-value branch bucket
// must collapse consistently for every score — a reply can never look Neutral
// in the UI while the engine branches it negative/positive.
func TestLabelLegacyAgreement(t *testing.T) {
	collapse := func(label string) string {
		switch label {
		case SentimentVeryNegative, SentimentNegative:
			return SentimentNegative
		case SentimentVeryPositive, SentimentPositive:
			return SentimentPositive
		default:
			return SentimentNeutral
		}
	}
	for s := -1.0; s <= 1.0; s += 0.01 {
		if collapse(LabelForScore(s)) != LegacyForScore(s) {
			t.Fatalf("disagreement at score %.2f: label=%s legacy=%s", s, LabelForScore(s), LegacyForScore(s))
		}
	}
}
