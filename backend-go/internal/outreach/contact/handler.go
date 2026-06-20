package contact

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	repo       Repository
	notesRoute chi.Router // mounted at /{contactID}/notes
	tasksRoute chi.Router // mounted at /{contactID}/tasks (proxies to /outreach/tasks?contact_id=...)
}

func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

// SetNestedRoutes lets the outreach composition layer plug nested routers
// for /{contactID}/notes (and any other future /{contactID}/<thing>) without
// causing an import cycle between the contact and crm packages.
func (h *Handler) SetNestedRoutes(notes chi.Router) {
	h.notesRoute = notes
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/ensure", h.EnsureFromBusiness)
	r.Post("/ensure-bulk", h.EnsureBulkFromBusinesses)
	r.Patch("/bulk", h.BulkUpdate)
	r.Get("/{contactID}", h.Get)
	r.Patch("/{contactID}", h.Update)
	if h.notesRoute != nil {
		r.Mount("/{contactID}/notes", h.notesRoute)
	}
	return r
}

type bulkEnsureRequest struct {
	BusinessIDs []uuid.UUID `json:"business_ids"`
}

// EnsureBulkFromBusinesses takes a list of business IDs and creates
// contacts for each — bucketing the result so the UI can show "added /
// already existed / no email yet". Idempotent: re-running after the
// user enters a missing email promotes that lead from no_email → added.
func (h *Handler) EnsureBulkFromBusinesses(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req bulkEnsureRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.BusinessIDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "business_ids is required")
		return
	}
	res, err := h.repo.EnsureBulkFromBusinesses(r.Context(), user.ID, req.BusinessIDs)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, res)
}

type bulkRequest struct {
	IDs               []uuid.UUID `json:"ids"`
	PipelineStage     *string     `json:"pipeline_stage,omitempty"`
	DefaultAutomation *string     `json:"default_automation,omitempty"`
	DefaultSequenceID *uuid.UUID  `json:"default_sequence_id,omitempty"`
}

// BulkUpdate applies the provided fields to all listed contacts in one go.
// Used by the lead-list "bulk actions" UI in P3 (set automation, attach
// sequence, change pipeline stage on N rows).
func (h *Handler) BulkUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req bulkRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.IDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "ids is required")
		return
	}
	n, err := h.repo.BulkUpdate(r.Context(), user.ID, req.IDs, BulkFields{
		PipelineStage:     req.PipelineStage,
		DefaultAutomation: req.DefaultAutomation,
		DefaultSequenceID: req.DefaultSequenceID,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"updated": n})
}

// EnsureFromBusiness upserts a contact for a (user, business) pair and returns it.
// Called when the user clicks "Email this lead" in the markets UI.
// Query: ?business_id=<uuid>
func (h *Handler) EnsureFromBusiness(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	businessIDStr := r.URL.Query().Get("business_id")
	if businessIDStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, "business_id is required")
		return
	}
	businessID, err := uuid.Parse(businessIDStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid business_id")
		return
	}
	c, err := h.repo.UpsertFromBusiness(r.Context(), user.ID, businessID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	stage := r.URL.Query().Get("stage")
	marketID := r.URL.Query().Get("market_id")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	var (
		contacts []domain.Contact
		total    int
		err      error
	)
	if marketID != "" {
		mid, perr := uuid.Parse(marketID)
		if perr != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid market_id")
			return
		}
		contacts, total, err = h.repo.ListByMarket(r.Context(), user.ID, mid, pageSize, (page-1)*pageSize)
	} else {
		contacts, total, err = h.repo.List(r.Context(), user.ID, stage, pageSize, (page-1)*pageSize)
	}
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
