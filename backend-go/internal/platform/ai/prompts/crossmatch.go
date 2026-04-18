package prompts

import (
	"encoding/json"
	"fmt"
)

const crossMatchSystem = `You are a B2B company matching expert. Your task is to determine whether companies from a trade data source (Tendata) match companies found on Google Places.

Rules:
- Only match if you are confident they are the same company. Do NOT force matches.
- Companies may have name variations: abbreviations, legal suffixes (Inc, LLC, ULC, Ltd), punctuation differences, or partial names.
  Examples of valid matches:
  - "MSI STONE ULC" matches "MS International" (same company, different name forms)
  - "BEDROSIANS TILE AND STONE" matches "Bedrosian's Tile & Stone Inc." (punctuation and suffix variation)
  - "ABC IMPORTS LLC" matches "ABC Imports" (legal suffix dropped)
- Consider the address/city as supporting evidence — a name match in the same city is stronger.
- If a Tendata company has no plausible match among the Places candidates, do NOT include it in the output.
- Return ONLY confident matches. An empty result array is perfectly acceptable.

Always respond with valid JSON only, no markdown fences.`

const crossMatchTemplate = `Match Tendata companies against Google Places candidates. Return only confident matches.

Tendata businesses:
%s

Google Places candidates:
%s

For each match, return:
- "tendata_id": the Tendata business ID
- "place_id": the Google Places place_id
- "confidence": 0.0 to 1.0 — how confident you are this is the same company

Respond with a JSON array of matches (may be empty if no confident matches):
[{"tendata_id": "...", "place_id": "...", "confidence": 0.9}]`

// CrossMatchEntry represents a Tendata business for cross-matching.
type CrossMatchEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	City string `json:"city"`
}

// PlacesCandidate represents a Google Places result for cross-matching.
type PlacesCandidate struct {
	PlaceID string `json:"place_id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Types   string `json:"types"`
	Website string `json:"website"`
}

// CrossMatchResult is a single matched pair returned by the AI.
type CrossMatchResult struct {
	TendataID  string  `json:"tendata_id"`
	PlaceID    string  `json:"place_id"`
	Confidence float64 `json:"confidence"`
}

// CrossMatchInput groups the two lists for a cross-match prompt.
type CrossMatchInput struct {
	TendataBusinesses []CrossMatchEntry
	PlacesCandidates  []PlacesCandidate
}

// CrossMatchPrompt holds the system and user prompts for cross-matching.
type CrossMatchPrompt struct {
	System string
	Prompt string
}

// BuildCrossMatchPrompt creates a prompt that asks the AI to match Tendata
// businesses against Google Places candidates.
func BuildCrossMatchPrompt(tendataList []CrossMatchEntry, placesList []PlacesCandidate) CrossMatchPrompt {
	tendataJSON, _ := json.MarshalIndent(tendataList, "", "  ")
	placesJSON, _ := json.MarshalIndent(placesList, "", "  ")

	return CrossMatchPrompt{
		System: withPreamble(crossMatchSystem),
		Prompt: fmt.Sprintf(crossMatchTemplate, string(tendataJSON), string(placesJSON)),
	}
}
