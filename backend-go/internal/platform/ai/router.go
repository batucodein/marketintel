package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LogFunc is called after every AI completion to persist the request log.
type LogFunc func(ctx context.Context, result *AIResult, userID, searchID *uuid.UUID)

// TaskConfig defines which provider/model to use for a given task.
type TaskConfig struct {
	Provider    string // "azure_openai", "anthropic", "google"
	Model       string
	Temperature float64
	MaxTokens   int
}

// Router dispatches AI calls to the correct provider based on task type.
type Router struct {
	providers map[string]Provider
	tasks     map[string]TaskConfig
	cache     *Cache
	logFunc   LogFunc
}

// DefaultTasks returns the default task→provider mapping.
func DefaultTasks() map[string]TaskConfig {
	return map[string]TaskConfig{
		"classification":  {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.1, MaxTokens: 4096},
		"scoring":         {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.2, MaxTokens: 4096},
		"product_name":    {Provider: "azure_openai", Model: "gpt-5.4-nano", Temperature: 0.2, MaxTokens: 1024},
		"crossmatch":      {Provider: "azure_openai", Model: "gpt-5.4-nano", Temperature: 0.0, MaxTokens: 2048},
		"outreach_draft":     {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.4, MaxTokens: 2048},
		"outreach_reply":     {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.3, MaxTokens: 2048},
		"outreach_sentiment": {Provider: "azure_openai", Model: "gpt-5.4-nano", Temperature: 0.0, MaxTokens: 512},
		// Group draft assistant: conversational turn that answers questions about
		// drafts + proposes standing directives (the user confirms before apply).
		"assistant_chat": {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.3, MaxTokens: 1024},
		"excel_mapping":      {Provider: "azure_openai", Model: "gpt-5.4-nano", Temperature: 0.1, MaxTokens: 2048},
		"website_extract":    {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.1, MaxTokens: 2048},
		// Simulation lab: buyer-persona roleplay + transcript judge. Same tier
		// as drafting so persona realism matches what real recipients see.
		"simulation_buyer_persona": {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.6, MaxTokens: 1024},
		"simulation_judge":         {Provider: "azure_openai", Model: "gpt-5.4-mini", Temperature: 0.1, MaxTokens: 1024},
	}
}

func NewRouter(providers map[string]Provider, tasks map[string]TaskConfig, cache *Cache) *Router {
	return &Router{
		providers: providers,
		tasks:     tasks,
		cache:     cache,
	}
}

// SetLogFunc sets the callback for persisting AI request logs.
func (r *Router) SetLogFunc(fn LogFunc) {
	r.logFunc = fn
}

// Complete calls the appropriate provider for the given task.
// User/search IDs are read from context (set via WithUserID/WithSearchID).
func (r *Router) Complete(ctx context.Context, task, prompt, system string, cacheTTL time.Duration) (*AIResult, error) {
	tc, ok := r.tasks[task]
	if !ok {
		tc = r.tasks["research"] // fallback
	}

	provider, ok := r.providers[tc.Provider]
	if !ok {
		return nil, fmt.Errorf("provider %q not configured", tc.Provider)
	}

	req := Request{
		Prompt:      prompt,
		System:      system,
		Model:       tc.Model,
		Temperature: tc.Temperature,
		MaxTokens:   tc.MaxTokens,
		CacheTTL:    cacheTTL,
		RequestType: task,
	}

	// Check cache
	if r.cache != nil && cacheTTL > 0 {
		key := CacheKey(tc.Provider, tc.Model, system, prompt, tc.Temperature)
		cached, err := r.cache.Get(ctx, key)
		if err != nil {
			slog.Warn("cache get error", "error", err)
		}
		if cached != nil {
			slog.Debug("cache hit", "task", task, "model", tc.Model)
			return cached, nil
		}
	}

	result, err := provider.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%s completion failed: %w", task, err)
	}

	// Store in cache
	if r.cache != nil && cacheTTL > 0 {
		key := CacheKey(tc.Provider, tc.Model, system, prompt, tc.Temperature)
		if cErr := r.cache.Set(ctx, key, result, cacheTTL); cErr != nil {
			slog.Warn("cache set error", "error", cErr)
		}
	}

	slog.Info("ai call",
		"task", task,
		"provider", result.Provider,
		"model", result.Model,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"cost_usd", result.CostUSD,
		"latency_ms", result.LatencyMS,
	)

	// Persist to database via log func — reads userID/searchID from context
	if r.logFunc != nil {
		userID := UserIDFromContext(ctx)
		searchID := SearchIDFromContext(ctx)
		r.logFunc(ctx, result, userID, searchID)
	}

	return result, nil
}

// CompleteJSON calls Complete and parses the response as JSON.
func (r *Router) CompleteJSON(ctx context.Context, task, prompt, system string, cacheTTL time.Duration) (json.RawMessage, *AIResult, error) {
	if system == "" {
		system = "You are a data extraction assistant. Always respond with valid JSON only, no markdown fences."
	} else if !strings.Contains(system, "JSON") {
		system += "\nAlways respond with valid JSON only, no markdown fences."
	}

	result, err := r.Complete(ctx, task, prompt, system, cacheTTL)
	if err != nil {
		return nil, nil, err
	}

	text := strings.TrimSpace(result.Content)

	// Strip markdown fences if present
	if strings.HasPrefix(text, "```") {
		if idx := strings.Index(text, "\n"); idx != -1 {
			text = text[idx+1:]
		} else {
			text = text[3:]
		}
		if strings.HasSuffix(text, "```") {
			text = strings.TrimSuffix(text, "```")
		}
		text = strings.TrimSpace(text)
	}

	if !json.Valid([]byte(text)) {
		return nil, result, fmt.Errorf("invalid JSON in AI response: %.200s", text)
	}

	return json.RawMessage(text), result, nil
}
