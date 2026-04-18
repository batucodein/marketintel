package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Search struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	UserID      uuid.UUID       `json:"user_id" db:"user_id"`
	SearchType  string          `json:"search_type" db:"search_type"`
	Query       json.RawMessage `json:"query" db:"query"`
	Status      string          `json:"status" db:"status"`
	ResultCount *int            `json:"result_count" db:"result_count"`
	CompletedAt *time.Time      `json:"completed_at" db:"completed_at"`
	TaskID      *string         `json:"task_id" db:"task_id"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

type TradeFlow struct {
	ID            uuid.UUID `json:"id" db:"id"`
	ReporterCode  string    `json:"reporter_code" db:"reporter_code"`
	PartnerCode   string    `json:"partner_code" db:"partner_code"`
	HSCode        string    `json:"hs_code" db:"hs_code"`
	Year          int       `json:"year" db:"year"`
	FlowType      string    `json:"flow_type" db:"flow_type"`
	TradeValueUSD *int      `json:"trade_value_usd" db:"trade_value_usd"`
	NetWeightKG   *int      `json:"net_weight_kg" db:"net_weight_kg"`
	Quantity      *int      `json:"quantity" db:"quantity"`
	FetchedAt     time.Time `json:"fetched_at" db:"fetched_at"`
}

type AIRequestLog struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	UserID       *uuid.UUID `json:"user_id" db:"user_id"`
	SearchID     *uuid.UUID `json:"search_id" db:"search_id"`
	RequestType  string     `json:"request_type" db:"request_type"`
	Provider     string     `json:"provider" db:"provider"`
	Model        string     `json:"model" db:"model"`
	InputTokens  *int       `json:"input_tokens" db:"input_tokens"`
	OutputTokens *int       `json:"output_tokens" db:"output_tokens"`
	CostUSD      *float64   `json:"cost_usd" db:"cost_usd"`
	LatencyMS    *int       `json:"latency_ms" db:"latency_ms"`
	CacheHit     bool       `json:"cache_hit" db:"cache_hit"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// AICostSummary is a summary of AI costs for a user.
type AICostSummary struct {
	TotalCostUSD    float64             `json:"total_cost_usd"`
	TotalInputTokens  int              `json:"total_input_tokens"`
	TotalOutputTokens int              `json:"total_output_tokens"`
	TotalRequests   int                 `json:"total_requests"`
	BySearch        []SearchCostSummary `json:"by_search"`
	ByDay           []DailyCostSummary  `json:"by_day"`
	ByModel         []ModelCostSummary  `json:"by_model"`
}

type SearchCostSummary struct {
	SearchID       uuid.UUID `json:"search_id"`
	SourceFileName string    `json:"source_file_name"`
	Status         string    `json:"status"`
	ResultCount    *int      `json:"result_count"`
	CostUSD        float64   `json:"cost_usd"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	Requests       int       `json:"requests"`
	CreatedAt      time.Time `json:"created_at"`
}

type DailyCostSummary struct {
	Date         string  `json:"date"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Requests     int     `json:"requests"`
}

type ModelCostSummary struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	CostUSD      float64 `json:"cost_usd"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Requests     int     `json:"requests"`
}
