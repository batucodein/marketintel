package prompts

import (
	"encoding/json"
	"fmt"
)

const classifySystem = `You are a B2B trade intelligence analyst specializing in international trade classification.
You classify businesses by their relevance to specific product categories (HS codes) and determine their role in the supply chain.
Always respond with valid JSON only, no markdown fences.`

const classifyTemplate = `Classify each business below for its relevance to the product "%s" (HS Code: %s).

For each business, determine:
1. **business_type**: One of: "potential_buyer", "competitor", "supplier", "distributor", "irrelevant"
2. **industry**: The business's primary industry (e.g. "construction", "interior design", "hospitality")
3. **sub_industry**: More specific classification (e.g. "residential renovation", "hotel chains")
4. **relevance_score**: 0.0 to 1.0 — how relevant this business is as a buyer/user of this product
5. **confidence**: 0.0 to 1.0 — how confident you are in the classification based on the data provided
6. **data_completeness**: 0.0 to 1.0 — how much real data you had to make this classification (1.0 = rich data, 0.1 = only a business name)
7. **description**: A concise 1-2 sentence description explaining what this business does and HOW they would use or need the seller's product ("%s"). Focus on the buyer-seller relationship.

Context:
- Seller is an exporter selling globally
- Target market: %s%s
- We want to find businesses that IMPORT or USE this product category
- The description should help the seller understand WHY this business is a potential buyer

Businesses to classify:
%s

Respond with a JSON array where each object has:
{"id": "<business_id>", "business_type": "...", "industry": "...", "sub_industry": "...", "relevance_score": 0.0, "confidence": 0.0, "data_completeness": 0.0, "description": "..."}`

// ClassifyBusiness represents a business to be classified.
type ClassifyBusiness struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address,omitempty"`
	Website     string `json:"website,omitempty"`
	Description string `json:"description,omitempty"`
	Categories  string `json:"categories,omitempty"`
}

type ClassifyPrompt struct {
	System string
	Prompt string
}

func BuildClassifyPrompt(businesses []ClassifyBusiness, productQuery, hsCode, countryCode string, city *string) ClassifyPrompt {
	if hsCode == "" {
		hsCode = "N/A"
	}

	cityContext := ""
	if city != nil && *city != "" {
		cityContext = fmt.Sprintf(", City: %s", *city)
	}

	bizJSON, _ := json.MarshalIndent(businesses, "", "  ")

	return ClassifyPrompt{
		System: withPreamble(classifySystem),
		Prompt: fmt.Sprintf(classifyTemplate,
			productQuery, hsCode,
			productQuery,
			countryCode, cityContext,
			string(bizJSON),
		),
	}
}
