package prompts

import "fmt"

// outreachSentimentSystem classifies an inbound buyer message into a tight
// 3-bucket scheme used by sequence triggers.
const outreachSentimentSystem = `You are a sentiment classifier for a single inbound B2B sales email.

Read ONLY the message body. Decide whether the recipient is signalling interest
(positive), neutral (asking questions, gathering info, no commitment), or
disinterest (politely declining, asking to be removed, hostile).

## RULES
- Base the call on what they wrote, NOT what you wish they wrote.
- If they ask for pricing or a sample, that is positive.
- If they ask "what is this about?" without warmth, that is neutral.
- If they say "not interested", "remove me", "stop", that is negative.
- Auto-replies and out-of-office are neutral.

## OUTPUT (JSON only, no markdown fences)
{
  "sentiment": "positive" | "neutral" | "negative",
  "confidence": 0.0-1.0,
  "reason": "one short sentence citing the line that drove the call"
}`

// OutreachSentimentInput is the typed input for the sentiment classifier.
type OutreachSentimentInput struct {
	From    string
	Subject string
	Body    string
}

// OutreachSentimentResult is the parsed AI response.
type OutreachSentimentResult struct {
	Sentiment  string  `json:"sentiment"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
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
