package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client *redis.Client
}

func NewCache(redisURL string) (*Cache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis url: %w", err)
	}
	return &Cache{client: redis.NewClient(opts)}, nil
}

func (c *Cache) Get(ctx context.Context, key string) (*AIResult, error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result AIResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	result.CacheHit = true
	return &result, nil
}

func (c *Cache) Set(ctx context.Context, key string, result *AIResult, ttl time.Duration) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *Cache) Close() error {
	return c.client.Close()
}

// CacheKey generates a deterministic cache key from the request parameters.
func CacheKey(provider, model, system, prompt string, temperature float64) string {
	payload := fmt.Sprintf("%s|%s|%s|%s|%.2f", provider, model, system, prompt, temperature)
	hash := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("ai:%s:%x", provider, hash)
}
