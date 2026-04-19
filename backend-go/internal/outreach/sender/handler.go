package sender

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	repo Repository
}

func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Get)
	r.Put("/", h.Upsert)
	return r
}

// Get returns the user's sender profile. If none exists yet, returns an
// empty default so the frontend can always render a form.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	p, err := h.repo.Get(r.Context(), user.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	if p == nil {
		p = &domain.SenderProfile{UserID: user.ID, Tone: "formal"}
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

type upsertRequest struct {
	CompanyName            string  `json:"company_name"`
	ProductDescription     string  `json:"product_description"`
	ValueProp              string  `json:"value_prop"`
	TargetBuyerDescription string  `json:"target_buyer_description"`
	Tone                   string  `json:"tone"`
	Signature              string  `json:"signature"`
	DefaultChannelID       *string `json:"default_channel_id"`
}

func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req upsertRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CompanyName == "" {
		httputil.WriteError(w, http.StatusBadRequest, "company_name is required")
		return
	}
	if req.Tone == "" {
		req.Tone = "formal"
	}

	p := domain.SenderProfile{
		UserID:                 user.ID,
		CompanyName:            req.CompanyName,
		ProductDescription:     req.ProductDescription,
		ValueProp:              req.ValueProp,
		TargetBuyerDescription: req.TargetBuyerDescription,
		Tone:                   req.Tone,
		Signature:              req.Signature,
	}
	if req.DefaultChannelID != nil && *req.DefaultChannelID != "" {
		id, err := uuid.Parse(*req.DefaultChannelID)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid default_channel_id")
			return
		}
		p.DefaultChannelID = &id
	}

	saved, err := h.repo.Upsert(r.Context(), p)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save profile")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
}
