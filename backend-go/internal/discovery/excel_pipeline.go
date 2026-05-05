package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// ExcelSearchState is the new SearchState shape — slim, Excel-only.
// Lives in search.query JSON.
type ExcelSearchState struct {
	MarketIDs      []uuid.UUID `json:"market_ids,omitempty"`
	SourceFileName string      `json:"source_file_name,omitempty"`
	UploadedAt     time.Time   `json:"uploaded_at,omitempty"`
	Warnings       []string    `json:"warnings,omitempty"`
}

// ProcessResult is the sync response after Excel parsing and market creation.
type ProcessResult struct {
	Markets       []MarketSummary `json:"markets"`
	TotalImporters int            `json:"total_importers"`
	TotalShipments int            `json:"total_shipments"`
	Warnings      []string        `json:"warnings,omitempty"`
}

// MarketSummary is a lightweight view of a created market returned inline after upload.
type MarketSummary struct {
	MarketID           uuid.UUID `json:"market_id"`
	DestinationCountry string    `json:"destination_country"`
	DominantHSCode     string    `json:"dominant_hs_code"`
	OriginCountry      string    `json:"origin_country"`
	OriginShare        float64   `json:"origin_share"`
	ImporterCount      int       `json:"importer_count"`
	ShipmentCount      int       `json:"shipment_count"`
}

// ProcessExcel is the sync portion of the new flow. Parses the Excel, splits
// records by destination country, creates one market per destination with
// derived metadata, and stores importers as businesses. Spawns one goroutine
// per market to run EnrichAndScore asynchronously.
func (p *Pipeline) ProcessExcel(
	ctx context.Context,
	searchID, userID uuid.UUID,
	excelReader io.Reader,
	fileName string,
) (*ProcessResult, error) {
	return p.ProcessExcelWithMapping(ctx, searchID, userID, excelReader, fileName, nil)
}

