package contact

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
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
	r.Get("/", h.List)
	r.Get("/{contactID}", h.Get)
	r.Patch("/{contactID}", h.Update)
	return r
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	stage := r.URL.Query().Get("stage")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	contacts, total, err := h.repo.List(r.Context(), user.ID, stage, pageSize, (page-1)*pageSize)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list contacts")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"contacts":  contacts,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid contact ID")
		return
	}
	c, err := h.repo.Get(r.Context(), user.ID, id)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, c)
}

type updateRequest struct {
	PipelineStage     *string `json:"pipeline_stage"`
	DefaultAutomation *string `json:"default_automation"`
	PrimaryEmail      *string `json:"primary_email"`
	PrimaryPhone      *string `json:"primary_phone"`
	DisplayName       *string `json:"display_name"`
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid contact ID")
		return
	}
	c, err := h.repo.Get(r.Context(), user.ID, id)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}

	var req updateRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.PipelineStage != nil {
		c.PipelineStage = *req.PipelineStage
	}
	if req.DefaultAutomation != nil {
		c.DefaultAutomation = *req.DefaultAutomation
	}
	if req.PrimaryEmail != nil {
		c.PrimaryEmail = req.PrimaryEmail
	}
	if req.PrimaryPhone != nil {
		c.PrimaryPhone = req.PrimaryPhone
	}
	if req.DisplayName != nil && *req.DisplayName != "" {
		c.DisplayName = *req.DisplayName
	}

	saved, err := h.repo.Update(r.Context(), *c)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to update contact")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
}
