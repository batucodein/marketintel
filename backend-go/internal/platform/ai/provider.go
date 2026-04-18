package ai

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AIResult is returned by every provider call.
type AIResult struct {
	Content      string  `json:"content"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Model        string  `json:"model"`
	CacheHit     bool    `json:"cache_hit"`
	LatencyMS    int     `json:"latency_ms"`
	RequestType  string  `json:"request_type"`
	Provider     string  `json:"provider"`
}

// Provider is the interface that all AI providers implement.
type Provider interface {
	Complete(ctx context.Context, req Request) (*AIResult, error)
	Name() string
}

// Request holds parameters for an AI completion call.
type Request struct {
	Prompt      string
	System      string
	Model       string
	Temperature float64
	MaxTokens   int
	CacheTTL    time.Duration
	RequestType string
	UserID      *uuid.UUID
	SearchID    *uuid.UUID
}
