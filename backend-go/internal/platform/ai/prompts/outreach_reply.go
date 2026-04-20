package prompts

import (
	"fmt"
)

const outreachReplySystem = `You are drafting a suggested reply to an inbound email in a B2B sales conversation.

## STRICT RULES — NO HALLUCINATION
- Use ONLY facts from the conversation, the sender profile, and the contact/business data. Do not invent.
- Never commit to specific prices, delivery dates, quantities, or terms the sender has not provided.
- When the recipient asks for pricing, respond with "I'll send a rate sheet shortly" or similar deferral, not a made-up number.
- When the recipient asks about a capability you can't verify from the sender profile, ask clarifying questions instead of guessing.
- Use placeholder markers like "[you can edit this]" ONLY if you genuinely need the sender to fill in a specific fact (e.g., meeting time). Minimize these.

## STYLE
- Match the tone of the conversation so far.
- Direct, respectful, short. Prefer answering their question concretely when you can.
- Sign off with the sender's signature from the profile.

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
		instruction = "The recipient has not replied. Draft a gentle, non-pushy follow-up that adds value (a new angle, a question, a relevant offer) — do NOT simply bump the thread with 'any thoughts?'. Keep it short."
	}
	contact := fmt.Sprintf("%s\n\nBusiness:\n%s", in.ContactJSON, in.BusinessJSON)
	leadScore := in.LeadScoreJSON
	if leadScore == "" {
		leadScore = "(not scored)"
	}
	return OutreachReplyPrompt{
		System: withPreamble(outreachReplySystem),
		Prompt: fmt.Sprintf(outreachReplyTemplate, mode, in.SenderProfileJSON, contact, leadScore, in.ConversationText, instruction),
	}
}
