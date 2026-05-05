package prompts

import (
	"encoding/json"
	"fmt"
)

const outreachDraftSystem = `You are a B2B export sales assistant drafting a cold outreach email for a lead discovered via customs data.

## STRICT RULES — NO HALLUCINATION
- Use ONLY facts from the provided inputs. Do not invent volumes, prices, dates, or relationships.
- Never include specific prices, discounts, or numeric offers unless the sender profile provided them. If you need to reference pricing, use neutral phrasing like "competitive pricing" or "happy to share our rate sheet on request".
- Never imply a prior conversation. This is a FIRST touch unless conversation_history is provided.
- Never include meeting times or commitments.
- If shipment context is missing, do not mention import volumes or recent activity. Focus on the sender's value.
- If a claim cannot be supported by the provided data, leave it out entirely rather than hedge.

## GOAL
Short, personal cold email that would make the recipient curious enough to reply. Reference ONE specific signal from their profile (a product they import, a supplier country, a business type) when available — generic emails get ignored.

## STRUCTURE
- Subject: concrete, under 60 characters, no clickbait or ALL CAPS
- Greeting with the contact's first name if available, otherwise company name
- Opening line: one sentence that shows you know who they are (cite the signal)
- Middle: 2-3 sentences on the sender's relevance + value
- Close: one question or soft call-to-action
- Signature from sender profile

## OUTPUT FORMAT (JSON only, no markdown fences)
{
  "subject": "...",
  "body": "plain text with line breaks as \\n",
  "cited_signals": ["the specific facts you used"],
  "data_completeness": 0.0-1.0,
  "confidence": 0.0-1.0
}

data_completeness = how much of the draft is grounded in the provided inputs vs. generic padding. Be honest.`

const outreachDraftTemplate = `## Sender profile
%s

## Campaign goal / positioning override
%s

## Contact
%s

## Business profile (from our enrichment)
%s

## Shipment history (what they import, from where, how much)
%s

## Lead score (our system's analysis of this buyer)
%s

## Attachment
%s

Draft the email now. Return JSON only.`

// OutreachDraftInput is the typed input for the drafter.
type OutreachDraftInput struct {
	SenderProfileJSON   string
	CampaignPositioning string // empty string if using sender profile defaults
	ContactJSON         string
	BusinessJSON        string
	ShipmentContext     string // the same format used in scoring.go
	LeadScoreJSON       string // strengths, rationale — helps AI pick signals to cite
	// Catalog attachment — when true, the sender's catalog PDF will be attached
	// to this email. The draft should reference it naturally (one short line)
	// rather than recap its contents.
	HasCatalog      bool
	CatalogFilename string
}

// OutreachDraftResult is the parsed AI response.
type OutreachDraftResult struct {
	Subject          string   `json:"subject"`
	Body             string   `json:"body"`
	CitedSignals     []string `json:"cited_signals"`
	DataCompleteness float64  `json:"data_completeness"`
	Confidence       float64  `json:"confidence"`
}

type OutreachDraftPrompt struct {
	System string
	Prompt string
}

func BuildOutreachDraftPrompt(in OutreachDraftInput) OutreachDraftPrompt {
	positioning := in.CampaignPositioning
	if positioning == "" {
		positioning = "(none — use sender profile defaults)"
	}
	shipment := in.ShipmentContext
	if shipment == "" {
		shipment = "(no shipment data available for this contact)"
	}
	score := in.LeadScoreJSON
	if score == "" {
		score = "(not scored yet)"
	}
	attachment := "(no attachment)"
	if in.HasCatalog {
		name := in.CatalogFilename
		if name == "" {
			name = "catalog.pdf"
		}
		attachment = fmt.Sprintf("A product catalog PDF (%s) will be attached to this email. Reference it naturally in ONE short line (e.g., \"I've attached our catalog for your review.\") — do NOT summarize its contents and do NOT make the email about the catalog.", name)
	}
	return OutreachDraftPrompt{
		System: withPreamble(outreachDraftSystem),
		Prompt: fmt.Sprintf(outreachDraftTemplate,
			in.SenderProfileJSON, positioning,
			in.ContactJSON, in.BusinessJSON,
			shipment, score, attachment,
		),
	}
}

// EncodeJSON is a tiny helper to stringify structs for the prompt fields,
// so callers don't have to repeat json.Marshal + string() boilerplate.
func EncodeJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}
