package domain

import "strings"

// SourceStatus represents the availability state of a data source.
type SourceStatus string

const (
	SourceAvailable     SourceStatus = "available"
	SourcePartial       SourceStatus = "partial"
	SourceUnavailable   SourceStatus = "unavailable"
	SourceNotConfigured SourceStatus = "not_configured"
)

// SourceReport holds the status of a single data source in a pipeline run.
type SourceReport struct {
	Source  string       `json:"source"`
	Status  SourceStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Items   int          `json:"items,omitempty"`
}

// DataQuality aggregates all source statuses for a pipeline stage.
// Included in API responses so the frontend can show data transparency.
// Passed to AI prompts so the model knows what's missing (Level 6 anti-hallucination).
type DataQuality struct {
	Sources             []SourceReport `json:"sources"`
	OverallCompleteness float64        `json:"overall_completeness"` // 0.0-1.0
	MissingDataNote     string         `json:"missing_data_note,omitempty"`
}

// AddSource appends a source report and recalculates completeness.
func (dq *DataQuality) AddSource(source string, status SourceStatus, message string, items int) {
	dq.Sources = append(dq.Sources, SourceReport{
		Source:  source,
		Status:  status,
		Message: message,
		Items:   items,
	})
	dq.recalculate()
}

func (dq *DataQuality) recalculate() {
	if len(dq.Sources) == 0 {
		dq.OverallCompleteness = 0
		return
	}

	var available float64
	for _, s := range dq.Sources {
		switch s.Status {
		case SourceAvailable:
			available += 1.0
		case SourcePartial:
			available += 0.5
		}
	}
	dq.OverallCompleteness = available / float64(len(dq.Sources))

	// Build missing data note for AI prompts
	var missing []string
	for _, s := range dq.Sources {
		if s.Status == SourceUnavailable || s.Status == SourceNotConfigured {
			missing = append(missing, s.Source)
		}
	}
	if len(missing) > 0 {
		dq.MissingDataNote = "UNAVAILABLE DATA SOURCES: " + strings.Join(missing, ", ") +
			". Do NOT infer, estimate, or fabricate data from these sources. " +
			"If your response requires data from an unavailable source, output \"unknown\" or \"insufficient data\" for those fields."
	} else {
		dq.MissingDataNote = ""
	}
}

// TrustTier classifies a data source into a trust level for the frontend.
func TrustTier(dataSource string) string {
	switch dataSource {
	case "tendata", "tendata_import":
		return "confirmed" // customs/trade data — highest trust
	case "tendata_verified":
		return "verified" // customs data cross-checked with Google Places
	case "google_places":
		return "potential" // directory listing — medium trust
	default:
		return "inferred" // AI-enriched or unknown — lowest trust
	}
}

// BusinessTrustTier returns a more specific trust tier using both data source and business type.
// Used when customs data distinguishes between existing buyers and new-market importers.
func BusinessTrustTier(dataSource, businessType string) string {
	// tendata_verified: customs data cross-checked with Google Places
	if dataSource == "tendata_verified" {
		switch businessType {
		case "confirmed_buyer":
			return "verified_buyer" // verified buyer — highest confidence lead
		case "confirmed_importer":
			return "verified_importer" // verified importer — high confidence
		default:
			return "verified"
		}
	}

	switch businessType {
	case "confirmed_buyer":
		return "confirmed_buyer" // imports from user's country — warmest lead
	case "confirmed_importer":
		return "confirmed_importer" // imports product from other countries
	}
	return TrustTier(dataSource)
}
