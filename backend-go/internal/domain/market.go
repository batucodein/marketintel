package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Market struct {
	ID                  uuid.UUID        `json:"id" db:"id"`
	Name                string           `json:"name" db:"name"`
	CountryCode         string           `json:"country_code" db:"country_code"`
	Region              *string          `json:"region" db:"region"`
	City                *string          `json:"city" db:"city"`
	Latitude            *decimal.Decimal `json:"latitude" db:"latitude"`
	Longitude           *decimal.Decimal `json:"longitude" db:"longitude"`
	RadiusKM            *int             `json:"radius_km" db:"radius_km"`
	ProductCategoryID   *uuid.UUID       `json:"product_category_id" db:"product_category_id"`
	EstimatedMarketSize *decimal.Decimal `json:"estimated_market_size" db:"estimated_market_size"`
	SaturationScore     *decimal.Decimal `json:"saturation_score" db:"saturation_score"`
	DemandScore         *decimal.Decimal `json:"demand_score" db:"demand_score"`
	LastAnalyzedAt      *time.Time       `json:"last_analyzed_at" db:"last_analyzed_at"`

	// Derived from Excel upload (migration 000007)
	DerivedProductName *string    `json:"derived_product_name" db:"derived_product_name"`
	DominantHSCode     *string    `json:"dominant_hs_code" db:"dominant_hs_code"`
	AllHSCodes         []string   `json:"all_hs_codes" db:"all_hs_codes"`
	OriginCountry      *string    `json:"origin_country" db:"origin_country"`
	OriginCountries    []string   `json:"origin_countries" db:"origin_countries"`
	OriginShare        *float64   `json:"origin_share" db:"origin_share"`
	ShipmentFromDate   *time.Time `json:"shipment_from_date" db:"shipment_from_date"`
	ShipmentToDate     *time.Time `json:"shipment_to_date" db:"shipment_to_date"`
	ImporterCount      *int       `json:"importer_count" db:"importer_count"`
	ShipmentCount      *int       `json:"shipment_count" db:"shipment_count"`
	SourceFileName     *string    `json:"source_file_name" db:"source_file_name"`
	UploadedAt         *time.Time `json:"uploaded_at" db:"uploaded_at"`

	// SenderProfileID is the brand this market sends as. One fixed brand per
	// market; Email Groups created from the market inherit it. Nil until the
	// user assigns one (groups can't be created from a market with no brand).
	SenderProfileID *uuid.UUID `json:"sender_profile_id" db:"sender_profile_id"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

type MarketAnalysis struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	MarketID     uuid.UUID       `json:"market_id" db:"market_id"`
	AnalysisType string          `json:"analysis_type" db:"analysis_type"`
	Content      json.RawMessage `json:"content" db:"content"`
	Summary      *string         `json:"summary" db:"summary"`
	GeneratedBy  string          `json:"generated_by" db:"generated_by"`
	ModelVersion *string         `json:"model_version" db:"model_version"`
	GeneratedAt  time.Time       `json:"generated_at" db:"generated_at"`
	ValidUntil   *time.Time      `json:"valid_until" db:"valid_until"`
}

type MarketRanking struct {
	ID                uuid.UUID        `json:"id" db:"id"`
	UserID            uuid.UUID        `json:"user_id" db:"user_id"`
	ProductCategoryID uuid.UUID        `json:"product_category_id" db:"product_category_id"`
	CountryCode       string           `json:"country_code" db:"country_code"`
	TOPSISScore       *decimal.Decimal `json:"topsis_score" db:"topsis_score"`
	SAWScore          *decimal.Decimal `json:"saw_score" db:"saw_score"`
	OpportunityScore  *decimal.Decimal `json:"opportunity_score" db:"opportunity_score"`
	ReliabilityScore  *decimal.Decimal `json:"reliability_score" db:"reliability_score"`
	AccessibilityScore *decimal.Decimal `json:"accessibility_score" db:"accessibility_score"`
	CriteriaData      json.RawMessage  `json:"criteria_data" db:"criteria_data"`
	AlgorithmUsed     *string          `json:"algorithm_used" db:"algorithm_used"`
	RankedAt          time.Time        `json:"ranked_at" db:"ranked_at"`
}
