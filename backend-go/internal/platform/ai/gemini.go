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

var geminiPricing = map[string][2]float64{
	"gemini-2.5-pro":        {1.25, 10.00},
	"gemini-2.5-flash":      {0.15, 0.60},
	"gemini-2.5-flash-lite": {0.075, 0.30},
}

type Gemini struct {
	apiKey string
	client *http.Client
}

func NewGemini(apiKey string) *Gemini {
	return &Gemini{
		apiKey: apiKey,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (g *Gemini) Name() string { return "google" }

func (g *Gemini) Complete(ctx context.Context, req Request) (*AIResult, error) {
	return withRetry(ctx, g.Name(), func() (*AIResult, int, error) {
		return g.doRequest(ctx, req)
	})
}

func (g *Gemini) doRequest(ctx context.Context, req Request) (*AIResult, int, error) {
	// Build contents array
	contents := []map[string]any{
		{"role": "user", "parts": []map[string]string{{"text": req.Prompt}}},
	}

	body := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"temperature":     req.Temperature,
			"maxOutputTokens": req.MaxTokens,
		},
	}

	if req.System != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]string{{"text": req.System}},
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		req.Model, g.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("calling gemini: %w", err)
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("gemini error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var gResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return nil, 200, fmt.Errorf("parsing response: %w", err)
	}

	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return nil, 200, fmt.Errorf("no content in response")
	}

	in, out := gResp.UsageMetadata.PromptTokenCount, gResp.UsageMetadata.CandidatesTokenCount

	return &AIResult{
		Content:      gResp.Candidates[0].Content.Parts[0].Text,
		InputTokens:  in,
		OutputTokens: out,
		CostUSD:      calcCost(req.Model, in, out, geminiPricing),
		Model:        req.Model,
		LatencyMS:    int(latency.Milliseconds()),
		RequestType:  req.RequestType,
		Provider:     "google",
	}, 200, nil
}
