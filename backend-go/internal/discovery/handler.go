package discovery

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// Handler serves the Excel-only discovery endpoints.
type Handler struct {
	pipeline *Pipeline
	repo     Repository
	mapper   *AIMapper
}

func NewHandler(pipeline *Pipeline, repo Repository, mapper *AIMapper) *Handler {
	return &Handler{pipeline: pipeline, repo: repo, mapper: mapper}
}

// Routes returns a chi router with the discovery endpoints.
// Flow:
//   POST  /preview    → upload Excel, get headers + AI mapping (no DB writes)
//   POST  /import     → upload Excel + user-confirmed mapping → kicks off pipeline
//   POST  /           → legacy: preview→import in one shot using AI mapping
//   GET   /canonical  → list of canonical fields the mapping UI knows about
//   GET   /{searchID} → poll search state
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.UploadExcel)
	r.Post("/preview", h.Preview)
	r.Post("/import", h.ImportWithMapping)
	r.Get("/canonical", h.Canonical)
	r.Get("/{searchID}", h.GetSearch)
	r.Get("/{searchID}/markets", h.GetMarkets)
	return r
}

// Canonical exposes the canonical field catalog so the mapping UI can
// render dropdowns without hard-coding the list. Public-shape JSON.
func (h *Handler) Canonical(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"fields": Canonical,
	})
}

// GetSearch returns the current state and status of a discovery.
// GET /discover/{searchID}
func (h *Handler) GetSearch(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	searchID, err := parseSearchID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid search ID")
		return
	}

	search, err := h.repo.GetSearch(r.Context(), searchID)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}
	if search.UserID != user.ID {
		httputil.WriteError(w, http.StatusForbidden, "not your search")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, search)
}

func parseSearchID(r *http.Request) (uuid.UUID, error) {
	idStr := chi.URLParam(r, "searchID")
	return uuid.Parse(idStr)
}
