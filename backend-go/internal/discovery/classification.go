package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

const classifyBatchSize = 10

// Classifier implements domain.BusinessClassifier using the AI router.
type Classifier struct {
	router   *ai.Router
	cacheTTL time.Duration
}

func NewClassifier(router *ai.Router) *Classifier {
	return &Classifier{
		router:   router,
		cacheTTL: 24 * time.Hour,
	}
}

// ClassifyBatch classifies businesses in batches of 10 via AI.
func (c *Classifier) ClassifyBatch(
	ctx context.Context,
	businesses []domain.Business,
	product domain.ProductContext,
) ([]domain.ClassificationResult, error) {
	if len(businesses) == 0 {
		return nil, nil
	}

	var allResults []domain.ClassificationResult

	for i := 0; i < len(businesses); i += classifyBatchSize {
		end := i + classifyBatchSize
		if end > len(businesses) {
			end = len(businesses)
		}
		batch := businesses[i:end]

		results, err := c.classifyOne(ctx, batch, product)
		if err != nil {
			slog.Error("classification batch failed",
				"batch_start", i,
				"batch_size", len(batch),
				"error", err,
			)
			continue
		}
		allResults = append(allResults, results...)
	}

	slog.Info("classification complete",
		"total_businesses", len(businesses),
		"total_classified", len(allResults),
	)
	return allResults, nil
}

func (c *Classifier) classifyOne(
	ctx context.Context,
	batch []domain.Business,
	product domain.ProductContext,
) ([]domain.ClassificationResult, error) {
	// Build prompt input
	promptBiz := make([]prompts.ClassifyBusiness, len(batch))
	idMap := make(map[string]uuid.UUID)

	for i, b := range batch {
		idStr := b.ID.String()
		idMap[idStr] = b.ID

		cb := prompts.ClassifyBusiness{
			ID:   idStr,
			Name: b.Name,
		}
		if b.Address != nil {
			cb.Address = *b.Address
		}
		if b.Website != nil {
			cb.Website = *b.Website
		}
		if b.Description != nil {
			cb.Description = *b.Description
		}
		if b.BusinessType != nil {
			cb.Categories = *b.BusinessType
		}
		promptBiz[i] = cb
	}

	p := prompts.BuildClassifyPrompt(promptBiz, product.Query, product.HSCode, product.CountryCode, product.City)

	raw, _, err := c.router.CompleteJSON(ctx, "classification", p.Prompt, p.System, c.cacheTTL)
	if err != nil {
		return nil, fmt.Errorf("classify AI call: %w", err)
	}

	var aiResults []classifyAIResponse
	if err := json.Unmarshal(raw, &aiResults); err != nil {
		return nil, fmt.Errorf("classify parse: %w (raw: %.200s)", err, string(raw))
	}

	results := make([]domain.ClassificationResult, 0, len(aiResults))
	for _, ar := range aiResults {
		bizID, ok := idMap[ar.ID]
		if !ok {
			continue
		}

		// Validate data_completeness is present (no-hallucination enforcement)
		if ar.DataCompleteness == 0 {
			slog.Warn("classification missing data_completeness", "business_id", ar.ID)
		}

		results = append(results, domain.ClassificationResult{
			BusinessID:       bizID,
			BusinessType:     ar.BusinessType,
			Industry:         ar.Industry,
			SubIndustry:      ar.SubIndustry,
			RelevanceScore:   ar.RelevanceScore,
			Confidence:       ar.Confidence,
			DataCompleteness: ar.DataCompleteness,
			Description:      ar.Description,
		})
	}

	return results, nil
}

type classifyAIResponse struct {
	ID               string  `json:"id"`
	BusinessType     string  `json:"business_type"`
	Industry         string  `json:"industry"`
	SubIndustry      string  `json:"sub_industry"`
	RelevanceScore   float64 `json:"relevance_score"`
	Confidence       float64 `json:"confidence"`
	DataCompleteness float64 `json:"data_completeness"`
	Description      string  `json:"description"`
}
