package ailog

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, log *domain.AIRequestLog) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO ai_request_log (id, user_id, search_id, request_type, provider, model, input_tokens, output_tokens, cost_usd, latency_ms, cache_hit)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		log.ID, log.UserID, log.SearchID, log.RequestType, log.Provider, log.Model,
		log.InputTokens, log.OutputTokens, log.CostUSD, log.LatencyMS, log.CacheHit,
	)
	return err
}

func (r *Repository) GetCostSummary(ctx context.Context, userID uuid.UUID) (*domain.AICostSummary, error) {
	summary := &domain.AICostSummary{}

	// Totals
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COUNT(*)
		 FROM ai_request_log WHERE user_id = $1`, userID,
	).Scan(&summary.TotalCostUSD, &summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalRequests)
	if err != nil {
		return nil, err
	}

	// By search (discovery) — join with searches for file name / status
	rows, err := r.pool.Query(ctx,
		`SELECT a.search_id,
		        COALESCE(s.query->>'source_file_name', ''),
		        COALESCE(s.status, ''),
		        s.result_count,
		        COALESCE(SUM(a.cost_usd), 0),
		        COALESCE(SUM(a.input_tokens), 0),
		        COALESCE(SUM(a.output_tokens), 0),
		        COUNT(*),
		        MIN(a.created_at)
		 FROM ai_request_log a
		 LEFT JOIN searches s ON s.id = a.search_id
		 WHERE a.user_id = $1 AND a.search_id IS NOT NULL
		 GROUP BY a.search_id, s.query, s.status, s.result_count
		 ORDER BY MIN(a.created_at) DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s domain.SearchCostSummary
		if err := rows.Scan(&s.SearchID, &s.SourceFileName, &s.Status, &s.ResultCount,
			&s.CostUSD, &s.InputTokens, &s.OutputTokens, &s.Requests, &s.CreatedAt); err != nil {
			continue
		}
		summary.BySearch = append(summary.BySearch, s)
	}

	// By day (last 30 days)
	rows2, err := r.pool.Query(ctx,
		`SELECT DATE(created_at) as day, COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COUNT(*)
		 FROM ai_request_log
		 WHERE user_id = $1 AND created_at > NOW() - INTERVAL '30 days'
		 GROUP BY DATE(created_at)
		 ORDER BY day DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	for rows2.Next() {
		var d domain.DailyCostSummary
		if err := rows2.Scan(&d.Date, &d.CostUSD, &d.InputTokens, &d.OutputTokens, &d.Requests); err != nil {
			continue
		}
		summary.ByDay = append(summary.ByDay, d)
	}

	// By model
	rows3, err := r.pool.Query(ctx,
		`SELECT provider, model, COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COUNT(*)
		 FROM ai_request_log
		 WHERE user_id = $1
		 GROUP BY provider, model
		 ORDER BY SUM(cost_usd) DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows3.Close()

	for rows3.Next() {
		var m domain.ModelCostSummary
		if err := rows3.Scan(&m.Provider, &m.Model, &m.CostUSD, &m.InputTokens, &m.OutputTokens, &m.Requests); err != nil {
			continue
		}
		summary.ByModel = append(summary.ByModel, m)
	}

	return summary, nil
}
