package prompts

import "fmt"

// outreachSentimentSystem classifies an inbound buyer message into (a) a
// continuous sentiment score and (b) a set of intent tags from a fixed
// vocabulary. Both come from one cheap call.
const outreachSentimentSystem = `You are a classifier for a single inbound B2B sales email reply.

Read ONLY the message body. Produce two things:

1. SCORE — how warm the reply is, as a number from -1.0 to 1.0:
   -1.0 = hostile / hard no / "remove me"
   -0.4 = polite decline, not interested right now
    0.0 = neutral (auto-reply, out-of-office, a bare question with no warmth)
    0.4 = mild interest, asking for details
    1.0 = eager, ready to buy, "send me a quote"

2. INTENT TAGS — zero or more labels, ONLY from this exact list:
   price_requested        - asks for pricing / a quote / a price list
   info_requested         - asks for catalog / specs / more product detail
   sample_requested       - asks for samples
   meeting_requested      - asks for a call / meeting / demo
   moq_question           - asks about minimum order quantity
   shipping_question      - asks about delivery / lead time / incoterms
   certification_question - asks about certificates / compliance
   ready_to_order         - explicit intent to place an order
   price_objection        - says it is too expensive
   timing_objection       - "not now", "maybe later"
   has_supplier           - already has a supplier
   not_interested         - declines, not interested
   unsubscribe            - asks to be removed / stop emailing / opt out
   out_of_office          - automated out-of-office / auto-reply
   wrong_contact          - "I'm not the right person", wrong department
   referral               - points you to a different person to contact

## RULES
- Base everything on what they actually wrote, NOT what you wish they wrote.
- Only emit a tag the recipient actually expressed. If none apply, return [].
- NEVER invent a tag to be helpful. An unsure tag is a wrong tag — omit it.
- An out-of-office / auto-reply is out_of_office with score 0.0.
- Each tag carries its own confidence 0.0-1.0.

## OUTPUT (JSON only, no markdown fences)
{
  "score": -1.0..1.0,
  "sentiment": "positive" | "neutral" | "negative",
  "confidence": 0.0-1.0,
  "reason": "one short sentence citing the line that drove the score",
  "intent_tags": [ { "tag": "price_requested", "confidence": 0.0-1.0 } ]
}`

// OutreachSentimentInput is the typed input for the classifier.
type OutreachSentimentInput struct {
	From    string
	Subject string
	Body    string
}

// IntentTagResult is one classified intent tag with its confidence.
type IntentTagResult struct {
	Tag        string  `json:"tag"`
	Confidence float64 `json:"confidence"`
}

// OutreachSentimentResult is the parsed AI response. Score is a pointer so a
// genuinely-0.0 score is distinguishable from "model omitted the field".
type OutreachSentimentResult struct {
	Score      *float64          `json:"score"`     // -1.0..1.0 (source of truth)
	Sentiment  string            `json:"sentiment"` // legacy 3-bucket, kept for back-compat
	Confidence float64           `json:"confidence"`
	Reason     string            `json:"reason"`
	IntentTags []IntentTagResult `json:"intent_tags"`
}

type OutreachSentimentPrompt struct {
	System string
	Prompt string
}

func BuildOutreachSentimentPrompt(in OutreachSentimentInput) OutreachSentimentPrompt {
	body := in.Body
	if body == "" {
		body = "(empty body)"
	}
	subj := in.Subject
	if subj == "" {
		subj = "(no subject)"
	}
	prompt := fmt.Sprintf("From: %s\nSubject: %s\n\n%s\n\nClassify. Return JSON only.",
		in.From, subj, body)
	return OutreachSentimentPrompt{
		System: withPreamble(outreachSentimentSystem),
		Prompt: prompt,
	}
}
