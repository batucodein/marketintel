package group

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Put("/steps", h.ReplaceSteps)
		r.Get("/emails", h.Emails)
		r.Get("/cadence", h.Cadence)
		r.Patch("/channel", h.SetChannel)
		r.Get("/playbook", h.GetPlaybook)
		r.Put("/playbook", h.SavePlaybook)
		r.Get("/assistant", h.AssistantHistory)
		r.Post("/assistant/messages", h.AssistantSend)
		r.Post("/assistant/messages/{messageID}/confirm", h.AssistantConfirm)
		r.Post("/assistant/messages/{messageID}/dismiss", h.AssistantDismiss)
	})
	return r
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	groups, err := h.svc.List(r.Context(), user.ID)
	if err != nil {
		slog.Error("group list failed", "user_id", user.ID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"groups": groups})
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
	detail, err := h.svc.Get(r.Context(), user.ID, id)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load group")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, detail)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var in CreateGroupInput
	if err := httputil.DecodeJSON(r, &in); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	group, err := h.svc.Create(r.Context(), user.ID, in)
	if errors.Is(err, ErrGroupNoBrand) {
		httputil.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, group)
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
	var body struct {
		Steps []domain.SequenceStep `json:"steps"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	steps, err := h.svc.ReplaceSteps(r.Context(), user.ID, id, body.Steps)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"steps": steps})
}

func (h *Handler) Emails(w http.ResponseWriter, r *http.Request) {
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
	q := r.URL.Query()
	facets := EmailFacets{
		Sentiment: q["sentiment"],
		Tags:      q["tag"],
		Status:    q["status"],
	}
	emails, err := h.svc.Emails(r.Context(), user.ID, id, facets)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list emails")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"emails": emails})
}

func (h *Handler) SetChannel(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		ChannelID string `json:"channel_id"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	channelID, err := uuid.Parse(body.ChannelID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid channel_id")
		return
	}
	if err := h.svc.SetChannel(r.Context(), user.ID, id, channelID); errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "group not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Cadence(w http.ResponseWriter, r *http.Request) {
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
	stats, err := h.svc.Cadence(r.Context(), user.ID, id)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load cadence stats")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, stats)
}

func (h *Handler) GetPlaybook(w http.ResponseWriter, r *http.Request) {
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
	entries, err := h.svc.GetPlaybook(r.Context(), user.ID, id)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load playbook")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"playbook": entries})
}

func (h *Handler) SavePlaybook(w http.ResponseWriter, r *http.Request) {
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
	var body struct {
		Playbook []PlaybookEntry `json:"playbook"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.svc.SavePlaybook(r.Context(), user.ID, id, body.Playbook); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := h.svc.GetPlaybook(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "saved, but failed to reload playbook")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"playbook": entries})
}

// AssistantHistory returns the chat thread. The memory (playbook) is loaded
// separately by the UI via the playbook endpoint.
func (h *Handler) AssistantHistory(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.groupID(w, r)
	if !ok {
		return
	}
	msgs, err := h.svc.AssistantHistory(r.Context(), user.ID, id)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "group not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load assistant")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

// AssistantSend posts a user message and returns the assistant's reply.
func (h *Handler) AssistantSend(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.groupID(w, r)
	if !ok {
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := httputil.DecodeJSON(r, &body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	msg, err := h.svc.AssistantSend(r.Context(), user.ID, id, body.Message)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, msg)
}

// AssistantConfirm applies the proposed directive on a message.
func (h *Handler) AssistantConfirm(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.groupID(w, r)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	updated, failed, err := h.svc.AssistantConfirm(r.Context(), user.ID, id, messageID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"updated": updated, "failed": failed})
}

// AssistantDismiss declines the proposal on a message.
func (h *Handler) AssistantDismiss(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.groupID(w, r)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid message id")
		return
	}
	if err := h.svc.AssistantDismiss(r.Context(), user.ID, id, messageID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to dismiss")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// groupID resolves auth + the {id} param, writing the error response on failure.
func (h *Handler) groupID(w http.ResponseWriter, r *http.Request) (*domain.User, uuid.UUID, bool) {
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
