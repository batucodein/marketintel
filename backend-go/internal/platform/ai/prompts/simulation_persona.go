package prompts

import "fmt"

// simulationPersonaSystem makes the model role-play a B2B buyer receiving a
// cold/follow-up email, replying in character. This is a SIMULATION role — the
// model is allowed (expected) to invent plausible buyer details; it must NOT
// use the no-hallucination preamble.
const simulationPersonaSystem = `You are role-playing a real B2B buyer at an importing company who just received a sales email. Reply exactly as that person would — short, human, in their voice. This is a test simulation, so invent plausible specifics (volumes, projects, objections) consistent with the persona.

## YOUR PERSONA
%s

## RULES
- Write ONLY the reply body text the buyer would send. No subject, no signature block, no quoted original.
- Match real inbox behaviour: busy people are terse. 1-4 sentences is normal.
- Stay 100%% in character. If the persona would not reply at all, that is handled elsewhere — when asked to reply, you always produce the in-character text.
- Do not reveal you are an AI or a simulation.
- React to what the sender actually wrote (their product, their ask), not generic filler.

## OUTPUT (JSON only, no markdown fences)
{
  "body": "the reply text",
  "intent": "one of: interested | asking_info | negotiating | declining | unsubscribing | wrong_person | out_of_office"
}`

// PersonaReplyInput is the typed input for a simulated buyer reply.
type PersonaReplyInput struct {
	PersonaPrompt    string // the persona's behavioural/intent description
	BusinessContext  string // who the buyer is (synthetic, from a real market business)
	ConversationText string // rendered thread so far (oldest first), YOU = sender, THEM = buyer
}

// PersonaReplyResult is the parsed reply.
type PersonaReplyResult struct {
	Body   string `json:"body"`
	Intent string `json:"intent"`
}

type PersonaReplyPrompt struct {
	System string
	Prompt string
}

func BuildPersonaReplyPrompt(in PersonaReplyInput) PersonaReplyPrompt {
	ctx := in.BusinessContext
	if ctx == "" {
		ctx = "(no extra company context)"
	}
	prompt := fmt.Sprintf(
		"## You are this company\n%s\n\n## Conversation so far\n%s\n\nWrite your in-character reply now. Return JSON only.",
		ctx, in.ConversationText,
	)
	return PersonaReplyPrompt{
		System: fmt.Sprintf(simulationPersonaSystem, in.PersonaPrompt),
		Prompt: prompt,
	}
}
