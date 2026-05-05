package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

// Repository is the discovery module's data access interface.
type Repository interface {
	CreateSearch(ctx context.Context, userID uuid.UUID, searchType string, query json.RawMessage) (*domain.Search, error)
	GetSearch(ctx context.Context, id uuid.UUID) (*domain.Search, error)
	UpdateSearchStatus(ctx context.Context, id uuid.UUID, status string, resultCount *int) error
	UpdateSearchQuery(ctx context.Context, id uuid.UUID, query json.RawMessage) error

	CreateMarket(ctx context.Context, m domain.Market) (*domain.Market, error)
	UpdateMarketProductName(ctx context.Context, marketID uuid.UUID, name string) error
	StoreBulkBusinesses(ctx context.Context, businesses []domain.Business, marketID uuid.UUID) ([]domain.Business, error)
	StoreClassifications(ctx context.Context, results []domain.ClassificationResult, marketID uuid.UUID) error
	StoreLeadScores(ctx context.Context, scores []domain.LeadScore) error
	UpdateBusinessType(ctx context.Context, id uuid.UUID, bizType, industry, subIndustry, description string) error

	GetBusinessesByMarket(ctx context.Context, marketID uuid.UUID) ([]domain.Business, error)
	ListBusinessesByMarket(ctx context.Context, marketID uuid.UUID, page, pageSize int) ([]domain.BusinessWithRelevance, int, error)
	UpdateBusinessWithPlacesData(ctx context.Context, businessID uuid.UUID, p PlacesEnrichment) error
	UpdateBusinessEmail(ctx context.Context, businessID uuid.UUID, email string) error
	UpdateBusinessDescription(ctx context.Context, businessID uuid.UUID, description string) error
	UpdateBusinessWebsiteData(ctx context.Context, businessID uuid.UUID, email, phone, description string, websiteData json.RawMessage) error
	UpdateBusinessEmailVerified(ctx context.Context, businessID uuid.UUID, verified bool) error

	// Column-mapping persistence — saves which header → canonical_key
	// mapping was confirmed for an upload. GetColumnMapping returns nil
	// when no mapping exists for that search (older uploads, or the
	// legacy auto-import path).
	SaveColumnMapping(ctx context.Context, searchID uuid.UUID, mapping map[string]string, aiConfidence map[string]float64, userOverrides map[string]string) error
	GetColumnMapping(ctx context.Context, searchID uuid.UUID) (map[string]string, error)
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) CreateSearch(ctx context.Context, userID uuid.UUID, searchType string, query json.RawMessage) (*domain.Search, error) {
	s := &domain.Search{
		ID:         uuid.New(),
		UserID:     userID,
		SearchType: searchType,
		Query:      query,
		Status:     "pending",
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO searches (id, user_id, search_type, query, status)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.UserID, s.SearchType, s.Query, s.Status,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *repository) GetSearch(ctx context.Context, id uuid.UUID) (*domain.Search, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, user_id, search_type, query, status, result_count, completed_at, task_id, created_at, updated_at
		 FROM searches WHERE id = $1`, id,
	)
	var s domain.Search
	err := row.Scan(&s.ID, &s.UserID, &s.SearchType, &s.Query, &s.Status, &s.ResultCount, &s.CompletedAt, &s.TaskID, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get search: %w", err)
	}
	return &s, nil
}

func (r *repository) UpdateSearchStatus(ctx context.Context, id uuid.UUID, status string, resultCount *int) error {
	if resultCount != nil {
		_, err := r.pool.Exec(ctx,
			`UPDATE searches SET status = $1, result_count = $2, completed_at = now(), updated_at = now() WHERE id = $3`,
			status, *resultCount, id,
		)
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE searches SET status = $1, updated_at = now() WHERE id = $2`,
		status, id,
	)
	return err
}

