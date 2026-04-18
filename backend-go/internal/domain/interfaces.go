package domain

import (
	"context"

	"github.com/google/uuid"
)

// Cross-module contracts. Feature modules depend on these interfaces,
// not on each other directly. This prevents circular dependencies
// and makes modules independently testable and extractable.

// BusinessClassifier classifies businesses by type and relevance.
type BusinessClassifier interface {
	ClassifyBatch(ctx context.Context, businesses []Business, product ProductContext) ([]ClassificationResult, error)
}

// LeadScorer scores businesses as potential leads.
type LeadScorer interface {
	ScoreBatch(ctx context.Context, businesses []Business, product ProductContext, marketCtx *MarketContext) ([]LeadScore, error)
}

// MarketDataFetcher fetches external market data for a country/product.
type MarketDataFetcher interface {
	FetchTradeData(ctx context.Context, hsCode, countryCode string) (*TradeData, error)
	FetchCountryInfo(ctx context.Context, countryCode string) (*CountryInfo, error)
	// FetchTopMarkets returns Turkey's top export destinations for an HS code, ranked by value.
	FetchTopMarkets(ctx context.Context, hsCode string, limit int) ([]TradePartner, error)
}

// ImporterFetcher fetches confirmed importers from trade data sources.
type ImporterFetcher interface {
	FetchImporters(ctx context.Context, hsCode, countryCode string, city *string) ([]ConfirmedImporter, error)
	FetchImporterCounts(ctx context.Context, hsCode string, countryCodes []string) ([]ImporterCount, error)
}

// ImporterCount holds the result of a lightweight count query per country.
type ImporterCount struct {
	CountryCode string `json:"country_code"`
	Count       int    `json:"count"`
}

// BusinessFinder discovers businesses via search APIs.
type BusinessFinder interface {
	SearchBusinesses(ctx context.Context, queries []string, lat, lng float64, radiusKM int, maxResults int) ([]Business, error)
	SearchByName(ctx context.Context, companyName, city, countryCode string) ([]Business, error)
	Geocode(ctx context.Context, city, countryCode string) (float64, float64, error)
}

// ProductContext holds the product info for classification/scoring.
type ProductContext struct {
	Query       string
	HSCode      string
	CountryCode string
	City        *string
}

// MarketContext holds market-level data for scoring.
type MarketContext struct {
	AnalysisSummary string
	TradeData       *TradeData
	CountryInfo     *CountryInfo
}

// TradeData holds aggregated trade flow info from UN Comtrade.
type TradeData struct {
	HSCode          string
	CountryCode     string
	ImportValueUSD  int64
	ExportValueUSD  int64
	ImportGrowthPct float64
	TopExporters    []TradePartner
	TopImporters    []TradePartner
	YearRange       string
}

type TradePartner struct {
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	ValueUSD    int64   `json:"value_usd"`
	SharePct    float64 `json:"share_pct"`
}

// CountryInfo holds basic country data from REST Countries / World Bank.
type CountryInfo struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Population    int64   `json:"population"`
	GDPPerCapita  float64 `json:"gdp_per_capita"`
	Region        string  `json:"region"`
	Currencies    string  `json:"currencies"`
	Languages     string  `json:"languages"`
}

// ConfirmedImporter is a company with real import history (from Tendata).
type ConfirmedImporter struct {
	CompanyName    string  `json:"company_name"`
	Country        string  `json:"country"`
	City           string  `json:"city"`
	Address        string  `json:"address"`
	TransactionCount int   `json:"transaction_count"`
	TotalValueUSD  float64 `json:"total_value_usd"`
	AvgOrderUSD    float64 `json:"avg_order_usd"`
	LastImportDate string  `json:"last_import_date"`
	TopSuppliers   string  `json:"top_suppliers"`
	ContactEmail   string  `json:"contact_email"`
	ContactPhone   string  `json:"contact_phone"`
	Website        string  `json:"website"`
}

// ClassificationResult is the output of classifying a single business.
type ClassificationResult struct {
	BusinessID       uuid.UUID `json:"id"`
	BusinessType     string    `json:"business_type"`
	Industry         string    `json:"industry"`
	SubIndustry      string    `json:"sub_industry"`
	RelevanceScore   float64   `json:"relevance_score"`
	Confidence       float64   `json:"confidence"`
	DataCompleteness float64   `json:"data_completeness"`
	Description      string    `json:"description"`
}
