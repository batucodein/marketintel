package dashboard

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// AICostProvider returns cost summary for a user.
type AICostProvider interface {
	GetCostSummary(ctx context.Context, userID uuid.UUID) (*domain.AICostSummary, error)
}

type Handler struct {
	pool     *pgxpool.Pool
	aiCosts  AICostProvider
}

func NewHandler(pool *pgxpool.Pool, aiCosts AICostProvider) *Handler {
	return &Handler{pool: pool, aiCosts: aiCosts}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/overview", h.Overview)
	r.Get("/top-leads", h.TopLeads)
	r.Get("/ai-costs", h.AICosts)
	return r
}

type overviewResponse struct {
	TotalMarkets    int `json:"total_markets"`
	TotalBusinesses int `json:"total_businesses"`
	TotalLeads      int `json:"total_leads"`
	TotalSearches   int `json:"total_searches"`
}

func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()
	var resp overviewResponse

	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM markets`).Scan(&resp.TotalMarkets)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM businesses`).Scan(&resp.TotalBusinesses)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM lead_scores WHERE user_id = $1`, user.ID).Scan(&resp.TotalLeads)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM searches WHERE user_id = $1`, user.ID).Scan(&resp.TotalSearches)

	httputil.WriteJSON(w, http.StatusOK, resp)
}

type topLead struct {
	BusinessID   uuid.UUID `json:"business_id"`
	BusinessName string    `json:"business_name"`
	OverallScore int       `json:"overall_score"`
	BusinessType *string   `json:"business_type"`
	City         *string   `json:"city"`
	CountryCode  *string   `json:"country_code"`
	MarketName   string    `json:"market_name"`
}

func (h *Handler) TopLeads(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT ls.business_id, b.name, ls.overall_score, b.business_type, b.city, b.country_code, m.name
		 FROM lead_scores ls
		 JOIN businesses b ON ls.business_id = b.id
		 JOIN markets m ON ls.market_id = m.id
		 WHERE ls.user_id = $1
		 ORDER BY ls.overall_score DESC
		 LIMIT 20`, user.ID,
	)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get top leads")
		return
	}
	defer rows.Close()

	var leads []topLead
	for rows.Next() {
		var l topLead
		if err := rows.Scan(&l.BusinessID, &l.BusinessName, &l.OverallScore, &l.BusinessType, &l.City, &l.CountryCode, &l.MarketName); err != nil {
			continue
		}
		leads = append(leads, l)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"leads": leads,
		"count": len(leads),
	})
}

func (h *Handler) AICosts(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	summary, err := h.aiCosts.GetCostSummary(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get AI costs")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, summary)
}
