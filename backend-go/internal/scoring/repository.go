package scoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/batuhan/marketintel/internal/domain"
)

// Repository is the scoring module's data access interface.
type Repository interface {
	// Markets
	GetMarket(ctx context.Context, id uuid.UUID) (*domain.Market, error)
	ListMarkets(ctx context.Context) ([]domain.Market, error)
	UpdateMarketScores(ctx context.Context, id uuid.UUID, demandScore, saturationScore float64, estimatedSize int64) error

	// Market Analysis

	// Rankings
	UpsertRanking(ctx context.Context, r *domain.MarketRanking) error
	ListRankings(ctx context.Context, userID, categoryID uuid.UUID) ([]domain.MarketRanking, error)

	DeleteMarket(ctx context.Context, id uuid.UUID) error

	// Leads
	ListLeads(ctx context.Context, marketID, userID uuid.UUID, minScore, page, pageSize int) ([]domain.BusinessWithRelevance, int, error)

	// Manual contact edits on a business (user fixes an email/phone scraper missed).
	UpdateBusinessContact(ctx context.Context, businessID uuid.UUID, email, phone, website *string) error
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) GetMarket(ctx context.Context, id uuid.UUID) (*domain.Market, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, name, country_code, region, city, latitude, longitude, radius_km,
		        product_category_id, estimated_market_size, saturation_score, demand_score,
		        last_analyzed_at, derived_product_name, dominant_hs_code, all_hs_codes,
		        origin_country, origin_countries, origin_share,
		        shipment_from_date, shipment_to_date, importer_count, shipment_count,
		        source_file_name, uploaded_at, created_at, updated_at
		 FROM markets WHERE id = $1`, id,
	)
	var m domain.Market
	err := row.Scan(&m.ID, &m.Name, &m.CountryCode, &m.Region, &m.City,
		&m.Latitude, &m.Longitude, &m.RadiusKM, &m.ProductCategoryID,
		&m.EstimatedMarketSize, &m.SaturationScore, &m.DemandScore,
		&m.LastAnalyzedAt, &m.DerivedProductName, &m.DominantHSCode, &m.AllHSCodes,
		&m.OriginCountry, &m.OriginCountries, &m.OriginShare,
		&m.ShipmentFromDate, &m.ShipmentToDate, &m.ImporterCount, &m.ShipmentCount,
		&m.SourceFileName, &m.UploadedAt, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get market: %w", err)
	}
	return &m, nil
}

func (r *repository) ListMarkets(ctx context.Context) ([]domain.Market, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT m.id, m.name, m.country_code, m.region, m.city, m.latitude, m.longitude, m.radius_km,
		        m.product_category_id, m.estimated_market_size, m.saturation_score, m.demand_score,
		        m.last_analyzed_at, m.derived_product_name, m.dominant_hs_code, m.all_hs_codes,
		        m.origin_country, m.origin_countries, m.origin_share,
		        m.shipment_from_date, m.shipment_to_date, m.importer_count, m.shipment_count,
		        m.source_file_name, m.uploaded_at, m.created_at, m.updated_at
		 FROM markets m
		 WHERE EXISTS (
		   SELECT 1 FROM searches s
		   WHERE s.query @> jsonb_build_object('market_ids', jsonb_build_array(m.id::text))
		 )
		 ORDER BY m.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var markets []domain.Market
	for rows.Next() {
		var m domain.Market
		if err := rows.Scan(&m.ID, &m.Name, &m.CountryCode, &m.Region, &m.City,
			&m.Latitude, &m.Longitude, &m.RadiusKM, &m.ProductCategoryID,
			&m.EstimatedMarketSize, &m.SaturationScore, &m.DemandScore,
			&m.LastAnalyzedAt, &m.DerivedProductName, &m.DominantHSCode, &m.AllHSCodes,
			&m.OriginCountry, &m.OriginCountries, &m.OriginShare,
			&m.ShipmentFromDate, &m.ShipmentToDate, &m.ImporterCount, &m.ShipmentCount,
			&m.SourceFileName, &m.UploadedAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		markets = append(markets, m)
	}
	return markets, nil
}

func (r *repository) UpdateMarketScores(ctx context.Context, id uuid.UUID, demandScore, saturationScore float64, estimatedSize int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE markets SET demand_score = $1, saturation_score = $2, estimated_market_size = $3,
		 last_analyzed_at = now(), updated_at = now()
		 WHERE id = $4`,
		decimal.NewFromFloat(demandScore), decimal.NewFromFloat(saturationScore),
		decimal.NewFromInt(estimatedSize), id,
	)
	return err
}

