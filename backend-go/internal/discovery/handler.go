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
}

func NewHandler(pipeline *Pipeline, repo Repository) *Handler {
	return &Handler{pipeline: pipeline, repo: repo}
}

// Routes returns a chi router with the discovery endpoints.
// Flow: POST / → upload Excel → get {search_id, markets}.
// Poll GET /{searchID} for status. Markets are fetched per-id via /markets/{id}.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.UploadExcel)
	r.Get("/{searchID}", h.GetSearch)
	r.Get("/{searchID}/markets", h.GetMarkets)
	return r
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
