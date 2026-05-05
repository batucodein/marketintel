package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// HeaderSample is the first-50-rows snapshot we feed to the AI mapper and
// also return to the frontend so it can render the side-by-side mapping UI.
type HeaderSample struct {
	Headers []string   `json:"headers"`
	Samples [][]string `json:"samples"` // up to 5 rows of trimmed values
}

// PreviewResult is the full payload returned by /discover/preview.
type PreviewResult struct {
	Sample           HeaderSample             `json:"sample"`
	Mapping          map[string]string        `json:"mapping"`           // header → canonical_key
	Confidence       map[string]float64       `json:"confidence"`        // header → 0..1
	UnmappedHeaders  []string                 `json:"unmapped_headers"`
	MissingRequired  []string                 `json:"missing_required"`
	Reasoning        map[string]string        `json:"reasoning"`         // header → AI rationale
	Canonical        []FieldDef               `json:"canonical"`         // catalog returned for the UI
	DataCompleteness float64                  `json:"data_completeness"`
}

// ReadHeaderSample opens the uploaded spreadsheet, returns the first row as
// headers and up to 5 subsequent rows as samples. Used by Preview.
func ReadHeaderSample(reader io.Reader) (HeaderSample, error) {
	f, err := excelize.OpenReader(reader)
	if err != nil {
		return HeaderSample{}, fmt.Errorf("excel: open: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return HeaderSample{}, fmt.Errorf("excel: no sheets found")
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return HeaderSample{}, fmt.Errorf("excel: read rows: %w", err)
	}
	if len(rows) < 1 {
		return HeaderSample{}, fmt.Errorf("excel: empty file")
	}

	headers := append([]string(nil), rows[0]...)
	var samples [][]string
	maxSamples := 5
	for i := 1; i < len(rows) && i <= maxSamples; i++ {
		// Pad / truncate so each sample has len(headers) cells.
		row := rows[i]
		fixed := make([]string, len(headers))
		for j := range fixed {
			if j < len(row) {
				fixed[j] = row[j]
			}
		}
		samples = append(samples, fixed)
	}
	return HeaderSample{Headers: headers, Samples: samples}, nil
}

// AIMapper wraps the AI router with the canonical catalog and produces a
// PreviewResult. Callers (the preview handler, and the legacy auto-import
// path) share the same logic.
type AIMapper struct {
	ai *ai.Router
}

func NewAIMapper(router *ai.Router) *AIMapper {
	return &AIMapper{ai: router}
}

// Map runs the AI mapping prompt and returns a sanitised PreviewResult.
// userID is used purely for AI cost-attribution.
func (m *AIMapper) Map(ctx context.Context, userID uuid.UUID, sample HeaderSample) (*PreviewResult, error) {
	if len(sample.Headers) == 0 {
		return nil, fmt.Errorf("no headers in upload")
	}

	catalog := make([]map[string]any, 0, len(Canonical))
	allowed := make(map[string]bool, len(Canonical))
	for _, f := range Canonical {
		catalog = append(catalog, map[string]any{
			"key":         f.Key,
			"label":       f.Label,
			"required":    f.Required,
			"group":       f.Group,
			"description": f.Description,
		})
		allowed[f.Key] = true
	}

	prompt := prompts.BuildExcelMappingPrompt(prompts.ExcelMappingInput{
		Headers:          sample.Headers,
		Samples:          sample.Samples,
		CanonicalCatalog: catalog,
	})

	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := m.ai.CompleteJSON(ctxAI, "excel_mapping", prompt.Prompt, prompt.System, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("ai mapping: %w", err)
	}

	var aiOut prompts.ExcelMappingResult
	if err := json.Unmarshal(raw, &aiOut); err != nil {
		return nil, fmt.Errorf("ai mapping: parse: %w", err)
	}

	cleaned, dropped := prompts.SanitizeExcelMapping(aiOut, allowed)
	if len(dropped) > 0 {
		slog.Info("excel_mapping: dropped untrusted entries", "dropped", dropped)
	}

	mapping := make(map[string]string)
	confidence := make(map[string]float64)
	reasoning := make(map[string]string)
	for _, m := range cleaned.Mappings {
		mapping[m.Header] = m.Canonical
		confidence[m.Header] = m.Confidence
		reasoning[m.Header] = m.Reason
	}

	// Recompute missing_required from server-side truth (don't trust AI alone).
	mappedKeys := map[string]bool{}
	for _, k := range mapping {
		mappedKeys[k] = true
	}
	var missingReq []string
	for _, k := range RequiredKeys() {
		if !mappedKeys[k] {
			missingReq = append(missingReq, k)
		}
	}

	// Recompute unmapped_headers from server-side truth.
	mappedHeaders := map[string]bool{}
	for h := range mapping {
		mappedHeaders[h] = true
	}
	var unmapped []string
	for _, h := range sample.Headers {
		if !mappedHeaders[h] {
			unmapped = append(unmapped, h)
		}
	}

	return &PreviewResult{
		Sample:           sample,
		Mapping:          mapping,
		Confidence:       confidence,
		UnmappedHeaders:  unmapped,
		MissingRequired:  missingReq,
		Reasoning:        reasoning,
		Canonical:        Canonical,
		DataCompleteness: cleaned.DataCompleteness,
	}, nil
}