func (r *repository) DeleteMarket(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, table := range []string{"lead_scores", "market_analyses", "competitors", "business_markets"} {
		if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE market_id = $1", table), id); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	// Also clean up market_rankings that reference this market's country
	if _, err := tx.Exec(ctx, "DELETE FROM markets WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete market: %w", err)
	}

	return tx.Commit(ctx)
}

// UpdateBusinessContact lets the user manually fix email/phone/website on a
// business when the scraper missed them. Passing nil keeps existing value;
// passing pointer-to-empty-string clears the field.
func (r *repository) UpdateBusinessContact(ctx context.Context, businessID uuid.UUID, email, phone, website *string) error {
	sets := []string{}
	args := []any{}
	idx := 1
	if email != nil {
		sets = append(sets, fmt.Sprintf("email = $%d", idx))
		args = append(args, nullIfEmpty(*email))
		idx++
	}
	if phone != nil {
		sets = append(sets, fmt.Sprintf("phone = $%d", idx))
		args = append(args, nullIfEmpty(*phone))
		idx++
	}
	if website != nil {
		sets = append(sets, fmt.Sprintf("website = $%d", idx))
		args = append(args, nullIfEmpty(*website))
		idx++
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = now()")
	args = append(args, businessID)
	q := fmt.Sprintf("UPDATE businesses SET %s WHERE id = $%d", strings.Join(sets, ", "), idx)
	_, err := r.pool.Exec(ctx, q, args...)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *repository) UpsertRanking(ctx context.Context, rk *domain.MarketRanking) error {
	criteriaJSON, _ := json.Marshal(rk.CriteriaData)
	if rk.ID == uuid.Nil {
		rk.ID = uuid.New()
	}

	_, err := r.pool.Exec(ctx,
		`INSERT INTO market_rankings (id, user_id, product_category_id, country_code,
		 topsis_score, saw_score, opportunity_score, reliability_score, accessibility_score,
		 criteria_data, algorithm_used, ranked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (user_id, product_category_id, country_code) DO UPDATE SET
		   topsis_score = EXCLUDED.topsis_score,
		   saw_score = EXCLUDED.saw_score,
		   opportunity_score = EXCLUDED.opportunity_score,
		   reliability_score = EXCLUDED.reliability_score,
		   accessibility_score = EXCLUDED.accessibility_score,
		   criteria_data = EXCLUDED.criteria_data,
		   algorithm_used = EXCLUDED.algorithm_used,
		   ranked_at = EXCLUDED.ranked_at`,
		rk.ID, rk.UserID, rk.ProductCategoryID, rk.CountryCode,
		rk.TOPSISScore, rk.SAWScore, rk.OpportunityScore, rk.ReliabilityScore, rk.AccessibilityScore,
		criteriaJSON, rk.AlgorithmUsed, rk.RankedAt,
	)
	return err
}

func (r *repository) ListRankings(ctx context.Context, userID, categoryID uuid.UUID) ([]domain.MarketRanking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, product_category_id, country_code,
		        topsis_score, saw_score, opportunity_score, reliability_score, accessibility_score,
		        criteria_data, algorithm_used, ranked_at
		 FROM market_rankings
		 WHERE user_id = $1 AND product_category_id = $2
		 ORDER BY topsis_score DESC`, userID, categoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rankings []domain.MarketRanking
	for rows.Next() {
		var rk domain.MarketRanking
		if err := rows.Scan(&rk.ID, &rk.UserID, &rk.ProductCategoryID, &rk.CountryCode,
			&rk.TOPSISScore, &rk.SAWScore, &rk.OpportunityScore, &rk.ReliabilityScore, &rk.AccessibilityScore,
			&rk.CriteriaData, &rk.AlgorithmUsed, &rk.RankedAt); err != nil {
			return nil, err
		}
		rankings = append(rankings, rk)
	}
	return rankings, nil
}

func (r *repository) ListLeads(ctx context.Context, marketID, userID uuid.UUID, minScore, page, pageSize int) ([]domain.BusinessWithRelevance, int, error) {
	offset := (page - 1) * pageSize

	var total int
	if minScore > 0 {
		err := r.pool.QueryRow(ctx,
			`SELECT count(*) FROM business_markets bm
			 JOIN lead_scores ls ON ls.business_id = bm.business_id AND ls.market_id = bm.market_id
			 WHERE bm.market_id = $1 AND ls.overall_score >= $2`,
			marketID, minScore,
		).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
	} else {
		err := r.pool.QueryRow(ctx,
			`SELECT count(*) FROM business_markets WHERE market_id = $1`, marketID,
		).Scan(&total)
		if err != nil {
			return nil, 0, err
		}
	}

	minScoreClause := ""
	if minScore > 0 {
		minScoreClause = fmt.Sprintf("AND ls.overall_score >= %d", minScore)
	}

	query := fmt.Sprintf(
		`SELECT b.id, b.name, b.website, b.google_place_id, b.country_code, b.city, b.address,
		        b.latitude, b.longitude, b.industry, b.sub_industry, b.business_type, b.description,
		        b.phone, b.email, b.data_source, b.enrichment_status, b.created_at, b.updated_at,
		        b.shipment_data, b.rating, b.rating_count, b.google_types,
		        bm.relevance_score, bm.discovered_via,
		        ls.overall_score, ls.purchase_likelihood, ls.deal_size_potential,
		        ls.urgency_score, ls.fit_score, ls.accessibility_score,
		        ls.scoring_rationale, ls.strengths, ls.weaknesses, ls.recommended_approach
		 FROM businesses b
		 JOIN business_markets bm ON b.id = bm.business_id
		 LEFT JOIN lead_scores ls ON ls.business_id = b.id AND ls.market_id = bm.market_id
		 WHERE bm.market_id = $1 %s
		 ORDER BY ls.overall_score DESC NULLS LAST, bm.relevance_score DESC NULLS LAST, b.name ASC
		 LIMIT $2 OFFSET $3`, minScoreClause)

	rows, err := r.pool.Query(ctx, query, marketID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []domain.BusinessWithRelevance
	for rows.Next() {
		var bwr domain.BusinessWithRelevance
		err := rows.Scan(
			&bwr.ID, &bwr.Name, &bwr.Website, &bwr.GooglePlaceID, &bwr.CountryCode, &bwr.City, &bwr.Address,
			&bwr.Latitude, &bwr.Longitude, &bwr.Industry, &bwr.SubIndustry, &bwr.BusinessType, &bwr.Description,
			&bwr.Phone, &bwr.Email, &bwr.DataSource, &bwr.EnrichmentStatus, &bwr.CreatedAt, &bwr.UpdatedAt,
			&bwr.ShipmentData, &bwr.Rating, &bwr.RatingCount, &bwr.GoogleTypes,
			&bwr.RelevanceScore, &bwr.DiscoveredVia,
			&bwr.OverallScore, &bwr.PurchaseLikelihood, &bwr.DealSizePotential,
			&bwr.UrgencyScore, &bwr.FitScore, &bwr.AccessibilityScore,
			&bwr.ScoringRationale, &bwr.Strengths, &bwr.Weaknesses, &bwr.RecommendedApproach,
		)
		if err != nil {
			slog.Error("scan lead", "error", err)
			continue
		}
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
