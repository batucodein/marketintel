package sequence

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	repo Repository
	svc  *Service
}

func NewHandler(repo Repository, svc *Service) *Handler {
	return &Handler{repo: repo, svc: svc}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Patch("/", h.Update)
		r.Delete("/", h.Delete)
		r.Post("/steps", h.ReplaceSteps)
	})
	return r
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.repo.ListSequences(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "list sequences failed")
		return
	}
	if out == nil {
		out = []domain.Sequence{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"sequences": out})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req CreateSequenceInput
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.svc.CreateWithSteps(r.Context(), user.ID, req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := h.svc.GetWithSteps(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "sequence not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

type updateRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	cur, err := h.repo.GetSequence(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "sequence not found")
		return
	}
	var req updateRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name != nil {
		cur.Name = *req.Name
	}
	if req.Description != nil {
		cur.Description = *req.Description
	}
	saved, err := h.repo.UpdateSequence(r.Context(), *cur)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.repo.DeleteSequence(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type replaceStepsRequest struct {
	Steps []domain.SequenceStep `json:"steps"`
}

func (h *Handler) ReplaceSteps(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req replaceStepsRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.svc.ReplaceSteps(r.Context(), user.ID, id, req.Steps)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"steps": out})
}
