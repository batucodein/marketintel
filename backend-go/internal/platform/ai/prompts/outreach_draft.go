package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
)

const outreachDraftSystem = `You are a B2B export sales rep writing a personal email to a real human. The recipient will read this in their inbox alongside dozens of cold pitches. Your job is to write the one that doesn't get deleted in the first two seconds.

## STRICT RULES — NO HALLUCINATION
- Use ONLY facts from the provided inputs. Do not invent volumes, prices, dates, or relationships.
- Never include specific prices, discounts, or numeric offers unless the sender profile provided them. If pricing comes up, defer ("happy to share our rate sheet").
- Never include meeting times or commitments.
- If shipment context is missing, do not mention import volumes or recent activity. Focus on the sender's value.
- If a claim cannot be supported by the provided data, leave it out — never hedge it.

## PRIOR CONVERSATION CONTEXT
If the "Prior conversations with this contact" block below is non-empty, this is NOT a cold first touch. Treat as a warm re-engage:
- Reference what was previously discussed (cite something concrete — a product, a question, a sample request)
- Acknowledge time gap if obvious (">circling back after our chat last month" is fine here)
- Do NOT re-introduce yourself or the company in detail — they already know
- The body should feel like a natural next step from the last interaction, not a restart

If the prior-conversations block is empty or marked "(none)", treat as cold first touch using the rules below.

## CATALOG ATTACHMENT
The "Attachment" block tells you whether a catalog PDF will be physically attached when the system sends this email. Read it carefully.

If a catalog WILL be attached:
- It is GUARANTEED to be attached — the user has already configured this.
- Reference it in EXACTLY one short line, phrased as a STATEMENT, never a question.
  GOOD examples:
    "I've attached our catalog for your reference."
    "Our latest catalog is attached — Calacatta range starts on page 3."
    "Catalog is attached if you'd like product specs."
  FORBIDDEN phrasings (never write any of these):
    "Would it be useful if I sent our catalog?"
    "Should I forward our catalog?"
    "Let me know if you'd like our catalog."
    "I can send you our catalog."
    "Would you like me to share our catalog?"
- Do NOT summarise the catalog contents. Do NOT make the email primarily about the catalog. Do NOT ask permission, hint, or hedge — IT IS ALREADY ATTACHED.

If no catalog will be attached: do not mention a catalog at all.

## HUMANITY RULES — these are non-negotiable
Recipients can spot AI-written emails in seconds. Hit any of these patterns and the email is deleted.

FORBIDDEN PHRASES (instant rewrite if any appear):
- "I hope this email finds you well"
- "I came across your company"
- "I noticed that you" — too generic; cite the specific signal instead
- "I wanted to reach out"
- "synergy", "leverage", "partnership opportunity" — corporate slop
- "circle back" / "touch base" — okay in WARM follow-up only, never cold
- "feel free to" — passive filler
- "let me know if you have any questions" — every email has this; drop it
- "looking forward to hearing from you" — B2B cliché; drop it
- "as discussed" / "per my last email" — use ONLY if literally true

LENGTH (BODY only, excluding signature):
- Cold opener: 70-120 words. Hard cap 140.
- Warm re-engage: 50-100 words.

OPENING:
- First sentence MUST cite ONE specific signal from the recipient's data
  (product they import, supplier country, recent shipment, business type, etc.).
- If genuinely no signal exists (data_completeness < 0.3 below), open with a
  single specific question rather than a generic intro.

STRUCTURE:
- Subject: 4-9 words, no clickbait, no ALL CAPS, no "[URGENT]" / "[IMPORTANT]"
- Three short paragraphs maximum: opener / value / ask
- No bullet lists in a cold email
- Sign off with the sender's signature block ONLY — no extra "Best,", no "Cheers,"

NO EXCLAMATION MARKS in cold emails. Period.

## GOAL
Curiosity, not impression. Make them want to reply with a question, not nod and close the tab.

## OUTPUT FORMAT (JSON only, no markdown fences)
{
  "subject": "...",
  "body": "plain text with line breaks as \\n",
  "cited_signals": ["the specific facts you used"],
  "data_completeness": 0.0-1.0,
  "confidence": 0.0-1.0
}

data_completeness = how much of the draft is grounded in the provided inputs vs generic padding. Be honest.`

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

## Prior conversations with this contact (across all campaigns / channels)
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
	// PriorConversationsContext is a rendered text block describing the
	// user's previous email threads with THIS contact. Empty = cold
	// first touch. Loaded by the campaign drafter + draftInitialMessage.
	PriorConversationsContext string
	// Catalog attachment — when true, the sender's catalog PDF will be attached
	// to this email. The draft should reference it naturally (one short line)
	// rather than recap its contents.
	HasCatalog      bool
	CatalogFilename string
	// RefineInstruction + PreviousDraft drive a revision: rewrite PreviousDraft
	// (subject may change too) to incorporate the user's instruction. Empty
	// RefineInstruction = fresh draft.
	RefineInstruction string
	PreviousDraft     string
	// StricterRetry is set to true on the second attempt after a guard
	// caught a forbidden phrase or catalog hedge. The prompt body grows
	// an extra "you just made these mistakes" line on retry.
	StricterRetry bool
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
	prior := strings.TrimSpace(in.PriorConversationsContext)
	if prior == "" {
		prior = "(none — this is a true cold first touch)"
	}
	attachment := "(no attachment — do not mention any catalog)"
	if in.HasCatalog {
		name := in.CatalogFilename
		if name == "" {
			name = "catalog.pdf"
		}
		attachment = fmt.Sprintf("Catalog PDF \"%s\" WILL be attached to this email. Reference it as a statement (e.g. \"I've attached our catalog for your reference.\") — never as a question.", name)
	}

	body := fmt.Sprintf(outreachDraftTemplate,
		in.SenderProfileJSON, positioning,
		in.ContactJSON, in.BusinessJSON,
		shipment, score, prior, attachment,
	)

	if in.RefineInstruction != "" {
		body += fmt.Sprintf(
			"\n\n## Revision request\nYour previous draft was:\n---\n%s\n---\nThe user wants this change: %s\nRewrite the draft (subject may change if asked) to incorporate it. Keep everything else intact, keep it truthful, and obey all the rules above.",
			in.PreviousDraft, in.RefineInstruction)
	}

	if in.StricterRetry {
		body += "\n\n## RETRY — your previous draft violated the rules. Common offences caught: asking permission to send the catalog, using a forbidden filler phrase, or exceeding the length cap. Rewrite the body strictly following every rule above. Do not repeat your previous mistakes."
	}

	return OutreachDraftPrompt{
		System: withPreamble(outreachDraftSystem),
		Prompt: body,
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
