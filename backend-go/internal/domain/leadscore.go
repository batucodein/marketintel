package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DimensionCompleteness is the per-dimension data-quality map persisted
// alongside a LeadScore. Values are 0.0-1.0 and reflect, for THIS lead,
// how much of the dimension's sub-score is grounded in real input vs.
// inferred. A row with no email/phone gets accessibility=0.0 here even
// if the AI reported a higher figure (the server clamps it).
//
// Keys are the dimension identifiers from internal/discovery/canonical.go
// (deal_size, purchase_likelihood, accessibility, fit, urgency).
type DimensionCompleteness map[string]float64

type LeadScore struct {
	ID                    uuid.UUID             `json:"id" db:"id"`
	BusinessID            uuid.UUID             `json:"business_id" db:"business_id"`
	MarketID              uuid.UUID             `json:"market_id" db:"market_id"`
	UserID                uuid.UUID             `json:"user_id" db:"user_id"`
	OverallScore          int                   `json:"overall_score" db:"overall_score"`
	PurchaseLikelihood    *int                  `json:"purchase_likelihood" db:"purchase_likelihood"`
	DealSizePotential     *int                  `json:"deal_size_potential" db:"deal_size_potential"`
	UrgencyScore          *int                  `json:"urgency_score" db:"urgency_score"`
	FitScore              *int                  `json:"fit_score" db:"fit_score"`
	AccessibilityScore    *int                  `json:"accessibility_score" db:"accessibility_score"`
	DimensionCompleteness DimensionCompleteness `json:"dimension_completeness" db:"dimension_completeness"`
	ScoringRationale      *string               `json:"scoring_rationale" db:"scoring_rationale"`
	Strengths             []string              `json:"strengths" db:"strengths"`
	Weaknesses            []string              `json:"weaknesses" db:"weaknesses"`
	MissingFields         []string              `json:"missing_fields,omitempty"`
	RecommendedApproach   *string               `json:"recommended_approach" db:"recommended_approach"`
	ModelVersion          string                `json:"model_version" db:"model_version"`
	PromptVersion         string                `json:"prompt_version" db:"prompt_version"`
	ScoredAt              time.Time             `json:"scored_at" db:"scored_at"`
	ExpiresAt             *time.Time            `json:"expires_at" db:"expires_at"`
}

// MarshalDimensionCompleteness returns the JSON bytes for DB persistence.
// Empty/nil maps still encode as `{}` so the JSONB column never sees NULL
// (matching the migration default).
func MarshalDimensionCompleteness(d DimensionCompleteness) ([]byte, error) {
	if d == nil {
		d = DimensionCompleteness{}
	}
	return json.Marshal(d)
}
