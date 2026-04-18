package prompts

// NoHallucinationPreamble is prepended to ALL system prompts.
// This is Level 1 of the no-hallucination enforcement.
const NoHallucinationPreamble = `CRITICAL: You must ONLY use the data explicitly provided below.
- Do NOT invent, assume, or hallucinate any facts.
- If a field is missing or empty, output "unknown" or "insufficient data".
- Do NOT fill in plausible-sounding values.
- Every claim must be traceable to the provided data.
- Include a "data_completeness" field (0.0-1.0) indicating how much of your response is based on actual provided data vs. general knowledge.
`

// withPreamble prepends the no-hallucination preamble to any system prompt.
func withPreamble(system string) string {
	return NoHallucinationPreamble + "\n" + system
}

// WithDataQuality appends Level 6 data availability context to a system prompt.
// When missingDataNote is non-empty, the AI is explicitly told which sources
// were unavailable so it doesn't fabricate data from those sources.
func WithDataQuality(system, missingDataNote string) string {
	if missingDataNote == "" {
		return system
	}
	return system + "\n\n" + missingDataNote
}