// ProcessExcelWithMapping is the new entry point that accepts a
// user-confirmed header → canonical mapping. When mapping is nil we fall
// back to the legacy fixed-header parser so existing tests/CLI uploads
// keep working. PR3 wires the dynamic parser; for now we accept the
// mapping argument so the public API surface is stable from PR2 onward.
func (p *Pipeline) ProcessExcelWithMapping(
	ctx context.Context,
	searchID, userID uuid.UUID,
	excelReader io.Reader,
	fileName string,
	mapping map[string]string,
) (*ProcessResult, error) {
	slog.Info("pipeline: processing excel upload", "search_id", searchID, "file", fileName, "dynamic_mapping", mapping != nil)

	var (
		records   []ShipmentRecord
		totalRows int
		skipped   int
		err       error
	)
	if mapping != nil && len(mapping) > 0 {
		records, totalRows, skipped, err = ParseExcelWithMapping(excelReader, mapping)
	} else {
		records, totalRows, skipped, err = ParseTendataExcelRaw(excelReader)
	}
	if err != nil {
		return nil, fmt.Errorf("parse excel: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("no usable rows in excel (total=%d, skipped=%d)", totalRows, skipped)
	}

	groups := SplitByDestination(records)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no destination countries found in excel")
	}

	uploadedAt := time.Now()
	var warnings []string
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("%d rows skipped (missing consignee name)", skipped))
	}
	if len(groups) > 1 {
		warnings = append(warnings, fmt.Sprintf("Excel contains shipments to %d countries; one market created per destination", len(groups)))
	}

	result := &ProcessResult{
		Markets:       make([]MarketSummary, 0, len(groups)),
		TotalImporters: 0,
		TotalShipments: 0,
		Warnings:      warnings,
	}

	var marketIDs []uuid.UUID

	for _, group := range groups {
		if group.DestinationCountry == "ZZ" {
			warnings = append(warnings, fmt.Sprintf("%d rows skipped (destination country missing or unrecognized)", len(group.Records)))
			continue
		}
		meta := DeriveMarketMetadata(group)
		if meta.ImporterCount == 0 {
			continue
		}

		// Placeholder name — replaced by AI-derived product name during enrichment.
		// Kept concise so the card header looks clean even if AI fails.
		marketName := fmt.Sprintf("%s — %s", displayHS(meta.DominantHSCode), meta.DestinationCountry)

		shipFrom := meta.ShipmentFromDate
		shipTo := meta.ShipmentToDate
		var shipFromPtr, shipToPtr *time.Time
		if !shipFrom.IsZero() {
			shipFromPtr = &shipFrom
		}
		if !shipTo.IsZero() {
			shipToPtr = &shipTo
		}

		originShare := meta.OriginShare
		importerCount := meta.ImporterCount
		shipmentCount := meta.ShipmentCount

		m := domain.Market{
			Name:             marketName,
			CountryCode:      meta.DestinationCountry,
			DominantHSCode:   strPtrIfNotEmpty(meta.DominantHSCode),
			AllHSCodes:       meta.AllHSCodes,
			OriginCountry:    strPtrIfNotEmpty(meta.OriginCountry),
			OriginCountries:  meta.AllOriginCountries,
			OriginShare:      &originShare,
			ShipmentFromDate: shipFromPtr,
			ShipmentToDate:   shipToPtr,
			ImporterCount:    &importerCount,
			ShipmentCount:    &shipmentCount,
			SourceFileName:   strPtrIfNotEmpty(fileName),
			UploadedAt:       &uploadedAt,
		}

		created, err := p.repo.CreateMarket(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("create market for %s: %w", meta.DestinationCountry, err)
		}

		// Aggregate this group's records into businesses (per-consignee roll-up)
		importers := AggregateRecords(group.Records, "")
		businesses := aggregationsToBusinesses(importers, meta.DestinationCountry)
		if _, err := p.repo.StoreBulkBusinesses(ctx, businesses, created.ID); err != nil {
			return nil, fmt.Errorf("store businesses for %s: %w", meta.DestinationCountry, err)
		}

		result.Markets = append(result.Markets, MarketSummary{
			MarketID:           created.ID,
			DestinationCountry: meta.DestinationCountry,
			DominantHSCode:     meta.DominantHSCode,
			OriginCountry:      meta.OriginCountry,
			OriginShare:        meta.OriginShare,
			ImporterCount:      meta.ImporterCount,
			ShipmentCount:      meta.ShipmentCount,
		})
		result.TotalImporters += meta.ImporterCount
		result.TotalShipments += meta.ShipmentCount

		marketIDs = append(marketIDs, created.ID)

		// Capture meta for the async phase
		go p.runEnrichAndScore(userID, searchID, created.ID, meta)
	}

	// Propagate warnings we collected during the loop
	result.Warnings = warnings

	if len(marketIDs) == 0 {
		return nil, fmt.Errorf("no usable shipments after parsing: %v", warnings)
	}

	// Persist search state
	state := ExcelSearchState{
		MarketIDs:      marketIDs,
		SourceFileName: fileName,
		UploadedAt:     uploadedAt,
		Warnings:       warnings,
	}
	stateJSON, _ := json.Marshal(state)
	if err := p.repo.UpdateSearchQuery(ctx, searchID, stateJSON); err != nil {
		slog.Error("pipeline: save search state", "error", err)
	}
	_ = p.repo.UpdateSearchStatus(ctx, searchID, "enriching", nil)

	// Track per-upload completion using a wrapper goroutine
	go p.trackCompletion(searchID, marketIDs)

	return result, nil
}

// runEnrichAndScore is the per-market async worker.
// Uses a background context since the request ctx will expire after ProcessExcel returns.
func (p *Pipeline) runEnrichAndScore(userID, searchID, marketID uuid.UUID, meta MarketMetadata) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	ctx = injectContext(ctx, userID, searchID)

	defer func() {
		if r := recover(); r != nil {
			slog.Error("pipeline: enrich panic", "market_id", marketID, "panic", r)
			p.markMarketDone(marketID)
		}
	}()

	if err := p.EnrichAndScore(ctx, userID, marketID, meta); err != nil {
		slog.Error("pipeline: enrich failed", "market_id", marketID, "error", err)
	}
	p.markMarketDone(marketID)
}

