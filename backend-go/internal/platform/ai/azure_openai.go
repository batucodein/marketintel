package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var azurePricing = map[string][2]float64{
	"gpt-5.4-mini": {0.40, 1.60},
	"gpt-5.4-nano": {0.10, 0.40},
}

type AzureOpenAI struct {
	apiKey   string
	endpoint string
	client   *http.Client
}

func NewAzureOpenAI(apiKey, endpoint string) *AzureOpenAI {
	return &AzureOpenAI{
		apiKey:   apiKey,
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *AzureOpenAI) Name() string { return "azure_openai" }

func (a *AzureOpenAI) Complete(ctx context.Context, req Request) (*AIResult, error) {
	return withRetry(ctx, a.Name(), func() (*AIResult, int, error) {
		return a.doRequest(ctx, req)
	})
}

func (a *AzureOpenAI) doRequest(ctx context.Context, req Request) (*AIResult, int, error) {
	messages := make([]map[string]string, 0, 2)
	if req.System != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.System})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Prompt})

	body := map[string]any{
		"model":                 req.Model,
		"messages":              messages,
		"temperature":           req.Temperature,
		"max_completion_tokens": req.MaxTokens,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2025-01-01-preview",
		a.endpoint, req.Model)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", a.apiKey)

	start := time.Now()
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("calling azure openai: %w", err)
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("azure openai error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var azResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &azResp); err != nil {
		return nil, 200, fmt.Errorf("parsing response: %w", err)
	}

	if len(azResp.Choices) == 0 {
		return nil, 200, fmt.Errorf("no choices in response")
	}

	in, out := azResp.Usage.PromptTokens, azResp.Usage.CompletionTokens

	return &AIResult{
		Content:      azResp.Choices[0].Message.Content,
		InputTokens:  in,
		OutputTokens: out,
		CostUSD:      calcCost(req.Model, in, out, azurePricing),
		Model:        req.Model,
		LatencyMS:    int(latency.Milliseconds()),
		RequestType:  req.RequestType,
		Provider:     "azure_openai",
	}, 200, nil
}

func calcCost(model string, input, output int, pricing map[string][2]float64) float64 {
	p, ok := pricing[model]
	if !ok {
		for _, v := range pricing {
			p = v
			break
		}
	}
	return (float64(input)*p[0] + float64(output)*p[1]) / 1_000_000
}
