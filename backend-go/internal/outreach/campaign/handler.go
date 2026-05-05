package campaign

import (
	"encoding/json"
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
		r.Get("/contacts", h.ListContacts)
		r.Post("/contacts", h.AddContacts)
		r.Post("/contacts/{contactId}/approve", h.ApproveContact)
		r.Post("/approve-all", h.ApproveAll)
		r.Post("/launch", h.Launch)
		r.Post("/pause", h.Pause)
		r.Post("/resume", h.Resume)
		r.Post("/stop", h.Stop)
	})
	return r
}

// --- routing helpers ---

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	out, err := h.repo.List(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "list campaigns failed")
		return
	}
	if out == nil {
		out = []domain.Campaign{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"campaigns": out})
}

type createRequest struct {
	Name                 string          `json:"name"`
	Goal                 string          `json:"goal"`
	ChannelID            *uuid.UUID      `json:"channel_id"`
	PositioningOverride  json.RawMessage `json:"positioning_override,omitempty"`
	SequenceID           *uuid.UUID      `json:"sequence_id"`
	SendPacePerDay       int             `json:"send_pace_per_day"`
	AttachCatalog        bool            `json:"attach_catalog"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	c := domain.Campaign{
		Name:                req.Name,
		Goal:                req.Goal,
		PositioningOverride: req.PositioningOverride,
		SequenceID:          req.SequenceID,
		SendPacePerDay:      req.SendPacePerDay,
		AttachCatalog:       req.AttachCatalog,
	}
	if req.ChannelID != nil {
		c.ChannelID = *req.ChannelID
	}
	saved, err := h.svc.CreateCampaign(r.Context(), user.ID, c)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, saved)
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
	sum, err := h.repo.Summary(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sum)
}

type updateRequest struct {
	Name                *string         `json:"name"`
	Goal                *string         `json:"goal"`
	PositioningOverride json.RawMessage `json:"positioning_override,omitempty"`
	SequenceID          *uuid.UUID      `json:"sequence_id"`
	SendPacePerDay      *int            `json:"send_pace_per_day"`
	AttachCatalog       *bool           `json:"attach_catalog"`
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
	current, err := h.repo.Get(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}
	var req updateRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name != nil {
		current.Name = *req.Name
	}
	if req.Goal != nil {
		current.Goal = *req.Goal
	}
	if req.PositioningOverride != nil {
		current.PositioningOverride = req.PositioningOverride
	}
	if req.SequenceID != nil {
		current.SequenceID = req.SequenceID
	}
	if req.SendPacePerDay != nil {
		current.SendPacePerDay = *req.SendPacePerDay
	}
	if req.AttachCatalog != nil {
		current.AttachCatalog = *req.AttachCatalog
	}
	saved, err := h.repo.Update(r.Context(), *current)
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
	if err := h.repo.Delete(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "delete failed")
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
	if _, err := h.repo.Get(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}
	statusFilter := r.URL.Query().Get("status")
	rows, err := h.repo.ListContacts(r.Context(), id, statusFilter)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "list contacts failed")
		return
	}
	if rows == nil {
		rows = []CampaignContactRow{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"contacts": rows})
}

type addContactsRequest struct {
	ContactIDs []uuid.UUID `json:"contact_ids"`
	MarketID   *uuid.UUID  `json:"market_id"`
	Force      bool        `json:"force"`
}

func (h *Handler) AddContacts(w http.ResponseWriter, r *http.Request) {
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
	var req addContactsRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.ContactIDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "contact_ids is required")
		return
	}
	out, err := h.svc.AddContacts(r.Context(), user.ID, id, req.ContactIDs, req.MarketID, req.Force)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) ApproveContact(w http.ResponseWriter, r *http.Request) {
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
	cid, err := uuid.Parse(chi.URLParam(r, "contactId"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid contactId")
		return
	}
	if err := h.svc.ApproveContact(r.Context(), user.ID, id, cid); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ApproveAll(w http.ResponseWriter, r *http.Request) {
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
	n, err := h.svc.ApproveAll(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"approved": n})
}

func (h *Handler) Launch(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Launch(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Pause(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Pause(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Resume(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Resume(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.Stop(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
