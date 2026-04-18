package ai_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

func init() {
	// Load .env from project root
	godotenv.Load("../../../.env")
}

func TestAzureOpenAIComplete(t *testing.T) {
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	if apiKey == "" || endpoint == "" {
		t.Skip("AZURE_OPENAI_API_KEY or AZURE_OPENAI_ENDPOINT not set")
	}

	provider := ai.NewAzureOpenAI(apiKey, endpoint)
	ctx := context.Background()

	result, err := provider.Complete(ctx, ai.Request{
		Prompt:      "What is 2+2? Reply with just the number.",
		System:      "You are a calculator. Reply with just the number, nothing else.",
		Model:       "gpt-5.4-nano",
		Temperature: 0,
		MaxTokens:   10,
		RequestType: "test",
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	t.Logf("Response: %q", result.Content)
	t.Logf("Tokens: in=%d out=%d cost=$%.6f latency=%dms",
		result.InputTokens, result.OutputTokens, result.CostUSD, result.LatencyMS)

	if result.Provider != "azure_openai" {
		t.Errorf("expected provider azure_openai, got %s", result.Provider)
	}
}

func TestRouterCompleteJSON(t *testing.T) {
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	if apiKey == "" || endpoint == "" {
		t.Skip("AZURE_OPENAI_API_KEY or AZURE_OPENAI_ENDPOINT not set")
	}

	azureProvider := ai.NewAzureOpenAI(apiKey, endpoint)
	providers := map[string]ai.Provider{
		"azure_openai": azureProvider,
	}

	router := ai.NewRouter(providers, ai.DefaultTasks(), nil)
	ctx := context.Background()

	p := prompts.BuildProductNamePrompt(prompts.ProductNameInput{
		DominantHSCode: "6802.91",
		ProductDescs:   []string{"MARBLE SINKS", "MARBLE VANITY TOPS"},
	})
	raw, result, err := router.CompleteJSON(ctx, "product_name", p.Prompt, p.System, 0)
	if err != nil {
		t.Fatalf("CompleteJSON failed: %v", err)
	}

	t.Logf("Cost: $%.6f, Latency: %dms", result.CostUSD, result.LatencyMS)

	var queries []string
	if err := json.Unmarshal(raw, &queries); err != nil {
		t.Fatalf("Failed to parse queries: %v (raw: %s)", err, string(raw))
	}

	t.Logf("Generated %d queries:", len(queries))
	for i, q := range queries {
		t.Logf("  %d. %s", i+1, q)
	}

	if len(queries) < 4 {
		t.Errorf("expected at least 4 queries, got %d", len(queries))
	}
}

func TestCacheRoundTrip(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/0"
	}

	cache, err := ai.NewCache(redisURL)
	if err != nil {
		t.Fatalf("NewCache failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	key := "ai:test:roundtrip"

	result := &ai.AIResult{
		Content:      "test content",
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.001,
		Model:        "test-model",
		Provider:     "test",
	}

	if err := cache.Set(ctx, key, result, 10*time.Second); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected cached result, got nil")
	}
	if got.Content != "test content" {
		t.Errorf("expected 'test content', got %q", got.Content)
	}
	if !got.CacheHit {
		t.Error("expected CacheHit=true")
	}
}
