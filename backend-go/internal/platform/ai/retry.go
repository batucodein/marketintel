package ai

import (
	"context"
	"log/slog"
	"math"
	"time"
)

const (
	maxRetries    = 5
	baseDelay     = 5 * time.Second
	maxDelay      = 120 * time.Second
)

// retryable returns true for HTTP status codes that warrant a retry.
func retryable(statusCode int) bool {
	return statusCode == 429 || statusCode == 500 || statusCode == 502 || statusCode == 503
}

// withRetry wraps a provider call with exponential backoff retry.
func withRetry(ctx context.Context, name string, fn func() (*AIResult, int, error)) (*AIResult, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, statusCode, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !retryable(statusCode) {
			return nil, err
		}

		delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		if delay > maxDelay {
			delay = maxDelay
		}

		slog.Warn("retrying AI call",
			"provider", name,
			"attempt", attempt+1,
			"status", statusCode,
			"delay", delay,
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}