// EnrichAndScore processes a single market asynchronously:
// 1. AI-derived product name
// 2. Google Places verification
// 3. Website scraping
// 4. Classification
// 5. Scoring
func (p *Pipeline) EnrichAndScore(
	ctx context.Context,
	userID, marketID uuid.UUID,
	meta MarketMetadata,
) error {
	slog.Info("pipeline: enriching market", "market_id", marketID, "destination", meta.DestinationCountry)

	// Step 1: AI product name (grounded summarization)
	productName := p.deriveProductName(ctx, meta)
	if productName != "" {
		if err := p.repo.UpdateMarketProductName(ctx, marketID, productName); err != nil {
			slog.Warn("pipeline: update product name failed", "market_id", marketID, "error", err)
		}
	}

	businesses, err := p.repo.GetBusinessesByMarket(ctx, marketID)
	if err != nil {
		return fmt.Errorf("get businesses: %w", err)
	}
	if len(businesses) == 0 {
		return nil
	}

	// Step 2: Google Places verification
	if p.places != nil {
		if _, err := p.verifyWithGooglePlaces(ctx, businesses, meta.DestinationCountry); err != nil {
			slog.Error("pipeline: places verification failed", "market_id", marketID, "error", err)
		}
		businesses, err = p.repo.GetBusinessesByMarket(ctx, marketID)
		if err != nil {
			return fmt.Errorf("reload businesses: %w", err)
		}
	}

	// Step 3: Website scraping
	p.scrapeEmails(ctx, businesses)

	// Step 4: Classification (uses derived product + HS code)
	product := domain.ProductContext{
		Query:       productName,
		HSCode:      meta.DominantHSCode,
		CountryCode: meta.DestinationCountry,
	}
	classResults, err := p.classifier.ClassifyBatch(ctx, businesses, product)
	if err != nil {
		slog.Error("pipeline: classification failed", "market_id", marketID, "error", err)
	}
	if len(classResults) > 0 {
		if err := p.repo.StoreClassifications(ctx, classResults, marketID); err != nil {
			slog.Error("pipeline: store classifications", "error", err)
		}
		for _, cr := range classResults {
			_ = p.repo.UpdateBusinessType(ctx, cr.BusinessID, cr.BusinessType, cr.Industry, cr.SubIndustry, cr.Description)
		}
	}

	// Step 5: Scoring (marketCtx=nil since Comtrade is removed)
	scores, err := p.scorer.ScoreBatch(ctx, businesses, product, nil)
	if err != nil {
		slog.Error("pipeline: scoring failed", "market_id", marketID, "error", err)
	}
	if len(scores) > 0 {
		for i := range scores {
			scores[i].MarketID = marketID
			scores[i].UserID = userID
		}
		if err := p.repo.StoreLeadScores(ctx, scores); err != nil {
			slog.Error("pipeline: store scores", "error", err)
		}
	}

	slog.Info("pipeline: market enriched",
		"market_id", marketID,
		"businesses", len(businesses),
		"classified", len(classResults),
		"scored", len(scores),
	)
	return nil
}

// deriveProductName calls the AI with grounded product descriptions + HS context.
// Returns "" on failure — caller should keep market without a derived name.
func (p *Pipeline) deriveProductName(ctx context.Context, meta MarketMetadata) string {
	if len(meta.TopProductDescs) == 0 {
		return ""
	}
	prompt := prompts.BuildProductNamePrompt(prompts.ProductNameInput{
		DominantHSCode:     meta.DominantHSCode,
		HSDescription:      "", // Lookup table TODO
		DominantOrigin:     meta.OriginCountry,
		OriginShare:        meta.OriginShare,
		AllOriginCountries: meta.AllOriginCountries,
		ProductDescs:       meta.TopProductDescs,
	})
	raw, _, err := p.router.CompleteJSON(ctx, "product_name", prompt.Prompt, prompt.System, 24*time.Hour)
	if err != nil {
		slog.Warn("pipeline: product name AI failed", "error", err)
		return ""
	}
	var result prompts.ProductNameResult
	if err := json.Unmarshal(raw, &result); err != nil {
		slog.Warn("pipeline: product name parse failed", "error", err)
		return ""
	}
	return result.ProductName
}