func (r *repository) UpdateSearchQuery(ctx context.Context, id uuid.UUID, query json.RawMessage) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE searches SET query = $1, updated_at = now() WHERE id = $2`,
		query, id,
	)
	return err
}

func (r *repository) GetBusinessesByMarket(ctx context.Context, marketID uuid.UUID) ([]domain.Business, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT b.id, b.name, b.website, b.google_place_id, b.country_code, b.city, b.address,
		        b.latitude, b.longitude, b.industry, b.sub_industry, b.business_type, b.description,
		        b.phone, b.email, b.data_source, b.enrichment_status,
		        b.rating, b.rating_count, b.google_types, b.opening_hours, b.shipment_data
		 FROM businesses b
		 JOIN business_markets bm ON b.id = bm.business_id
		 WHERE bm.market_id = $1`,
		marketID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var businesses []domain.Business
	for rows.Next() {
		var b domain.Business
		err := rows.Scan(
			&b.ID, &b.Name, &b.Website, &b.GooglePlaceID, &b.CountryCode, &b.City, &b.Address,
			&b.Latitude, &b.Longitude, &b.Industry, &b.SubIndustry, &b.BusinessType, &b.Description,
			&b.Phone, &b.Email, &b.DataSource, &b.EnrichmentStatus,
			&b.Rating, &b.RatingCount, &b.GoogleTypes, &b.OpeningHours, &b.ShipmentData,
		)
		if err != nil {
			return nil, fmt.Errorf("scan business: %w", err)
		}
		businesses = append(businesses, b)
	}
	return businesses, nil
}

func (r *repository) CreateMarket(ctx context.Context, m domain.Market) (*domain.Market, error) {
	// Each upload creates its own market — no reuse.
	m.ID = uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO markets (id, name, country_code, city,
			dominant_hs_code, all_hs_codes, origin_country, origin_countries, origin_share,
			shipment_from_date, shipment_to_date, importer_count, shipment_count,
			source_file_name, uploaded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		m.ID, m.Name, m.CountryCode, m.City,
		m.DominantHSCode, m.AllHSCodes, m.OriginCountry, m.OriginCountries, m.OriginShare,
		m.ShipmentFromDate, m.ShipmentToDate, m.ImporterCount, m.ShipmentCount,
		m.SourceFileName, m.UploadedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create market: %w", err)
	}
	return &m, nil
}

