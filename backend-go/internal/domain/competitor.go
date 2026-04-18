package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Competitor struct {
	ID                   uuid.UUID        `json:"id" db:"id"`
	MarketID             uuid.UUID        `json:"market_id" db:"market_id"`
	Name                 string           `json:"name" db:"name"`
	Website              *string          `json:"website" db:"website"`
	CountryCode          *string          `json:"country_code" db:"country_code"`
	CompetitorType       *string          `json:"competitor_type" db:"competitor_type"`
	EstimatedMarketShare *decimal.Decimal `json:"estimated_market_share" db:"estimated_market_share"`
	Strengths            []string         `json:"strengths" db:"strengths"`
	Weaknesses           []string         `json:"weaknesses" db:"weaknesses"`
	DataSource           *string          `json:"data_source" db:"data_source"`
	LastAnalyzedAt       *time.Time       `json:"last_analyzed_at" db:"last_analyzed_at"`
	CreatedAt            time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at" db:"updated_at"`
}

type PriceSnapshot struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	CompetitorID      uuid.UUID       `json:"competitor_id" db:"competitor_id"`
	ProductName       *string         `json:"product_name" db:"product_name"`
	ProductCategoryID *uuid.UUID      `json:"product_category_id" db:"product_category_id"`
	Price             decimal.Decimal `json:"price" db:"price"`
	Currency          string          `json:"currency" db:"currency"`
	PriceUnit         *string         `json:"price_unit" db:"price_unit"`
	SourceURL         *string         `json:"source_url" db:"source_url"`
	CapturedAt        time.Time       `json:"captured_at" db:"captured_at"`
}
