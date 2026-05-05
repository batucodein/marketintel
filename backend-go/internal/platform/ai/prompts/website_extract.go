package prompts

import (
	"encoding/json"
	"fmt"
)

// websiteExtractSystem is the system prompt for the LLM-based contact /
// product extraction step that replaces brittle regex passes over raw
// HTML / markdown. NO HALLUCINATION — every value must trace back to a
// literal substring in the input.
const websiteExtractSystem = `You extract structured contact and product information from a company's web page (markdown or plain text).

## STRICT RULES — NO HALLUCINATION
- Emails, phones, WhatsApp numbers, social links: copy them VERBATIM from the input. If they aren't in the input, return empty arrays.
- Do NOT infer emails from the domain (e.g. don't return "info@<domain>" unless that exact string is in the page).
- The "description" must be a 1-2 sentence summary built ONLY from text in the input. If the page has no descriptive content, return "".
- Products: only extract items the page actually lists. Don't generalise from the company name.
- Output "data_completeness" as the proportion of useful fields (emails, phones, description, products) that you could fill from the input — 1.0 if all, 0.0 if none.
- For the contact_form_url field: emit a URL only if the input contains a literal "/contact" or similar form-page link.

## OUTPUT (JSON only, no markdown fences)
{
  "emails": ["sales@example.com"],
  "phones": ["+1 555 123 4567"],
  "whatsapp": "+1 555 123 4567",
  "social_links": {"linkedin": "https://...", "instagram": "https://..."},
  "products": ["Calacatta marble slabs", "Travertine tiles"],
  "services": ["Stone cutting", "Container shipping"],
  "description": "Family-run marble exporter from Afyon, Türkiye, supplying US contractors since 2010.",
  "hq_address": "Org. San. Bölgesi 5. Cad No:12, Afyon, Türkiye",
  "contact_form_url": "https://example.com/contact",
  "data_completeness": 0.0-1.0
}`

// WebsiteExtractInput is the typed input for BuildWebsiteExtractPrompt.
type WebsiteExtractInput struct {
	BusinessName string
	Country      string
	URL          string
	// Body is the scraped markdown/text. Capped at ~12 KB by the caller.
	Body string
}

// WebsiteExtractResult is the structured output. Empty slices/strings
// when the AI couldn't ground a value in the input — never null/missing.
type WebsiteExtractResult struct {
	Emails           []string          `json:"emails"`
	Phones           []string          `json:"phones"`
	WhatsApp         string            `json:"whatsapp"`
	SocialLinks      map[string]string `json:"social_links"`
	Products         []string          `json:"products"`
	Services         []string          `json:"services"`
	Description      string            `json:"description"`
	HQAddress        string            `json:"hq_address"`
	ContactFormURL   string            `json:"contact_form_url"`
	DataCompleteness float64           `json:"data_completeness"`
}

type WebsiteExtractPrompt struct {
	System string
	Prompt string
}

func BuildWebsiteExtractPrompt(in WebsiteExtractInput) WebsiteExtractPrompt {
	body := in.Body
	if len(body) > 12000 {
		body = body[:12000]
	}
	header := map[string]string{
		"business_name": in.BusinessName,
		"country":       in.Country,
		"url":           in.URL,
	}
	headerJSON, _ := json.Marshal(header)

	prompt := fmt.Sprintf(
		"## Context\n%s\n\n## Page content\n%s\n\nExtract structured data. Return JSON only.",
		string(headerJSON),
		body,
	)
	return WebsiteExtractPrompt{
		System: withPreamble(websiteExtractSystem),
		Prompt: prompt,
	}
}
