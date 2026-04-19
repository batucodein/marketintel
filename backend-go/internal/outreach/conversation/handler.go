package conversation

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	repo    Repository
	service *Service
}

func NewHandler(repo Repository, svc *Service) *Handler {
	return &Handler{repo: repo, service: svc}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Start)
	r.Get("/{id}", h.Get)
	r.Patch("/{id}", h.Update)
	r.Post("/{id}/read", h.MarkRead)
	r.Post("/{id}/messages", h.SendMessage)
	r.Post("/{id}/draft", h.DraftReply)
	return r
}

// List returns the inbox.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	unread := r.URL.Query().Get("unread") == "true"
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	rows, total, err := h.repo.ListInbox(r.Context(), user.ID, unread, pageSize, (page-1)*pageSize)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list inbox")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"conversations": rows,
		"total":         total,
		"page":          page,
		"page_size":     pageSize,
	})
}

// Get returns conversation + full message history.
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
	conv, err := h.repo.Get(r.Context(), user.ID, id)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}
	msgs, err := h.repo.ListMessages(r.Context(), conv.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"conversation": conv,
		"messages":     msgs,
	})
}

type startRequest struct {
	ContactID  string `json:"contact_id"`
	DraftWithAI bool   `json:"draft_with_ai"`
}

// Start creates a new conversation for a contact and (optionally) AI-drafts the opening.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req startRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	contactID, err := uuid.Parse(req.ContactID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid contact_id")
		return
	}
	res, err := h.service.StartFromContact(r.Context(), user.ID, contactID, req.DraftWithAI)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, res)
}

type updateRequest struct {
	Status     *string `json:"status"`
	Automation *string `json:"automation"`
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
	var req updateRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Status != nil {
		if err := h.repo.UpdateStatus(r.Context(), user.ID, id, *req.Status); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update status")
			return
		}
	}
	if req.Automation != nil {
		if err := h.repo.UpdateAutomation(r.Context(), user.ID, id, *req.Automation); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to update automation")
			return
		}
	}
	conv, err := h.repo.Get(r.Context(), user.ID, id)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, conv)
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
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
	if err := h.repo.MarkRead(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to mark read")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
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
	var req SendMessageRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	msg, err := h.service.SendMessage(r.Context(), user.ID, id, req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, msg)
}

func (h *Handler) DraftReply(w http.ResponseWriter, r *http.Request) {
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
	msg, err := h.service.DraftReply(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, msg)
}
