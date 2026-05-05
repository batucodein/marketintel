package prompts

import (
	"encoding/json"
	"fmt"
)

const scoreSystem = `You are a B2B trade intelligence analyst helping an exporter find the best buyers to sell their products to.
You score potential buyers based on HARD DATA — not guesses from company names.

## SCORING PHILOSOPHY
The exporter needs leads they can ACTUALLY sell to. This means:
1. The buyer must import enough volume to justify an international deal
2. The exporter must be able to REACH the buyer (email, phone, website)
3. The buyer must be actively importing (not a one-time buyer from 2 years ago)
4. The product must fit their business

## DATA COMPLETENESS ENFORCEMENT
Your scores MUST reflect the data you actually have:
- If you only have a company name and no other data: data_completeness ≤ 0.2, and overall_score CANNOT exceed 45
- If you have name + basic profile but no shipment data: data_completeness ≤ 0.4, and overall_score CANNOT exceed 60
- If you have rich shipment data + contact info: data_completeness can be 0.7-1.0
- NEVER give a high score based on what the company name SOUNDS like. "Stone" in the name does NOT prove they're a large buyer.

## DIMENSION DEFINITIONS AND SCORING GUIDELINES

### 1. deal_size_potential (WEIGHT: 30%% — most important)
Based ONLY on import volume/value from shipment_context. International trade has high fixed costs (logistics, customs, negotiations), so tiny buyers are not viable leads.
- No shipment data: score 20-30 (unknown, assume small)
- <100 kg or <$1,000 total: score 10-25 (not viable for international trade)
- 100-1,000 kg or $1K-$50K: score 30-45 (small buyer)
- 1,000-10,000 kg or $50K-$500K: score 50-70 (medium buyer, worth pursuing)
- 10,000-50,000 kg or $500K-$2M: score 70-85 (large buyer)
- >50,000 kg or >$2M: score 85-95 (major buyer, high priority)

### 2. purchase_likelihood (WEIGHT: 25%%)
Based on import ACTIVITY — frequency and pattern. A company that imports regularly is more likely to buy than a one-time importer.
- No shipment data: score 20-35 (unknown)
- 1 transaction: score 25-40 (one-time buyer, might not repeat)
- 2-5 transactions: score 45-65 (recurring buyer)
- 6-15 transactions: score 65-80 (regular buyer)
- >15 transactions: score 80-90 (heavy buyer, very likely to purchase)
- BONUS +10 if "buys_from_home" (already imports from exporter's country — proven channel)

### 3. accessibility_score (WEIGHT: 25%%)
Can the exporter actually REACH this business? This is critical — an unreachable buyer is worthless.
IMPORTANT: Check the "contact_summary" field carefully — it contains ALL discovered contact channels (extra emails, phone numbers, WhatsApp, social media links). Do NOT say "no contact info" if these fields have data.
- Has verified email (email_verified="yes"): +35 points (confirmed deliverable)
- Has email (unverified): +25 points (direct outreach possible)
- Has phone number: +25 points (direct call possible)
- Has WhatsApp: +20 points (instant messaging, common in international trade)
- Has website: +15 points (can find contact form, verify business)
- Has LinkedIn: +10 points (professional outreach)
- Has contact form on website: +5 points
- Has physical address: +5 points (can send mail, verify location)
- Has other social media: +5 points
- No contact info at all: score 10-15 (nearly unusable lead)
- Cap at 95 max

### 4. fit_score (WEIGHT: 15%%)
How well does the product match their business? When customs data shows they import the exact HS code, fit is already PROVEN. This dimension differentiates business types.
- Imports exact HS code + relevant industry: score 80-95
- Imports related HS codes + adjacent industry: score 60-75
- Name/industry suggests fit but no import data: score 35-55
- Name only, no profile data: score 20-35 (too uncertain)
- Clearly irrelevant industry: score 5-15

### 5. urgency_score (WEIGHT: 5%%)
Based ONLY on recency of last import. Do NOT guess about seasonal demand or market conditions.
- Last shipment within 30 days: score 80-90 (actively buying now)
- Last shipment 1-3 months ago: score 60-75
- Last shipment 3-6 months ago: score 45-60
- Last shipment 6-12 months ago: score 30-45
- Last shipment >12 months ago: score 15-30 (may have stopped importing)
- No shipment date known: score 30 (neutral, don't guess)

## OVERALL SCORE CALCULATION
overall_score = deal_size_potential × 0.30 + purchase_likelihood × 0.25 + accessibility_score × 0.25 + fit_score × 0.15 + urgency_score × 0.05

THEN apply data_completeness cap:
- If data_completeness ≤ 0.2: cap overall_score at 45
- If data_completeness ≤ 0.4: cap overall_score at 60
- If data_completeness ≤ 0.6: cap overall_score at 75

## DIMENSION-LEVEL DATA QUALITY (CRITICAL)
For EACH lead, the input includes a "field_availability" object listing which canonical fields the source upload actually carried for this row (e.g. {"consignee_email": false, "total_value_usd": true, ...}). You MUST emit a "dimension_completeness" object alongside the sub-scores indicating, per dimension, how much of that dimension's score is grounded in real data vs. inferred:

- accessibility: depends on consignee_email, consignee_phone (and any contact_summary enrichment). If ALL of those are missing/false → dimension_completeness.accessibility ≤ 0.2 AND accessibility_score ≤ 30.
- deal_size: depends on quantity, weight_kg, total_value_usd. If ALL are absent → dimension_completeness.deal_size ≤ 0.2.
- fit: depends on hs_code, product_description, shipper_country. If all missing → dimension_completeness.fit ≤ 0.3.
- urgency: depends on shipment_date. If absent → dimension_completeness.urgency ≤ 0.3.
- purchase_likelihood: depends on transaction count derived from the input. If transaction_count = 1 with no other context → dimension_completeness.purchase_likelihood ≤ 0.4.

You may NOT compensate for missing fields by inferring from name or industry. Be conservative.

Always respond with valid JSON only, no markdown fences.`

