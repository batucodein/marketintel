package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Business struct {
	ID                    uuid.UUID        `json:"id" db:"id"`
	Name                  string           `json:"name" db:"name"`
	Website               *string          `json:"website" db:"website"`
	GooglePlaceID         *string          `json:"google_place_id" db:"google_place_id"`
	LinkedinURL           *string          `json:"linkedin_url" db:"linkedin_url"`
	CountryCode           *string          `json:"country_code" db:"country_code"`
	City                  *string          `json:"city" db:"city"`
	Address               *string          `json:"address" db:"address"`
	Latitude              *decimal.Decimal `json:"latitude" db:"latitude"`
	Longitude             *decimal.Decimal `json:"longitude" db:"longitude"`
	Industry              *string          `json:"industry" db:"industry"`
	SubIndustry           *string          `json:"sub_industry" db:"sub_industry"`
	EmployeeCountRange    *string          `json:"employee_count_range" db:"employee_count_range"`
	EstimatedRevenueRange *string          `json:"estimated_revenue_range" db:"estimated_revenue_range"`
	BusinessType          *string          `json:"business_type" db:"business_type"`
	Description           *string          `json:"description" db:"description"`
	Phone                 *string          `json:"phone" db:"phone"`
	Email                 *string          `json:"email" db:"email"`
	SocialLinks           json.RawMessage  `json:"social_links" db:"social_links"`
	ShipmentData          json.RawMessage  `json:"shipment_data" db:"shipment_data"`
	WebsiteData           json.RawMessage  `json:"website_data" db:"website_data"`
	EmailVerified         *bool            `json:"email_verified" db:"email_verified"`
	Rating                *float64         `json:"rating" db:"rating"`
	RatingCount           *int             `json:"rating_count" db:"rating_count"`
	GoogleTypes           json.RawMessage  `json:"google_types" db:"google_types"`
	OpeningHours          json.RawMessage  `json:"opening_hours" db:"opening_hours"`
	// InputFieldPresence is the per-row map of which canonical fields the
	// source upload actually carried for THIS business — set by the
	// dynamic Excel importer. Format: {"consignee_email": true,
	// "total_value_usd": false, ...}. Used by existence-aware scoring.
	InputFieldPresence    json.RawMessage  `json:"input_field_presence" db:"input_field_presence"`
	DataSource            *string          `json:"data_source" db:"data_source"`
	EnrichmentStatus      string           `json:"enrichment_status" db:"enrichment_status"`
	LastEnrichedAt        *time.Time       `json:"last_enriched_at" db:"last_enriched_at"`
	DataConfidence        *decimal.Decimal `json:"data_confidence" db:"data_confidence"`
	CreatedAt             time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time        `json:"updated_at" db:"updated_at"`
}

type BusinessMarket struct {
	BusinessID    uuid.UUID        `json:"business_id" db:"business_id"`
	MarketID      uuid.UUID        `json:"market_id" db:"market_id"`
	RelevanceScore *decimal.Decimal `json:"relevance_score" db:"relevance_score"`
	DiscoveredVia *string          `json:"discovered_via" db:"discovered_via"`
}

// BusinessWithRelevance is returned when querying businesses for a market.
type BusinessWithRelevance struct {
	Business
	RelevanceScore *decimal.Decimal `json:"relevance_score" db:"relevance_score"`
	DiscoveredVia  *string          `json:"discovered_via" db:"discovered_via"`
	TrustTier      string           `json:"trust_tier"`

	// Google Places enrichment (duplicated for flat JSON response)
	Rating       *float64        `json:"rating" db:"rating"`
	RatingCount  *int            `json:"rating_count" db:"rating_count"`
	GoogleTypes  json.RawMessage `json:"google_types" db:"google_types"`
	ShipmentData json.RawMessage `json:"shipment_data" db:"shipment_data"`

	// Scoring fields (from lead_scores LEFT JOIN, nullable when not yet scored)
	OverallScore          *int                  `json:"overall_score,omitempty"`
	PurchaseLikelihood    *int                  `json:"purchase_likelihood,omitempty"`
	DealSizePotential     *int                  `json:"deal_size_potential,omitempty"`
	UrgencyScore          *int                  `json:"urgency_score,omitempty"`
	FitScore              *int                  `json:"fit_score,omitempty"`
	AccessibilityScore    *int                  `json:"accessibility_score,omitempty"`
	DimensionCompleteness DimensionCompleteness `json:"dimension_completeness,omitempty"`
	ScoringRationale      *string               `json:"scoring_rationale,omitempty"`
	Strengths             []string              `json:"strengths,omitempty"`
	Weaknesses            []string              `json:"weaknesses,omitempty"`
	MissingFields         []string              `json:"missing_fields,omitempty"`
	RecommendedApproach   *string               `json:"recommended_approach,omitempty"`
}
