// Package datasource — jina_reader.go is the Tier 2 scraping fallback.
//
// When our regex/HTTP scraper hits 403 or returns essentially empty
// content, we re-fetch the same URL through https://r.jina.ai which:
//   1. Renders JS server-side (Chromium)
//   2. Strips boilerplate (nav, ads, scripts)
//   3. Returns clean Markdown
//
// Free at low volume (no API key required for our usage). The Jina
// service is invoked with `GET https://r.jina.ai/<URL>` and identifies
// itself; we just send a User-Agent and a short timeout.
package datasource

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JinaReader fetches clean markdown for a URL via r.jina.ai.
type JinaReader struct {
	baseURL string
	client  *http.Client
}

// NewJinaReader configures the fallback. baseURL defaults to
// https://r.jina.ai when empty (the public free endpoint).
func NewJinaReader(baseURL string) *JinaReader {
	if baseURL == "" {
		baseURL = "https://r.jina.ai"
	}
	return &JinaReader{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetch returns the rendered markdown for the given URL, or an error if
// Jina itself rejects the request. The markdown is typically much
// cleaner than raw HTML and is the ideal input for LLM extraction.
func (j *JinaReader) Fetch(ctx context.Context, targetURL string) (string, error) {
	if targetURL == "" {
		return "", fmt.Errorf("jina: empty url")
	}
	// r.jina.ai expects the target URL appended after a slash, NOT
	// query-encoded. Slashes inside the target stay as-is.
	full := j.baseURL + "/" + strings.TrimPrefix(targetURL, "https://")
	full = strings.TrimPrefix(full, "http://")
	if !strings.HasPrefix(full, "https://r.jina.ai") {
		full = j.baseURL + "/" + targetURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MarketIntel/1.0 (+jina-fallback)")
	req.Header.Set("Accept", "text/markdown")
	req.Header.Set("X-Return-Format", "markdown")

	resp, err := j.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("jina: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("jina: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap
	if err != nil {
		return "", err
	}
	return string(body), nil
}
