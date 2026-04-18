package domain

import (
	"time"

	"github.com/google/uuid"
)

type LeadScore struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	BusinessID          uuid.UUID  `json:"business_id" db:"business_id"`
	MarketID            uuid.UUID  `json:"market_id" db:"market_id"`
	UserID              uuid.UUID  `json:"user_id" db:"user_id"`
	OverallScore        int        `json:"overall_score" db:"overall_score"`
	PurchaseLikelihood  *int       `json:"purchase_likelihood" db:"purchase_likelihood"`
	DealSizePotential   *int       `json:"deal_size_potential" db:"deal_size_potential"`
	UrgencyScore        *int       `json:"urgency_score" db:"urgency_score"`
	FitScore            *int       `json:"fit_score" db:"fit_score"`
	AccessibilityScore  *int       `json:"accessibility_score" db:"accessibility_score"`
	ScoringRationale    *string    `json:"scoring_rationale" db:"scoring_rationale"`
	Strengths           []string   `json:"strengths" db:"strengths"`
	Weaknesses          []string   `json:"weaknesses" db:"weaknesses"`
	RecommendedApproach *string    `json:"recommended_approach" db:"recommended_approach"`
	ModelVersion        string     `json:"model_version" db:"model_version"`
	PromptVersion       string     `json:"prompt_version" db:"prompt_version"`
	ScoredAt            time.Time  `json:"scored_at" db:"scored_at"`
	ExpiresAt           *time.Time `json:"expires_at" db:"expires_at"`
}
