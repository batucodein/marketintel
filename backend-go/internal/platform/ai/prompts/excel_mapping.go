package prompts

import (
	"encoding/json"
	"fmt"
	"strings"
)

// outreachMappingSystem (named excelMappingSystem here, kept short) tells
// the AI to map raw Excel headers to MarketIntel's canonical field keys.
// We deliberately give it ONLY the catalog of allowed keys plus their
// descriptions — never let it invent new keys, never let it hallucinate
// a mapping when no header fits.
const excelMappingSystem = `You map columns of an uploaded Excel file to MarketIntel's canonical field catalog.

## INPUTS YOU RECEIVE
1. The list of detected header names (raw, in order)
2. Up to 5 sample rows so you can sanity-check the column contents
3. The catalog of canonical fields you may map to (key, label, description)

## STRICT RULES
- ONLY map a header to a canonical key listed in the catalog. Do NOT invent keys.
- A header without a confident match must be left unmapped (omit it from "mappings" or set canonical to null).
- Confidence is a number 0.0 to 1.0. Only emit confidence >= 0.6 mappings.
- If multiple headers plausibly fit the same canonical key (e.g. "Buyer" and "Consignee" both → consignee_name), map BOTH and rank by confidence; the importer will pick the first non-empty value at row time.
- Pay attention to the sample values, not just the header text. If a header reads "Phone" but the column actually contains email addresses, map it to consignee_email instead.
- When in doubt, do not guess. Mapping the wrong header is worse than leaving a field unmapped.
- Required canonical fields that no header maps to MUST appear in "missing_required". The user will be warned.

## OUTPUT (JSON only, no markdown fences)
{
  "mappings": [
    {"header": "Consignee Std Name", "canonical": "consignee_name", "confidence": 0.98, "reason": "exact synonym + sample values are company names"}
  ],
  "unmapped_headers": ["Mode of Transport", "Notes"],
  "missing_required": ["consignee_country"],
  "data_completeness": 0.0-1.0
}

data_completeness here = the share of REQUIRED canonical fields that you confidently mapped (1.0 = all required mapped, 0.0 = none).`

// ExcelMappingInput is the typed input for BuildExcelMappingPrompt.
type ExcelMappingInput struct {
	// Headers are the raw column names exactly as they appear in row 1
	// of the uploaded file.
	Headers []string

	// Samples is up to 5 rows of values (each row aligned to Headers).
	Samples [][]string

	// CanonicalCatalog describes the allowed mapping targets. Each entry
	// is a {key, label, description, required, group} map — the AI
	// reads `key` as the value to emit.
	CanonicalCatalog []map[string]any
}

// ExcelMappingResult is what the AI returns; the orchestration layer
// then enforces server-side rules (unknown keys are dropped, etc.).
type ExcelMappingResult struct {
	Mappings []struct {
		Header     string  `json:"header"`
		Canonical  string  `json:"canonical"`
		Confidence float64 `json:"confidence"`
		Reason     string  `json:"reason"`
	} `json:"mappings"`
	UnmappedHeaders  []string `json:"unmapped_headers"`
	MissingRequired  []string `json:"missing_required"`
	DataCompleteness float64  `json:"data_completeness"`
}

type ExcelMappingPrompt struct {
	System string
	Prompt string
}

func BuildExcelMappingPrompt(in ExcelMappingInput) ExcelMappingPrompt {
	headersJSON, _ := json.MarshalIndent(in.Headers, "", "  ")
	samplesJSON, _ := json.MarshalIndent(in.Samples, "", "  ")
	catalogJSON, _ := json.MarshalIndent(in.CanonicalCatalog, "", "  ")

	var b strings.Builder
	b.WriteString("## Detected headers (in order)\n")
	b.Write(headersJSON)
	b.WriteString("\n\n## Sample rows (up to 5)\n")
	b.Write(samplesJSON)
	b.WriteString("\n\n## Canonical field catalog (allowed mapping targets)\n")
	b.Write(catalogJSON)
	b.WriteString("\n\nMap each header. Return JSON only.")

	return ExcelMappingPrompt{
		System: withPreamble(excelMappingSystem),
		Prompt: b.String(),
	}
}

// Sanity-check the AI output: drop mappings whose `canonical` is not in
// the allow-list. Returns the cleaned result + the set of dropped entries
// (logged by the caller for observability).
func SanitizeExcelMapping(in ExcelMappingResult, allowed map[string]bool) (cleaned ExcelMappingResult, dropped []string) {
	cleaned = ExcelMappingResult{
		UnmappedHeaders:  in.UnmappedHeaders,
		MissingRequired:  in.MissingRequired,
		DataCompleteness: in.DataCompleteness,
	}
	for _, m := range in.Mappings {
		if !allowed[m.Canonical] {
			dropped = append(dropped, fmt.Sprintf("%s→%s (not in catalog)", m.Header, m.Canonical))
			continue
		}
		if m.Confidence < 0.6 {
			dropped = append(dropped, fmt.Sprintf("%s→%s (confidence %.2f below 0.6)", m.Header, m.Canonical, m.Confidence))
			continue
		}
		cleaned.Mappings = append(cleaned.Mappings, m)
	}
	return cleaned, dropped
}