// UpdateMarketProductName sets the AI-derived product name on a market.
func (r *repository) UpdateMarketProductName(ctx context.Context, marketID uuid.UUID, name string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE markets SET derived_product_name = $1, updated_at = now() WHERE id = $2`,
		name, marketID,
	)
	return err
}

func (r *repository) StoreBulkBusinesses(ctx context.Context, businesses []domain.Business, marketID uuid.UUID) ([]domain.Business, error) {
	stored := make([]domain.Business, 0, len(businesses))

	for i := range businesses {
		b := &businesses[i]

		// Check for existing by google_place_id
		if b.GooglePlaceID != nil {
			row := r.pool.QueryRow(ctx,
				`SELECT id FROM businesses WHERE google_place_id = $1`, *b.GooglePlaceID)
			var existingID uuid.UUID
			if err := row.Scan(&existingID); err == nil {
				b.ID = existingID
				r.linkBusinessToMarket(ctx, b.ID, marketID, b.DataSource)
				stored = append(stored, *b)
				continue
			}
		}

		// Check for existing by name + country (dedup for Excel imports without google_place_id)
		if b.GooglePlaceID == nil && b.CountryCode != nil {
			row := r.pool.QueryRow(ctx,
				`SELECT id FROM businesses WHERE name = $1 AND country_code = $2`, b.Name, *b.CountryCode)
			var existingID uuid.UUID
			if err := row.Scan(&existingID); err == nil {
				b.ID = existingID
				r.linkBusinessToMarket(ctx, b.ID, marketID, b.DataSource)
				stored = append(stored, *b)
				continue
			}
		}

		// Insert new business
		if b.ID == uuid.Nil {
			b.ID = uuid.New()
		}

		shipmentData := b.ShipmentData
		if shipmentData == nil {
			shipmentData = json.RawMessage("null")
		}
		googleTypes := b.GoogleTypes
		if googleTypes == nil {
			googleTypes = json.RawMessage("null")
		}
		openingHours := b.OpeningHours
		if openingHours == nil {
			openingHours = json.RawMessage("null")
		}

		presence := b.InputFieldPresence
		if presence == nil {
			presence = json.RawMessage(`{}`)
		}

		var query string
		if b.GooglePlaceID != nil {
			query = `INSERT INTO businesses (id, name, website, google_place_id, country_code, city, address,
			 latitude, longitude, phone, email, business_type, description, data_source, enrichment_status,
			 shipment_data, rating, rating_count, google_types, opening_hours, input_field_presence)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
			 ON CONFLICT (google_place_id) DO UPDATE SET updated_at = now()`
		} else {
			query = `INSERT INTO businesses (id, name, website, google_place_id, country_code, city, address,
			 latitude, longitude, phone, email, business_type, description, data_source, enrichment_status,
			 shipment_data, rating, rating_count, google_types, opening_hours, input_field_presence)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`
		}
		_, err := r.pool.Exec(ctx, query,
			b.ID, b.Name, b.Website, b.GooglePlaceID, b.CountryCode, b.City, b.Address,
			b.Latitude, b.Longitude, b.Phone, b.Email, b.BusinessType, b.Description,
			b.DataSource, b.EnrichmentStatus, shipmentData,
			b.Rating, b.RatingCount, googleTypes, openingHours, presence,
		)
		if err != nil {
			slog.Error("store business failed", "name", b.Name, "error", err)
			continue
		}

		r.linkBusinessToMarket(ctx, b.ID, marketID, b.DataSource)
		stored = append(stored, *b)
	}

	return stored, nil
}

func (r *repository) linkBusinessToMarket(ctx context.Context, businessID, marketID uuid.UUID, dataSource *string) {
	via := "google_places"
	if dataSource != nil {
		via = *dataSource
	}
	_, _ = r.pool.Exec(ctx,
		`INSERT INTO business_markets (business_id, market_id, discovered_via)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (business_id, market_id) DO NOTHING`,
		businessID, marketID, via,
	)
}

func (r *repository) StoreClassifications(ctx context.Context, results []domain.ClassificationResult, marketID uuid.UUID) error {
	for _, cr := range results {
		_, err := r.pool.Exec(ctx,
			`UPDATE business_markets SET relevance_score = $1
			 WHERE business_id = $2 AND market_id = $3`,
			cr.RelevanceScore, cr.BusinessID, marketID,
		)
		if err != nil {
			slog.Error("store classification failed", "business_id", cr.BusinessID, "error", err)
		}
	}
	return nil
}

func (r *repository) StoreLeadScores(ctx context.Context, scores []domain.LeadScore) error {
	for _, s := range scores {
		dimJSON, _ := domain.MarshalDimensionCompleteness(s.DimensionCompleteness)
		_, err := r.pool.Exec(ctx,
			`INSERT INTO lead_scores (id, business_id, market_id, user_id, overall_score,
			 purchase_likelihood, deal_size_potential, urgency_score, fit_score, accessibility_score,
			 scoring_rationale, strengths, weaknesses, recommended_approach,
			 model_version, prompt_version, scored_at, dimension_completeness)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
			 ON CONFLICT (business_id, market_id, user_id) DO UPDATE SET
			   overall_score = EXCLUDED.overall_score,
			   purchase_likelihood = EXCLUDED.purchase_likelihood,
			   deal_size_potential = EXCLUDED.deal_size_potential,
			   urgency_score = EXCLUDED.urgency_score,
			   fit_score = EXCLUDED.fit_score,
			   accessibility_score = EXCLUDED.accessibility_score,
			   scoring_rationale = EXCLUDED.scoring_rationale,
			   strengths = EXCLUDED.strengths,
			   weaknesses = EXCLUDED.weaknesses,
			   recommended_approach = EXCLUDED.recommended_approach,
			   model_version = EXCLUDED.model_version,
			   prompt_version = EXCLUDED.prompt_version,
			   scored_at = EXCLUDED.scored_at,
			   dimension_completeness = EXCLUDED.dimension_completeness`,
			s.ID, s.BusinessID, s.MarketID, s.UserID, s.OverallScore,
			s.PurchaseLikelihood, s.DealSizePotential, s.UrgencyScore, s.FitScore, s.AccessibilityScore,
			s.ScoringRationale, s.Strengths, s.Weaknesses, s.RecommendedApproach,
			s.ModelVersion, s.PromptVersion, s.ScoredAt, dimJSON,
		)
		if err != nil {
			slog.Error("store lead score failed", "business_id", s.BusinessID, "error", err)
		}
	}
	return nil
}

func (r *repository) UpdateBusinessType(ctx context.Context, id uuid.UUID, bizType, industry, subIndustry, description string) error {
	// Don't downgrade confirmed trust tiers from trade data with AI classification
	_, err := r.pool.Exec(ctx,
		`UPDATE businesses SET
		 business_type = CASE WHEN business_type IN ('confirmed_buyer', 'confirmed_importer') THEN business_type ELSE $1 END,
		 industry = $2, sub_industry = $3, description = $4, updated_at = now()
		 WHERE id = $5`,
		bizType, industry, subIndustry, description, id,
	)
	return err
}

// PlacesEnrichment holds Google Places data to merge into a Tendata business.
type PlacesEnrichment struct {
	GooglePlaceID string
	Website       string
	Phone         string
	Email         string
	Latitude      float64
	Longitude     float64
	Description   string
	Rating        float64
	RatingCount   int
	GoogleTypes   json.RawMessage
	OpeningHours  json.RawMessage
}

func (r *repository) UpdateBusinessWithPlacesData(ctx context.Context, businessID uuid.UUID, p PlacesEnrichment) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE businesses SET
		 google_place_id = $1,
		 website = COALESCE(website, $2),
		 phone = COALESCE(phone, $3),
		 email = COALESCE(email, $4),
		 latitude = $5,
		 longitude = $6,
		 description = COALESCE(description, $7),
		 rating = $8,
		 rating_count = $9,
		 google_types = $10,
		 opening_hours = $11,
		 data_source = 'tendata_verified',
		 enrichment_status = 'enriched',
		 updated_at = now()
		 WHERE id = $12`,
		p.GooglePlaceID, p.Website, p.Phone, p.Email,
		p.Latitude, p.Longitude, p.Description,
		p.Rating, p.RatingCount, p.GoogleTypes, p.OpeningHours,
		businessID,
	)
	if err != nil {
		return fmt.Errorf("update business with places data: %w", err)
	}
	return nil
}

