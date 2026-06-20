package prompts

import (
	"fmt"
)

const outreachReplySystem = `You are drafting the next message in an ongoing B2B sales conversation. The recipient has either replied to you, or they haven't replied and it's time for a non-pushy follow-up. Either way, the conversation history below is your most important input — read it before you write a single word.

## STRICT RULES — NO HALLUCINATION
- Use ONLY facts from the conversation, the sender profile, and the contact/business data. Do not invent.
- Never commit to specific prices, delivery dates, quantities, or terms the sender has not provided.
- When the recipient asks for pricing, respond with "I'll send a rate sheet shortly" or similar deferral — never a fabricated number.
- When the recipient asks about a capability you can't verify from the sender profile, ask a clarifying question instead of guessing.
- Use placeholder markers like "[you can edit this]" ONLY if you genuinely need the sender to fill in a specific fact (e.g. a meeting time). Keep them rare.

## CONVERSATION AWARENESS — read this carefully
The "Full conversation" block below contains every prior message in this thread, oldest first. YOU = outbound (sent by the user this AI is helping). THEM = inbound (the recipient).

Before writing, scan for:
- Specific questions THEY asked. ANSWER them directly. Never make them repeat themselves.
- Facts THEY shared (volumes, projects, timeline, supplier issues). Reference one if relevant.
- Tone they used (formal/casual, short/detailed). Match it.
- Things YOU promised. If YOU said "I'll send a rate sheet" in the last outbound, this reply should DELIVER the rate sheet (or apologise + deliver), not pretend it never happened.

## CATALOG ATTACHMENT
The "Attachment" block tells you whether a catalog PDF will be attached to THIS outbound.

If a catalog WILL be attached:
- It is GUARANTEED. Reference in one short STATEMENT line, never a question.
  GOOD: "Catalog is attached — Calacatta range starts on page 3."
  FORBIDDEN: "Would you like me to send our catalog?", "Should I attach our catalog?", "Let me know if you'd like our catalog."
- Don't recap it. Don't make the email about the catalog.

If no catalog will be attached: do not mention any catalog.

## HUMANITY RULES — non-negotiable
This is an ongoing conversation. The recipient already knows it's the user. But sound human anyway.

FORBIDDEN PHRASES (rewrite if any appear):
- "I hope this email finds you well"
- "I wanted to reach out"
- "synergy", "leverage", "partnership opportunity"
- "feel free to"
- "let me know if you have any questions"
- "looking forward to hearing from you"
- "as discussed" / "per my last email" — use ONLY if literally true in the thread

LENGTH (body, excluding signature):
- Reply: 40-90 words. Shorter is usually better.
- Follow-up: 30-70 words. Always shorter than the initial cold email.

STRUCTURE:
- No greeting on a 3rd+ message in a thread (already established rapport)
- Address their question (if reply mode) or add value (if followup mode) in the FIRST sentence
- Maximum two short paragraphs
- Sign off with the sender's signature ONLY — no extra "Best,", no "Cheers,"

Reply emails: one exclamation mark allowed if it fits the tone they're using. Otherwise none.

## FOLLOW-UP MODE specifics (when task is "followup")
The recipient hasn't replied. Don't apologise for that. Don't say "just bumping this" or "any thoughts?". Add real new value:
- A relevant news / industry note tied to their business
- A new angle on the original ask
- A specific question they can answer in one line
- Or an offer that costs them nothing to accept (sample, 15-min call, white paper link)

## OUTPUT FORMAT (JSON only)
{
  "body": "plain text reply with line breaks",
  "needs_user_input": ["list of facts the user should verify before sending, empty array if none"],
  "confidence": 0.0-1.0
}`

const outreachReplyTemplate = `## Task mode
%s

## Sender profile
%s

## Contact + business
%s

## Lead score (our system's analysis)
%s

## Full conversation (oldest first; YOU = outbound, THEM = inbound)
%s

## Attachment on this outbound
%s

%s

Return JSON only.`

type OutreachReplyInput struct {
	// Mode: "reply" | "followup"
	// - reply: respond to the most recent inbound message from the recipient
	// - followup: gentle non-pushy bump when the last outbound got no reply
	Mode              string
	SenderProfileJSON string
	ContactJSON       string
	BusinessJSON      string
	LeadScoreJSON     string // optional; "(not scored)" if absent
	ConversationText  string
	// HasCatalog means this outbound will have the sender's catalog PDF attached.
	HasCatalog      bool
	CatalogFilename string
	// Guidance is the user's standing reply guidance for this kind of reply —
	// the per-group playbook + remembered per-brand lessons. Empty = none.
	Guidance string
	// RefineInstruction + PreviousDraft drive a revision: rewrite PreviousDraft
	// to incorporate the user's instruction. Empty RefineInstruction = fresh draft.
	RefineInstruction string
	PreviousDraft     string
	// StricterRetry on the second attempt after a guard caught a violation.
	StricterRetry bool
}

type OutreachReplyResult struct {
	Body           string   `json:"body"`
	NeedsUserInput []string `json:"needs_user_input"`
	Confidence     float64  `json:"confidence"`
}

type OutreachReplyPrompt struct {
	System string
	Prompt string
}

func BuildOutreachReplyPrompt(in OutreachReplyInput) OutreachReplyPrompt {
	mode := in.Mode
	if mode == "" {
		mode = "reply"
	}
	instruction := "Draft a reply to the most recent inbound message. Use the conversation history so you don't repeat what you already said."
	if mode == "followup" {
		instruction = "The recipient has not replied. Draft a non-pushy follow-up that adds NEW value — not 'just bumping this'. Keep it short and specific."
	}
	contact := fmt.Sprintf("%s\n\nBusiness:\n%s", in.ContactJSON, in.BusinessJSON)
	leadScore := in.LeadScoreJSON
	if leadScore == "" {
		leadScore = "(not scored)"
	}
	attachment := "(no attachment — do not mention any catalog)"
	if in.HasCatalog {
		name := in.CatalogFilename
		if name == "" {
			name = "catalog.pdf"
		}
		attachment = fmt.Sprintf("Catalog PDF \"%s\" WILL be attached. Reference it as a statement if relevant — never a question.", name)
	}

	body := fmt.Sprintf(outreachReplyTemplate, mode, in.SenderProfileJSON, contact, leadScore, in.ConversationText, attachment, instruction)

	if in.Guidance != "" {
		body += "\n\n## Standing reply guidance (the user's standing instructions for this kind of reply — follow them; still never invent facts)\n" + in.Guidance
	}

	if in.RefineInstruction != "" {
		body += fmt.Sprintf(
			"\n\n## Revision request\nYour previous draft was:\n---\n%s\n---\nThe user wants this change: %s\nRewrite the draft to incorporate it. Keep everything else intact, keep it truthful, and obey all the rules above.",
			in.PreviousDraft, in.RefineInstruction)
	}

	if in.StricterRetry {
		body += "\n\n## RETRY — your previous draft violated the rules. Common offences caught: asking permission to send an already-attached catalog, using a forbidden filler phrase, or exceeding the length cap. Rewrite the body strictly. Do not repeat your previous mistakes."
	}

	return OutreachReplyPrompt{
		System: withPreamble(outreachReplySystem),
		Prompt: body,
	}
}
