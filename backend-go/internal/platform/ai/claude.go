package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var claudePricing = map[string][2]float64{
	"claude-sonnet-4-6":          {3.00, 15.00},
	"claude-haiku-4-5-20251001":  {0.80, 4.00},
	"claude-opus-4-6":            {15.00, 75.00},
}

type Claude struct {
	apiKey string
	client *http.Client
}

func NewClaude(apiKey string) *Claude {
	return &Claude{
		apiKey: apiKey,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Claude) Name() string { return "anthropic" }

func (c *Claude) Complete(ctx context.Context, req Request) (*AIResult, error) {
	return withRetry(ctx, c.Name(), func() (*AIResult, int, error) {
		return c.doRequest(ctx, req)
	})
}

func (c *Claude) doRequest(ctx context.Context, req Request) (*AIResult, int, error) {
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   []map[string]string{{"role": "user", "content": req.Prompt}},
	}
	if req.System != "" {
		body["system"] = req.System
	}
	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	start := time.Now()
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("calling anthropic: %w", err)
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("anthropic error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var cResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &cResp); err != nil {
		return nil, 200, fmt.Errorf("parsing response: %w", err)
	}

	if len(cResp.Content) == 0 {
		return nil, 200, fmt.Errorf("no content in response")
	}

	in, out := cResp.Usage.InputTokens, cResp.Usage.OutputTokens

	return &AIResult{
		Content:      cResp.Content[0].Text,
		InputTokens:  in,
		OutputTokens: out,
		CostUSD:      calcCost(req.Model, in, out, claudePricing),
		Model:        req.Model,
		LatencyMS:    int(latency.Milliseconds()),
		RequestType:  req.RequestType,
		Provider:     "anthropic",
	}, 200, nil
}