func (r *repository) UpdateBusinessEmail(ctx context.Context, businessID uuid.UUID, email string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE businesses SET email = $1, updated_at = now() WHERE id = $2 AND (email IS NULL OR email = '')`,
		email, businessID,
	)
	return err
}

func (r *repository) UpdateBusinessDescription(ctx context.Context, businessID uuid.UUID, description string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE businesses SET description = $1, updated_at = now() WHERE id = $2 AND (description IS NULL OR description = '')`,
		description, businessID,
	)
	return err
}

// UpdateBusinessWebsiteData stores the enriched website scraping results and updates contact info.
func (r *repository) UpdateBusinessWebsiteData(ctx context.Context, businessID uuid.UUID, email, phone, description string, websiteData json.RawMessage) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE businesses SET
		 email = COALESCE(NULLIF(email, ''), NULLIF($1, '')),
		 phone = COALESCE(NULLIF(phone, ''), NULLIF($2, '')),
		 description = COALESCE(NULLIF(description, ''), NULLIF($3, '')),
		 website_data = $4,
		 updated_at = now()
		 WHERE id = $5`,
		email, phone, description, websiteData, businessID,
	)
	return err
}

// UpdateBusinessEmailVerified marks whether an email address passed SMTP verification.
func (r *repository) UpdateBusinessEmailVerified(ctx context.Context, businessID uuid.UUID, verified bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE businesses SET email_verified = $1, updated_at = now() WHERE id = $2`,
		verified, businessID,
	)
	return err
}

