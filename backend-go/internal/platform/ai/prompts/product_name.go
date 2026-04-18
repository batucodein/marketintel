package prompts

import (
	"encoding/json"
	"fmt"
)

const productNameSystem = `You are a trade data analyst. Your job is to read a sample of product descriptions from customs shipment records and summarize them into a short, clean product name for a market card.

## RULES
- ONLY use words and concepts that appear in the provided product descriptions or HS code description.
- Do NOT invent materials, finishes, dimensions, colors, or use-cases that are not mentioned.
- Prefer the most common / dominant product type if descriptions are mixed.
- Keep the name 2-6 words. Title case.
- If descriptions are too noisy, contradictory, or empty, return the HS description verbatim and set confidence low.
- Also validate the origin-country distribution. If one origin has >= 85%% of shipments, mark is_single_origin=true.

## OUTPUT
Return JSON only:
{
  "product_name": "Marble Sinks and Vanity Tops",
  "confidence": 0.85,
  "data_completeness": 0.9,
  "origin_check": {
    "dominant_origin": "TR",
    "share_percent": 0.95,
    "is_single_origin": true
  }
}

confidence: how confident the name reflects the descriptions.
data_completeness: how much real text you used vs. fallback (0.0-1.0).`

const productNameTemplate = `Dominant HS code: %s
HS code description: %s

Dominant origin country: %s (share: %.2f)
All origin countries (sorted by frequency): %s

Top product descriptions from customs records (most frequent first):
%s

Summarize into a short product name following the rules.`

// ProductNameInput is everything the AI needs to derive a clean product name.
type ProductNameInput struct {
	DominantHSCode     string
	HSDescription      string
	DominantOrigin     string
	OriginShare        float64
	AllOriginCountries []string
	ProductDescs       []string
}

// ProductNameResult is the parsed AI response.
type ProductNameResult struct {
	ProductName      string  `json:"product_name"`
	Confidence       float64 `json:"confidence"`
	DataCompleteness float64 `json:"data_completeness"`
	OriginCheck      struct {
		DominantOrigin string  `json:"dominant_origin"`
		SharePercent   float64 `json:"share_percent"`
		IsSingleOrigin bool    `json:"is_single_origin"`
	} `json:"origin_check"`
}

// ProductNamePrompt holds the system and user prompts.
type ProductNamePrompt struct {
	System string
	Prompt string
}

func BuildProductNamePrompt(in ProductNameInput) ProductNamePrompt {
	hsDesc := in.HSDescription
	if hsDesc == "" {
		hsDesc = "unknown"
	}
	origin := in.DominantOrigin
	if origin == "" {
		origin = "unknown"
	}

	descsJSON, _ := json.MarshalIndent(in.ProductDescs, "", "  ")
	originsJSON, _ := json.Marshal(in.AllOriginCountries)

	return ProductNamePrompt{
		System: withPreamble(productNameSystem),
		Prompt: fmt.Sprintf(productNameTemplate,
			in.DominantHSCode,
			hsDesc,
			origin,
			in.OriginShare,
			string(originsJSON),
			string(descsJSON),
		),
	}
}
