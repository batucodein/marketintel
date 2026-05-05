// Package leadctx assembles the per-contact context the AI prompts need —
// the business row, a human-readable shipment summary, and the latest lead
// score. Conversation, campaign, and sequence services all share these
// helpers so the prompt inputs stay consistent.
package leadctx

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// Loader reads the project DB to assemble prompt-ready context.
type Loader struct {
	pool *pgxpool.Pool
}

func NewLoader(pool *pgxpool.Pool) *Loader {
	return &Loader{pool: pool}
}

// BusinessJSON returns the business row encoded as the AI prompt expects.
// Falls back to a placeholder string on error (rather than fail the draft).
func (l *Loader) BusinessJSON(ctx context.Context, id uuid.UUID) string {
	var b domain.Business
	row := l.pool.QueryRow(ctx,
		`SELECT id, name, website, country_code, city, address, phone, email,
		        industry, sub_industry, business_type, description, shipment_data
		 FROM businesses WHERE id = $1`, id,
	)
	if err := row.Scan(&b.ID, &b.Name, &b.Website, &b.CountryCode, &b.City,
		&b.Address, &b.Phone, &b.Email, &b.Industry, &b.SubIndustry,
		&b.BusinessType, &b.Description, &b.ShipmentData); err != nil {
		return fmt.Sprintf("(business %s not loadable: %v)", id, err)
	}
	return prompts.EncodeJSON(b)
}

// ShipmentContext returns a one-line summary of trade activity for the
// business — empty string when no shipment data is on file.
func (l *Loader) ShipmentContext(ctx context.Context, id uuid.UUID) string {
	var rawJSON []byte
	err := l.pool.QueryRow(ctx, `SELECT shipment_data FROM businesses WHERE id = $1`, id).Scan(&rawJSON)
	if err != nil || len(rawJSON) == 0 {
		return ""
	}
	var sd struct {
		TransactionCount int     `json:"transaction_count"`
		TotalWeightKG    float64 `json:"total_weight_kg"`
		TotalValueUSD    float64 `json:"total_value_usd"`
		LastShipmentDate string  `json:"last_shipment_date"`
		Products         []string `json:"products"`
		Suppliers        []struct {
			Name    string `json:"name"`
			Country string `json:"country"`
		} `json:"suppliers"`
		TrustTier string `json:"trust_tier"`
	}
	if err := json.Unmarshal(rawJSON, &sd); err != nil {
		return ""
	}
	var parts []string
	if sd.TransactionCount > 0 {
		parts = append(parts, fmt.Sprintf("%d shipments totaling %.0f kg", sd.TransactionCount, sd.TotalWeightKG))
	}
	if sd.TotalValueUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.0f declared value", sd.TotalValueUSD))
	}
	if sd.LastShipmentDate != "" {
		parts = append(parts, "last shipment "+sd.LastShipmentDate)
	}
	if len(sd.Products) > 0 {
		parts = append(parts, "products: "+strings.Join(sd.Products, ", "))
	}
	if len(sd.Suppliers) > 0 {
		var sups []string
		for _, s := range sd.Suppliers {
			sups = append(sups, s.Name+" ("+s.Country+")")
		}
		parts = append(parts, "current suppliers: "+strings.Join(sups, "; "))
	}
	if sd.TrustTier != "" {
		parts = append(parts, "trust tier: "+sd.TrustTier)
	}
	return strings.Join(parts, " | ")
}

// LeadScoreJSON returns the highest-scored, most-recent lead score for the
// business scoped to the user. Empty string if none.
func (l *Loader) LeadScoreJSON(ctx context.Context, userID, businessID uuid.UUID) string {
	var (
		overall, purchase, dealSize, urgency, fit, access *int
		rationale, recApproach                            *string
	)
	err := l.pool.QueryRow(ctx,
		`SELECT overall_score, purchase_likelihood, deal_size_potential,
		        urgency_score, fit_score, accessibility_score,
		        scoring_rationale, recommended_approach
		 FROM lead_scores
		 WHERE business_id = $1 AND user_id = $2
		 ORDER BY overall_score DESC NULLS LAST, scored_at DESC
		 LIMIT 1`, businessID, userID,
	).Scan(&overall, &purchase, &dealSize, &urgency, &fit, &access, &rationale, &recApproach)
	if err != nil {
		return ""
	}
	return prompts.EncodeJSON(map[string]any{
		"overall_score":        overall,
		"purchase_likelihood":  purchase,
		"deal_size_potential":  dealSize,
		"urgency_score":        urgency,
		"fit_score":            fit,
		"accessibility_score":  access,
		"scoring_rationale":    rationale,
		"recommended_approach": recApproach,
	})
}

// SenderHasCatalog reports whether the sender profile has a usable catalog
// uploaded and ready to attach.
func SenderHasCatalog(sp *domain.SenderProfile) (bool, string) {
	if sp == nil || sp.CatalogFileName == nil || *sp.CatalogFileName == "" {
		return false, ""
	}
	if sp.CatalogSizeBytes == nil || *sp.CatalogSizeBytes <= 0 {
		return false, ""
	}
	return true, *sp.CatalogFileName
}
