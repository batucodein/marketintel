package prompts

import "fmt"

// simulationJudgeSystem grades a finished simulated outreach transcript so the
// team can spot quality problems before going live. It is an evaluator role,
// not a generator — no no-hallucination preamble needed.
const simulationJudgeSystem = `You are a strict reviewer grading an AI sales agent's performance in a simulated B2B outreach conversation. You see the full transcript (YOU = our AI agent, THEM = the simulated buyer) plus the buyer's known persona.

Grade the AGENT (the YOU messages), not the buyer.

## WHAT TO JUDGE
- human_feel (0-100): Would the recipient believe a thoughtful human wrote these, or does it read like automation/AI? Penalise templated openers, filler, robotic phrasing, repetition across follow-ups.
- correctness (0-100): Given the persona, did the agent behave correctly? e.g. stopped following up once they replied; did not push after a clear "not interested"; answered the buyer's actual question; right number of touches; referenced real context.
- Note any specific failures: ignored a question, repeated itself, kept emailing after a decline, generic/robotic lines, hallucinated facts.

## OUTPUT (JSON only, no markdown fences)
{
  "score": 0-100,            // overall, weighted toward human_feel + correctness
  "human_feel": 0-100,
  "correctness": 0-100,
  "notes": "2-4 sentences: what worked, what to fix"
}`

// JudgeInput is the typed input for grading one transcript.
type JudgeInput struct {
	PersonaLabel     string // e.g. "Eager buyer (asks pricing)"
	PersonaPrompt    string // the persona's behaviour description
	Outcome          string // derived outcome (replied / cold / unsubscribed / ...)
	ConversationText string // full rendered transcript
}

// JudgeResult is the parsed grade.
type JudgeResult struct {
	Score       int    `json:"score"`
	HumanFeel   int    `json:"human_feel"`
	Correctness int    `json:"correctness"`
	Notes       string `json:"notes"`
}

type JudgePrompt struct {
	System string
	Prompt string
}

func BuildJudgePrompt(in JudgeInput) JudgePrompt {
	prompt := fmt.Sprintf(
		"## Buyer persona\n%s\n%s\n\n## Final outcome\n%s\n\n## Transcript\n%s\n\nGrade the agent. Return JSON only.",
		in.PersonaLabel, in.PersonaPrompt, in.Outcome, in.ConversationText,
	)
	return JudgePrompt{
		System: simulationJudgeSystem,
		Prompt: prompt,
	}
}
