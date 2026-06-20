package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

const (
	scoreBatchSize  = 8  // businesses per AI call
	scoreConcurrent = 3  // parallel AI calls
)

// Scorer implements domain.LeadScorer using the AI router with concurrent batching.
type Scorer struct {
	router   *ai.Router
	cacheTTL time.Duration
}

func NewScorer(router *ai.Router) *Scorer {
	return &Scorer{
		router:   router,
		cacheTTL: 12 * time.Hour,
	}
}

// ScoreBatch scores businesses in concurrent batches.
// 60 leads / 8 per batch = 8 batches / 3 concurrent = ~3 rounds × ~3s = ~10 seconds.
// senderProfile, when non-nil, is fed into the prompt so leads are ranked
// against the user's specific positioning (target industries/countries,
// deal-size band, deal breakers, moats) instead of just the market product.
func (s *Scorer) ScoreBatch(
	ctx context.Context,
	businesses []domain.Business,
	product domain.ProductContext,
	marketCtx *domain.MarketContext,
	senderProfile *domain.SenderProfile,
) ([]domain.LeadScore, error) {
	if len(businesses) == 0 {
		return nil, nil
	}

	// Build batches
	var batches [][]domain.Business
	for i := 0; i < len(businesses); i += scoreBatchSize {
		end := i + scoreBatchSize
		if end > len(businesses) {
			end = len(businesses)
		}
		batches = append(batches, businesses[i:end])
	}

	// Run batches concurrently with bounded parallelism
	type batchResult struct {
		index   int
		scores  []domain.LeadScore
	}

	results := make([]batchResult, len(batches))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(scoreConcurrent)

	for i, batch := range batches {
		i, batch := i, batch
		g.Go(func() error {
			scores, err := s.scoreOne(gctx, batch, product, marketCtx, senderProfile)
			if err != nil {
				slog.Error("scoring batch failed",
					"batch_index", i,
					"batch_size", len(batch),
					"error", err,
				)
				return nil // don't fail the whole group
			}
			results[i] = batchResult{index: i, scores: scores}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var allScores []domain.LeadScore
	for _, r := range results {
		allScores = append(allScores, r.scores...)
	}

	slog.Info("scoring complete",
		"total_businesses", len(businesses),
		"total_scored", len(allScores),
		"batches", len(batches),
		"concurrency", scoreConcurrent,
	)
	return allScores, nil
}

func (s *Scorer) scoreOne(
	ctx context.Context,
	batch []domain.Business,
	product domain.ProductContext,
	marketCtx *domain.MarketContext,
	senderProfile *domain.SenderProfile,
) ([]domain.LeadScore, error) {
	// Build prompt input
	promptBiz := make([]prompts.ScoreBusiness, len(batch))
	idMap := make(map[string]uuid.UUID)

	for i, b := range batch {
		idStr := b.ID.String()
		idMap[idStr] = b.ID

		sb := prompts.ScoreBusiness{
			ID:   idStr,
			Name: b.Name,
		}
		if b.CountryCode != nil {
			sb.CountryCode = *b.CountryCode
		}
		if b.City != nil {
			sb.City = *b.City
		}
		if b.Address != nil {
			sb.Address = *b.Address
		}
		if b.Website != nil {
			sb.Website = *b.Website
		}
		if b.Phone != nil {
			sb.Phone = *b.Phone
		}
		if b.Email != nil {
			sb.Email = *b.Email
		}
		if b.Industry != nil {
			sb.Industry = *b.Industry
		}
		if b.BusinessType != nil {
			sb.BusinessType = *b.BusinessType
		}
		if b.Description != nil {
			sb.Description = *b.Description
		}
		// Include customs/shipment data if available (from Excel import)
		if b.ShipmentData != nil {
			sb.ShipmentContext = shipmentContextForAI(b.ShipmentData)
		}
		// Include enriched website contact data
		if b.WebsiteData != nil {
			sb.ContactSummary = contactSummaryForAI(b.WebsiteData)
		}
		// Include email verification status
		if b.EmailVerified != nil {
			if *b.EmailVerified {
				sb.EmailVerified = "yes"
			} else {
				sb.EmailVerified = "no"
			}
		}
		// Include the per-row canonical-field presence flags so the AI
		// (and the server-side dimension cap) can reason about what
		// data was actually given vs inferred.
		sb.FieldAvailability = mergeFieldAvailability(b)
		promptBiz[i] = sb
	}

	var mktCtxStr, tradeCtxStr string
	if marketCtx != nil {
		mktCtxStr = marketCtx.AnalysisSummary
		if marketCtx.TradeData != nil {
			td := marketCtx.TradeData
			tradeCtxStr = fmt.Sprintf(
				"HS %s in %s: Import volume $%d, Export volume $%d, %d top exporters",
				td.HSCode, td.CountryCode,
				td.ImportValueUSD, td.ExportValueUSD,
				len(td.TopExporters),
			)
		}
	}

	sp := prompts.SenderPositioning{}
	if senderProfile != nil {
		sp = prompts.SenderPositioning{
			CompanyName:            senderProfile.CompanyName,
			ProductDescription:     senderProfile.ProductDescription,
			ValueProp:              senderProfile.ValueProp,
			TargetBuyerDescription: senderProfile.TargetBuyerDescription,
			TargetIndustries:       senderProfile.TargetIndustries,
			TargetCountries:        senderProfile.TargetCountries,
			AvoidCountries:         senderProfile.AvoidCountries,
			MinDealSizeUSD:         senderProfile.MinDealSizeUSD,
			TypicalDealSizeUSD:     senderProfile.TypicalDealSizeUSD,
			DealBreakers:           senderProfile.DealBreakers,
			CompetitiveMoats:       senderProfile.CompetitiveMoats,
		}
	}
	p := prompts.BuildScorePrompt(promptBiz, product.Query, product.HSCode, mktCtxStr, tradeCtxStr, sp)

	raw, aiResult, err := s.router.CompleteJSON(ctx, "scoring", p.Prompt, p.System, s.cacheTTL)
	if err != nil {
		return nil, fmt.Errorf("score AI call: %w", err)
	}

	var aiScores []scoreAIResponse
	if err := json.Unmarshal(raw, &aiScores); err != nil {
		return nil, fmt.Errorf("score parse: %w (raw: %.200s)", err, string(raw))
	}

	// Build a quick lookup from business_id back to the source business so
	// we can re-derive the FieldAvailability for the dimension cap.
	byID := make(map[uuid.UUID]domain.Business, len(batch))
	for _, b := range batch {
		byID[b.ID] = b
	}

	scores := make([]domain.LeadScore, 0, len(aiScores))
	for _, as := range aiScores {
		bizID, ok := idMap[as.ID]
		if !ok {
			continue
		}

		// No-hallucination enforcement
		if as.DataCompleteness == 0 {
			slog.Warn("scoring missing data_completeness", "business_id", as.ID)
		}

		// Apply per-dimension caps based on real input availability.
		availability := mergeFieldAvailability(byID[bizID])
		var dimComp domain.DimensionCompleteness
		var missing []string
		as, dimComp, missing = applyDimensionCaps(as, availability)

		// Recompute overall_score from sub-scores AFTER caps so the
		// weighted average reflects the clamped values.
		overall := as.DealSizePotential*0.30 +
			as.PurchaseLikelihood*0.25 +
			as.AccessibilityScore*0.25 +
			as.FitScore*0.15 +
			as.UrgencyScore*0.05
		if as.OverallScore > overall+1 {
			// AI's overall was higher than its own sub-scores would imply;
			// trust the recomputed value.
			as.OverallScore = overall
		}

		// Enforce data_completeness cap server-side (don't trust AI alone)
		overallScore := roundToInt(as.OverallScore)
		if as.DataCompleteness <= 0.2 && overallScore > 45 {
			slog.Info("scoring: capping score due to low data_completeness",
				"business_id", as.ID, "raw_score", overallScore, "capped_to", 45,
				"data_completeness", as.DataCompleteness)
			overallScore = 45
		} else if as.DataCompleteness <= 0.4 && overallScore > 60 {
			slog.Info("scoring: capping score due to low data_completeness",
				"business_id", as.ID, "raw_score", overallScore, "capped_to", 60,
				"data_completeness", as.DataCompleteness)
			overallScore = 60
		} else if as.DataCompleteness <= 0.6 && overallScore > 75 {
			slog.Info("scoring: capping score due to low data_completeness",
				"business_id", as.ID, "raw_score", overallScore, "capped_to", 75,
				"data_completeness", as.DataCompleteness)
			overallScore = 75
		}

		score := domain.LeadScore{
			ID:                    uuid.New(),
			BusinessID:            bizID,
			OverallScore:          overallScore,
			PurchaseLikelihood:    intPtr(roundToInt(as.PurchaseLikelihood)),
			DealSizePotential:     intPtr(roundToInt(as.DealSizePotential)),
			UrgencyScore:          intPtr(roundToInt(as.UrgencyScore)),
			FitScore:              intPtr(roundToInt(as.FitScore)),
			AccessibilityScore:    intPtr(roundToInt(as.AccessibilityScore)),
			DimensionCompleteness: dimComp,
			ScoringRationale:      strPtr(as.ScoringRationale),
			Strengths:             as.Strengths,
			Weaknesses:            as.Weaknesses,
			MissingFields:         missing,
			RecommendedApproach:   strPtr(as.RecommendedApproach),
			ModelVersion:          aiResult.Model,
			PromptVersion:         "v3",
			ScoredAt:              time.Now(),
		}
		scores = append(scores, score)
	}

	return scores, nil
}

type scoreAIResponse struct {
	ID                    string             `json:"id"`
	PurchaseLikelihood    float64            `json:"purchase_likelihood"`
	DealSizePotential     float64            `json:"deal_size_potential"`
	UrgencyScore          float64            `json:"urgency_score"`
	FitScore              float64            `json:"fit_score"`
	AccessibilityScore    float64            `json:"accessibility_score"`
	OverallScore          float64            `json:"overall_score"`
	ScoringRationale      string             `json:"scoring_rationale"`
	Strengths             []string           `json:"strengths"`
	Weaknesses            []string           `json:"weaknesses"`
	RecommendedApproach   string             `json:"recommended_approach"`
	DataCompleteness      float64            `json:"data_completeness"`
	DimensionCompleteness map[string]float64 `json:"dimension_completeness"`
	MissingFields         []string           `json:"missing_fields"`
}

// mergeFieldAvailability builds the per-business presence map fed to the
// AI scorer. Reads input_field_presence (set by the dynamic Excel
// importer) as the base, then promotes any field that enrichment has
// since populated (email, phone, etc.) to true.
func mergeFieldAvailability(b domain.Business) map[string]bool {
	out := map[string]bool{}
	if len(b.InputFieldPresence) > 0 {
		_ = json.Unmarshal(b.InputFieldPresence, &out)
	}
	if b.Email != nil && *b.Email != "" {
		out["consignee_email"] = true
	}
	if b.Phone != nil && *b.Phone != "" {
		out["consignee_phone"] = true
	}
	return out
}

// applyDimensionCaps clamps a dimension's sub-score when its required
// canonical fields were entirely absent (defence-in-depth on top of the
// AI's self-reporting). Returns the (possibly modified) score plus the
// final dimension_completeness map.
func applyDimensionCaps(as scoreAIResponse, availability map[string]bool) (scoreAIResponse, domain.DimensionCompleteness, []string) {
	dc := domain.DimensionCompleteness{}
	if as.DimensionCompleteness != nil {
		for k, v := range as.DimensionCompleteness {
			dc[k] = v
		}
	}
	missing := append([]string(nil), as.MissingFields...)

	dims := []struct {
		name string
		cap  float64
		ref  *float64
	}{
		{DimAccessibility, 30, &as.AccessibilityScore},
		{DimDealSize, 30, &as.DealSizePotential},
		{DimFit, 35, &as.FitScore},
		{DimUrgency, 35, &as.UrgencyScore},
	}
	for _, d := range dims {
		needed := FieldsForDimension(d.name)
		if len(needed) == 0 {
			continue
		}
		anyPresent := false
		var absent []string
		for _, k := range needed {
			if availability[k] {
				anyPresent = true
			} else {
				absent = append(absent, k)
			}
		}
		if !anyPresent {
			if *d.ref > d.cap {
				slog.Info("scoring: capping dimension due to missing inputs",
					"business_id", as.ID, "dim", d.name, "raw", *d.ref, "capped", d.cap, "missing", absent)
				*d.ref = d.cap
			}
			if dc[d.name] > 0.4 {
				dc[d.name] = 0.4
			}
			for _, k := range absent {
				if !containsString(missing, k) {
					missing = append(missing, k)
				}
			}
		}
	}
	return as, dc, missing
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func roundToInt(v float64) int { return int(math.Round(v)) }
func intPtr(v int) *int        { return &v }
func strPtr(v string) *string  { return &v }

// contactSummaryForAI builds a text summary of all contact channels from website_data for AI scoring.
func contactSummaryForAI(websiteData json.RawMessage) string {
	if websiteData == nil {
		return ""
	}

	var data struct {
		Emails         []string          `json:"emails"`
		Phones         []string          `json:"phones"`
		WhatsApp       string            `json:"whatsapp"`
		SocialLinks    map[string]string `json:"social_links"`
		Products       []string          `json:"products"`
		HasContactForm bool              `json:"has_contact_form"`
	}
	if err := json.Unmarshal(websiteData, &data); err != nil {
		return ""
	}

	var parts []string

	if len(data.Emails) > 1 {
		parts = append(parts, fmt.Sprintf("Additional emails: %s", strings.Join(data.Emails[1:], ", ")))
	}
	if len(data.Phones) > 0 {
		parts = append(parts, fmt.Sprintf("Phones from website: %s", strings.Join(data.Phones, ", ")))
	}
	if data.WhatsApp != "" {
		parts = append(parts, fmt.Sprintf("WhatsApp: %s", data.WhatsApp))
	}
	if len(data.SocialLinks) > 0 {
		var socials []string
		for name, url := range data.SocialLinks {
			socials = append(socials, name+": "+url)
		}
		parts = append(parts, fmt.Sprintf("Social: %s", strings.Join(socials, ", ")))
	}
	if data.HasContactForm {
		parts = append(parts, "Has contact form on website")
	}
	if len(data.Products) > 0 {
		max := 10
		if len(data.Products) < max {
			max = len(data.Products)
		}
		parts = append(parts, fmt.Sprintf("Products/services: %s", strings.Join(data.Products[:max], ", ")))
	}

	return strings.Join(parts, " | ")
}
