package simulation

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/events"
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
	r.Post("/", h.Run)
	r.Get("/personas", h.ListPersonas)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Delete("/", h.Delete)
		r.Post("/advance", h.Advance)
		r.Get("/playbook", h.GetPlaybook)
		r.Put("/playbook", h.SetPlaybook)
		r.Route("/leads/{leadID}", func(r chi.Router) {
			r.Post("/approve", h.ApproveLead)
			r.Post("/dismiss", h.DismissLead)
			r.Patch("/draft", h.EditDraft)
			r.Post("/refine", h.RefineDraft)
		})
		r.Get("/assistant", h.AssistantHistory)
		r.Post("/assistant/messages", h.AssistantSend)
		r.Post("/assistant/messages/{messageID}/confirm", h.AssistantConfirm)
		r.Post("/assistant/messages/{messageID}/dismiss", h.AssistantDismiss)
	})
	return r
}

func (h *Handler) AssistantHistory(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	msgs, err := h.svc.AssistantHistory(r.Context(), user.ID, id)
	if err != nil {
		simErr(w, err)
		return
	}
	if msgs == nil {
		msgs = []AssistantMessage{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (h *Handler) AssistantSend(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	msg, err := h.svc.AssistantSend(r.Context(), user.ID, id, body.Message)
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, msg)
}

func (h *Handler) AssistantConfirm(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	updated, failed, err := h.svc.AssistantConfirm(r.Context(), user.ID, id, mid, func(done, total int) {
		if h.svc.broker == nil {
			return
		}
		sid := id
		h.svc.broker.Publish(events.Event{Kind: events.KindSimulationProgress, UserID: user.ID,
			Data: map[string]any{"simulation_id": sid.String(), "action": "assistant_apply", "done": done, "total": total}})
	})
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"updated": updated, "failed": failed})
}

func (h *Handler) AssistantDismiss(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	mid, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	if err := h.svc.AssistantDismiss(r.Context(), user.ID, id, mid); err != nil {
		simErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ids resolves the authenticated user + the {id} (simulation) URL param.
func (h *Handler) ids(w http.ResponseWriter, r *http.Request) (*domain.User, uuid.UUID, bool) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return nil, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return nil, uuid.Nil, false
	}
	return user, id, true
}

func (h *Handler) leadID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "leadID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid lead id")
		return uuid.Nil, false
	}
	return id, true
}

// simErr maps a service error to the right status (404 for not-found, else 400).
func simErr(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "simulation not found")
		return
	}
	httputil.WriteError(w, http.StatusBadRequest, err.Error())
}

func (h *Handler) Advance(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	sim, err := h.svc.Advance(r.Context(), user.ID, id)
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
}

func (h *Handler) ApproveLead(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	lid, ok := h.leadID(w, r)
	if !ok {
		return
	}
	sim, err := h.svc.ApproveLead(r.Context(), user.ID, id, lid)
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
}

func (h *Handler) DismissLead(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	lid, ok := h.leadID(w, r)
	if !ok {
		return
	}
	sim, err := h.svc.DismissLead(r.Context(), user.ID, id, lid)
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
}

func (h *Handler) EditDraft(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	lid, ok := h.leadID(w, r)
	if !ok {
		return
	}
	var body struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sim, err := h.svc.EditLead(r.Context(), user.ID, id, lid, body.Subject, body.Body)
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
}

func (h *Handler) RefineDraft(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	lid, ok := h.leadID(w, r)
	if !ok {
		return
	}
	var body struct {
		Instruction string `json:"instruction"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sim, err := h.svc.RefineLead(r.Context(), user.ID, id, lid, body.Instruction)
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
}

func (h *Handler) GetPlaybook(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	sim, err := h.repo.Get(r.Context(), user.ID, id)
	if err != nil {
		simErr(w, err)
		return
	}
	pb := sim.Playbook
	if pb == nil {
		pb = map[string]string{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"playbook": pb})
}

func (h *Handler) SetPlaybook(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.ids(w, r)
	if !ok {
		return
	}
	var body struct {
		Playbook map[string]string `json:"playbook"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sim, err := h.svc.SetPlaybook(r.Context(), user.ID, id, body.Playbook)
	if err != nil {
		simErr(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
}

// ListPersonas exposes the persona catalog for the UI picker.
func (h *Handler) ListPersonas(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	out := make([]item, 0, len(Personas))
	for _, p := range Personas {
		out = append(out, item{Key: p.Key, Label: p.Label})
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"personas": out})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sims, err := h.repo.List(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list simulations")
		return
	}
	if sims == nil {
		sims = []Simulation{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"simulations": sims})
}

func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in RunInput
	if err := httputil.DecodeJSON(r, &in); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.MarketID == uuid.Nil || in.BrandID == uuid.Nil {
		httputil.WriteError(w, http.StatusBadRequest, "market_id and brand_id are required")
		return
	}
	// Default to the stepped interactive engine; "quick" runs the one-shot batch.
	run := h.svc.CreateInteractive
	if in.Mode == "quick" {
		run = h.svc.Run
	}
	sim, err := run(r.Context(), user.ID, in)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
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
	sim, err := h.repo.Get(r.Context(), user.ID, id)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "simulation not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load simulation")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, sim)
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
		httputil.WriteError(w, http.StatusNotFound, "simulation not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete simulation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