const scoreTemplate = `Score the following businesses as potential buyers for "%s" (HS Code: %s) from an exporter.

## Market Context
%s

## Trade Context
%s

## Businesses to Score
%s

Score each business on these dimensions (0-100 each):
1. **deal_size_potential**: Import volume/value from shipment data (30%% weight)
2. **purchase_likelihood**: Import frequency and activity pattern (25%% weight)
3. **accessibility_score**: Available contact channels — email, website, phone (25%% weight)
4. **fit_score**: Product-business alignment from industry and import data (15%% weight)
5. **urgency_score**: Recency of last import only (5%% weight)

Also provide:
- **overall_score**: Weighted average using weights above, THEN capped by data_completeness
- **scoring_rationale**: 2-3 sentences explaining the score, referencing specific data points
- **strengths**: 1-3 specific strengths (cite data: "imports 15,000 kg annually", not "likely large buyer")
- **weaknesses**: 1-3 specific weaknesses (cite data: "no email or website found", not "may be hard to reach")
- **recommended_approach**: 1-2 sentence outreach strategy based on available contact channels
- **data_completeness**: 0.0-1.0 — proportion of scoring based on actual provided data
- **dimension_completeness**: object with keys deal_size, purchase_likelihood, accessibility, fit, urgency — each a 0.0-1.0 figure following the rules above
- **missing_fields**: array of canonical-field keys that were absent in the input AND not filled by enrichment for this lead (e.g. ["consignee_email", "consignee_phone"])

Respond as a JSON array:
[{"id": "<business_id>", "deal_size_potential": 0, "purchase_likelihood": 0, "accessibility_score": 0, "fit_score": 0, "urgency_score": 0, "overall_score": 0, "scoring_rationale": "...", "strengths": [...], "weaknesses": [...], "recommended_approach": "...", "data_completeness": 0.0, "dimension_completeness": {"deal_size": 0.0, "purchase_likelihood": 0.0, "accessibility": 0.0, "fit": 0.0, "urgency": 0.0}, "missing_fields": []}, ...]`

// ScoreBusiness represents a business to be scored.
type ScoreBusiness struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	CountryCode     string `json:"country_code,omitempty"`
	City            string `json:"city,omitempty"`
	Address         string `json:"address,omitempty"`
	Website         string `json:"website,omitempty"`
	Phone           string `json:"phone,omitempty"`
	Email           string `json:"email,omitempty"`
	EmailVerified   string `json:"email_verified,omitempty"`   // "yes", "no", or "" if not checked
	Industry        string `json:"industry,omitempty"`
	BusinessType    string `json:"business_type,omitempty"`
	Description     string `json:"description,omitempty"`
	ContactSummary  string `json:"contact_summary,omitempty"`  // all contact channels: extra emails, phones, whatsapp, social links
	ShipmentContext string `json:"shipment_context,omitempty"` // customs/trade history summary
	// FieldAvailability is the per-row presence map written by the
	// dynamic Excel importer (input_field_presence) merged with whatever
	// enrichment subsequently filled in. Drives the dimension_completeness
	// caps the AI is required to honour.
	FieldAvailability map[string]bool `json:"field_availability,omitempty"`
}

type ScorePrompt struct {
	System string
	Prompt string
}

func BuildScorePrompt(businesses []ScoreBusiness, productQuery, hsCode, marketContext, tradeContext string) ScorePrompt {
	if hsCode == "" {
		hsCode = "N/A"
	}
	if marketContext == "" {
		marketContext = "No market analysis available yet."
	}
	if tradeContext == "" {
		tradeContext = "No trade flow data available yet."
	}

	bizJSON, _ := json.MarshalIndent(businesses, "", "  ")

	return ScorePrompt{
		System: withPreamble(scoreSystem),
		Prompt: fmt.Sprintf(scoreTemplate,
			productQuery, hsCode,
			marketContext,
			tradeContext,
			string(bizJSON),
		),
	}
}
