package contactgroup

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	repo     Repository
	svc      *Service
	contacts contact.Repository
}

func NewHandler(repo Repository, svc *Service, contacts contact.Repository) *Handler {
	return &Handler{repo: repo, svc: svc, contacts: contacts}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(r chi.Router) {
		r.Patch("/", h.SetBrand)
		r.Delete("/", h.Delete)
		r.Get("/contacts", h.ListContacts)
		r.Post("/businesses", h.AddBusinesses)
	})
	return r
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	groups, err := h.repo.List(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list contact groups")
		return
	}
	if groups == nil {
		groups = []GroupWithCount{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

type createRequest struct {
	Name            string  `json:"name"`
	SenderProfileID *string `json:"sender_profile_id"`
	BusinessIDs     []string `json:"business_ids"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	spID, err := parseOptUUID(req.SenderProfileID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid sender_profile_id")
		return
	}
	bizIDs, err := parseUUIDs(req.BusinessIDs)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid business_ids")
		return
	}
	groupID, res, err := h.svc.CreateAndAdd(r.Context(), user.ID, req.Name, spID, bizIDs)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"id": groupID, "result": res})
}

func (h *Handler) SetBrand(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		SenderProfileID *string `json:"sender_profile_id"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	spID, err := parseOptUUID(req.SenderProfileID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid sender_profile_id")
		return
	}
	if err := h.repo.SetBrand(r.Context(), user.ID, id, spID); errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusBadRequest, "group or brand not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to set brand")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if err := h.repo.Delete(r.Context(), user.ID, id); errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "contact group not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete contact group")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
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
	contacts, total, err := h.contacts.ListByGroup(r.Context(), user.ID, id, 500, 0)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list contacts")
		return
	}
	if contacts == nil {
		contacts = []domain.Contact{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"contacts": contacts, "total": total})
}

func (h *Handler) AddBusinesses(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		BusinessIDs []string `json:"business_ids"`
	}
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	bizIDs, err := parseUUIDs(req.BusinessIDs)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid business_ids")
		return
	}
	res, err := h.svc.AddBusinesses(r.Context(), user.ID, id, bizIDs)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "contact group not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, res)
}

func parseOptUUID(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseUUIDs(in []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