// ---- completion tracking ----

var (
	completionMu    sync.Mutex
	pendingMarkets  = make(map[uuid.UUID]map[uuid.UUID]bool) // searchID → marketID → done
	searchForMarket = make(map[uuid.UUID]uuid.UUID)          // marketID → searchID
)

func (p *Pipeline) trackCompletion(searchID uuid.UUID, marketIDs []uuid.UUID) {
	completionMu.Lock()
	pendingMarkets[searchID] = make(map[uuid.UUID]bool)
	for _, mid := range marketIDs {
		pendingMarkets[searchID][mid] = false
		searchForMarket[mid] = searchID
	}
	completionMu.Unlock()
}

func (p *Pipeline) markMarketDone(marketID uuid.UUID) {
	completionMu.Lock()
	searchID, ok := searchForMarket[marketID]
	if !ok {
		completionMu.Unlock()
		return
	}
	pendingMarkets[searchID][marketID] = true
	allDone := true
	for _, done := range pendingMarkets[searchID] {
		if !done {
			allDone = false
			break
		}
	}
	if allDone {
		delete(pendingMarkets, searchID)
		for mid := range pendingMarkets[searchID] {
			delete(searchForMarket, mid)
		}
	}
	completionMu.Unlock()

	if allDone {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Compute total result count across all markets
		count := p.countLeadsForSearch(ctx, searchID)
		if err := p.repo.UpdateSearchStatus(ctx, searchID, "completed", &count); err != nil {
			slog.Error("pipeline: mark search completed", "search_id", searchID, "error", err)
		} else {
			slog.Info("pipeline: search completed", "search_id", searchID, "total_leads", count)
		}
	}
}

func (p *Pipeline) countLeadsForSearch(ctx context.Context, searchID uuid.UUID) int {
	search, err := p.repo.GetSearch(ctx, searchID)
	if err != nil || search == nil {
		return 0
	}
	var state ExcelSearchState
	if err := json.Unmarshal(search.Query, &state); err != nil {
		return 0
	}
	total := 0
	for _, mid := range state.MarketIDs {
		businesses, err := p.repo.GetBusinessesByMarket(ctx, mid)
		if err == nil {
			total += len(businesses)
		}
	}
	return total
}

// displayHS returns a safe HS display string.
func displayHS(hs string) string {
	if hs == "" {
		return "unknown"
	}
	return "HS " + hs
}

// aggregationsToBusinesses converts per-consignee aggregations to Business domain records.
// Uses ShipmentData JSONB column for customs metadata; populates
// InputFieldPresence + Email when the dynamic importer collected them.
func aggregationsToBusinesses(importers []ShipmentAggregation, countryCode string) []domain.Business {
	out := make([]domain.Business, 0, len(importers))
	for _, imp := range importers {
		ds := "tendata_import"
		bt := "confirmed_importer"
		b := domain.Business{
			ID:                  uuid.New(),
			Name:                imp.CompanyName,
			CountryCode:         strPtrIfNotEmpty(countryCode),
			City:                strPtrIfNotEmpty(imp.City),
			Address:             strPtrIfNotEmpty(imp.Address),
			Phone:               strPtrIfNotEmpty(imp.Phone),
			Email:               strPtrIfNotEmpty(imp.Email),
			DataSource:          &ds,
			BusinessType:        &bt,
			EnrichmentStatus:    "pending",
			ShipmentData:        shipmentDataJSON(imp),
			InputFieldPresence:  presenceJSON(imp.FieldPresence),
		}
		out = append(out, b)
	}
	return out
}

// presenceJSON marshals the per-importer canonical-field presence map.
// Always returns a non-empty JSON object so the JSONB column never sees
// NULL — matches the migration default of '{}'.
func presenceJSON(p map[string]bool) json.RawMessage {
	if p == nil {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(p)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// injectContext attaches userID/searchID to ctx for AI logging.
func injectContext(ctx context.Context, userID, searchID uuid.UUID) context.Context {
	ctx = ai.WithUserID(ctx, userID)
	ctx = ai.WithSearchID(ctx, searchID)
	return ctx
}
