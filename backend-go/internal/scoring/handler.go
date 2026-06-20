package scoring

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// Handler handles market + scoring HTTP endpoints.
type Handler struct {
	repo Repository
}

func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

// MarketRoutes returns routes for /markets.
// Note: /markets/{id}/analysis (POST+GET) was removed — the AI-driven market
// analysis feature is no longer supported because it was hallucination-prone
// without proper trade-data sources.
func (h *Handler) MarketRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListMarkets)
	r.Get("/{marketID}", h.GetMarket)
	r.Delete("/{marketID}", h.DeleteMarket)
	r.Patch("/{marketID}/brand", h.AssignBrand)
	r.Get("/{marketID}/leads", h.GetLeads)
	r.Patch("/{marketID}/leads/{businessID}", h.UpdateLead)
	return r
}

// ScoringRoutes returns routes for /scoring.
func (h *Handler) ScoringRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/rank-markets", h.RankMarkets)
	r.Get("/rankings", h.GetRankings)
	return r
}

func (h *Handler) ListMarkets(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	markets, err := h.repo.ListMarkets(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list markets")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, markets)
}

func (h *Handler) GetMarket(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "marketID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid market ID")
		return
	}

	market, err := h.repo.GetMarket(r.Context(), user.ID, id)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, market)
}

func (h *Handler) DeleteMarket(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "marketID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid market ID")
		return
	}

	if err := h.repo.DeleteMarket(r.Context(), user.ID, id); errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "market not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete market")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AssignBrand sets (or clears) the market's fixed sender profile (brand).
// PATCH /markets/{marketID}/brand  body {"sender_profile_id": "<uuid>"|null}
func (h *Handler) AssignBrand(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	marketID, err := uuid.Parse(chi.URLParam(r, "marketID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid market ID")
		return
	}
	var req struct {
		SenderProfileID *string `json:"sender_profile_id"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var profileID *uuid.UUID
	if req.SenderProfileID != nil && *req.SenderProfileID != "" {
		id, err := uuid.Parse(*req.SenderProfileID)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid sender_profile_id")
			return
		}
		profileID = &id
	}
	if err := h.repo.SetMarketSenderProfile(r.Context(), marketID, user.ID, profileID); errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusBadRequest, "brand not found or not yours")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to assign brand")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateLead lets the user manually correct contact info on a lead
// (email/phone/website) when scraping missed or got them wrong.
// PATCH /markets/{marketID}/leads/{businessID}
type updateLeadRequest struct {
	Email   *string `json:"email,omitempty"`
	Phone   *string `json:"phone,omitempty"`
	Website *string `json:"website,omitempty"`
}

func (h *Handler) UpdateLead(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	businessID, err := uuid.Parse(chi.URLParam(r, "businessID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid business ID")
		return
	}
	var req updateLeadRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Email == nil && req.Phone == nil && req.Website == nil {
		httputil.WriteError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := h.repo.UpdateBusinessContact(r.Context(), businessID, req.Email, req.Phone, req.Website); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to update lead")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetLeads(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	marketID, err := uuid.Parse(chi.URLParam(r, "marketID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid market ID")
		return
	}

	// Don't expose another tenant's market via the leads endpoint.
	if owns, err := h.repo.UserOwnsMarket(r.Context(), user.ID, marketID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get leads")
		return
	} else if !owns {
		httputil.WriteError(w, http.StatusNotFound, "market not found")
		return
	}

	minScore, _ := strconv.Atoi(r.URL.Query().Get("min_score"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	leads, total, err := h.repo.ListLeads(r.Context(), marketID, user.ID, minScore, page, pageSize)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get leads")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"market_id": marketID,
		"leads":     leads,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

type rankMarketsRequest struct {
	ProductCategoryID uuid.UUID        `json:"product_category_id"`
	CriteriaData      []map[string]any `json:"criteria_data"`
}

func (h *Handler) RankMarkets(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req rankMarketsRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.CriteriaData) < 2 {
		httputil.WriteError(w, http.StatusBadRequest, "need at least 2 markets to rank")
		return
	}

	rankings := RankMarkets(req.CriteriaData)

	algo := "topsis+saw"
	for _, rk := range rankings {
		criteriaJSON, _ := json.Marshal(req.CriteriaData)
		dbRanking := &domain.MarketRanking{
			UserID:             user.ID,
			ProductCategoryID:  req.ProductCategoryID,
			CountryCode:        rk.CountryCode,
			TOPSISScore:        decPtr(rk.TOPSISScore),
			SAWScore:           decPtr(rk.SAWScore),
			OpportunityScore:   decPtr(rk.OpportunityScore),
			ReliabilityScore:   decPtr(rk.ReliabilityScore),
			AccessibilityScore: decPtr(rk.AccessibilityScore),
			CriteriaData:       criteriaJSON,
			AlgorithmUsed:      &algo,
			RankedAt:           time.Now(),
		}
		_ = h.repo.UpsertRanking(r.Context(), dbRanking)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"rankings": rankings,
		"count":    len(rankings),
	})
}

func (h *Handler) GetRankings(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	categoryIDStr := r.URL.Query().Get("product_category_id")
	categoryID, err := uuid.Parse(categoryIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid product_category_id")
		return
	}

	rankings, err := h.repo.ListRankings(r.Context(), user.ID, categoryID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get rankings")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"rankings": rankings,
		"count":    len(rankings),
	})
}

func decPtr(v float64) *decimal.Decimal {
	d := decimal.NewFromFloat(v)
	return &d
}
