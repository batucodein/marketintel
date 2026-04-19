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

const outreachReplyTemplate = `## Sender profile
%s

## Contact + business
%s

## Full conversation (oldest first)
%s

Draft a reply to the most recent inbound message. Return JSON only.`

type OutreachReplyInput struct {
	SenderProfileJSON string
	ContactJSON       string
	BusinessJSON      string
	ConversationText  string // rendered transcript with DIR/subject/body per message
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
	contact := fmt.Sprintf("%s\n\nBusiness:\n%s", in.ContactJSON, in.BusinessJSON)
	return OutreachReplyPrompt{
		System: withPreamble(outreachReplySystem),
		Prompt: fmt.Sprintf(outreachReplyTemplate, in.SenderProfileJSON, contact, in.ConversationText),
	}
}