func (r *repository) ListBusinessesByMarket(ctx context.Context, marketID uuid.UUID, page, pageSize int) ([]domain.BusinessWithRelevance, int, error) {
	offset := (page - 1) * pageSize

	// Count
	var total int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM business_markets WHERE market_id = $1`, marketID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT b.id, b.name, b.website, b.google_place_id, b.country_code, b.city, b.address,
		        b.latitude, b.longitude, b.industry, b.sub_industry, b.business_type, b.description,
		        b.phone, b.email, b.data_source, b.enrichment_status, b.created_at, b.updated_at,
		        b.shipment_data, b.rating, b.rating_count, b.google_types,
		        bm.relevance_score, bm.discovered_via,
		        ls.overall_score, ls.purchase_likelihood, ls.deal_size_potential,
		        ls.urgency_score, ls.fit_score, ls.accessibility_score,
		        ls.scoring_rationale, ls.strengths, ls.weaknesses, ls.recommended_approach,
		        ls.dimension_completeness
		 FROM businesses b
		 JOIN business_markets bm ON b.id = bm.business_id
		 LEFT JOIN lead_scores ls ON ls.business_id = b.id AND ls.market_id = bm.market_id
		 WHERE bm.market_id = $1
		 ORDER BY ls.overall_score DESC NULLS LAST, bm.relevance_score DESC NULLS LAST, b.name ASC
		 LIMIT $2 OFFSET $3`,
		marketID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []domain.BusinessWithRelevance
	for rows.Next() {
		var bwr domain.BusinessWithRelevance
		var dimRaw []byte
		err := rows.Scan(
			&bwr.ID, &bwr.Name, &bwr.Website, &bwr.GooglePlaceID, &bwr.CountryCode, &bwr.City, &bwr.Address,
			&bwr.Latitude, &bwr.Longitude, &bwr.Industry, &bwr.SubIndustry, &bwr.BusinessType, &bwr.Description,
			&bwr.Phone, &bwr.Email, &bwr.DataSource, &bwr.EnrichmentStatus, &bwr.CreatedAt, &bwr.UpdatedAt,
			&bwr.ShipmentData, &bwr.Rating, &bwr.RatingCount, &bwr.GoogleTypes,
			&bwr.RelevanceScore, &bwr.DiscoveredVia,
			&bwr.OverallScore, &bwr.PurchaseLikelihood, &bwr.DealSizePotential,
			&bwr.UrgencyScore, &bwr.FitScore, &bwr.AccessibilityScore,
			&bwr.ScoringRationale, &bwr.Strengths, &bwr.Weaknesses, &bwr.RecommendedApproach,
			&dimRaw,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan business: %w", err)
		}
		if len(dimRaw) > 0 {
			_ = json.Unmarshal(dimRaw, &bwr.DimensionCompleteness)
		}
		// Compute trust tier from data source + business type
		ds := ""
		if bwr.DataSource != nil {
			ds = *bwr.DataSource
		}
		bt := ""
		if bwr.BusinessType != nil {
			bt = *bwr.BusinessType
		}
		bwr.TrustTier = domain.BusinessTrustTier(ds, bt)
		results = append(results, bwr)
	}

	return results, total, nil
}

// SaveColumnMapping records which header → canonical mapping the user
// confirmed for this upload. Idempotent (PRIMARY KEY on search_id);
// re-running for the same search overwrites.
func (r *repository) SaveColumnMapping(ctx context.Context, searchID uuid.UUID, mapping map[string]string, aiConfidence map[string]float64, userOverrides map[string]string) error {
	if mapping == nil {
		mapping = map[string]string{}
	}
	if aiConfidence == nil {
		aiConfidence = map[string]float64{}
	}
	if userOverrides == nil {
		userOverrides = map[string]string{}
	}
	mappingJSON, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal mapping: %w", err)
	}
	confJSON, err := json.Marshal(aiConfidence)
	if err != nil {
		return fmt.Errorf("marshal ai_confidence: %w", err)
	}
	overridesJSON, err := json.Marshal(userOverrides)
	if err != nil {
		return fmt.Errorf("marshal user_overrides: %w", err)
	}

	// Build the unmapped[] from mapping keys not present (server-side
	// truth, derived from the input).
	unmappedJSON := []byte("[]")

	_, err = r.pool.Exec(ctx,
		`INSERT INTO search_column_mappings (search_id, mapping, ai_confidence, user_overrides, unmapped)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (search_id) DO UPDATE SET
		   mapping = EXCLUDED.mapping,
		   ai_confidence = EXCLUDED.ai_confidence,
		   user_overrides = EXCLUDED.user_overrides,
		   unmapped = EXCLUDED.unmapped`,
		searchID, mappingJSON, confJSON, overridesJSON, unmappedJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert column mapping: %w", err)
	}
	return nil
}

func (r *repository) GetColumnMapping(ctx context.Context, searchID uuid.UUID) (map[string]string, error) {
	var raw json.RawMessage
	err := r.pool.QueryRow(ctx,
		`SELECT mapping FROM search_column_mappings WHERE search_id = $1`,
		searchID,
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get column mapping: %w", err)
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode mapping: %w", err)
	}
	return out, nil
}

// suppress unused-import warning in dev builds where slog isn't yet used
// in the new methods (kept here so future maintenance can easily add
// instrumentation without re-importing).
var _ = slog.Default

