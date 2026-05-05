package crm

import (
	"net/http"
	"time"

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

// Routes mounted under /outreach. Tasks and notes share this handler.
func (h *Handler) TaskRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListTasks)
	r.Post("/", h.CreateTask)
	r.Get("/overdue-count", h.OverdueCount)
	r.Patch("/{id}", h.UpdateTask)
	r.Delete("/{id}", h.DeleteTask)
	return r
}

func (h *Handler) ContactNotesRoutes() chi.Router {
	r := chi.NewRouter()
	// Mounted at /outreach/contacts/{contactID}/notes — chi gives us the
	// contactID via URL param.
	r.Get("/", h.ListNotes)
	r.Post("/", h.CreateNote)
	r.Delete("/{noteID}", h.DeleteNote)
	return r
}

// --- Tasks ---

type taskRequest struct {
	ContactID      *uuid.UUID `json:"contact_id"`
	ConversationID *uuid.UUID `json:"conversation_id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	DueAt          *time.Time `json:"due_at"`
	CompletedAt    *time.Time `json:"completed_at"`
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req taskRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title == "" {
		httputil.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}
	out, err := h.repo.CreateTask(r.Context(), domain.Task{
		UserID:         user.ID,
		ContactID:      req.ContactID,
		ConversationID: req.ConversationID,
		Title:          req.Title,
		Body:           req.Body,
		DueAt:          req.DueAt,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var contactID *uuid.UUID
	if s := r.URL.Query().Get("contact_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid contact_id")
			return
		}
		contactID = &id
	}
	openOnly := r.URL.Query().Get("status") == "open"
	out, err := h.repo.ListTasks(r.Context(), user.ID, contactID, openOnly)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []domain.Task{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"tasks": out})
}

func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
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
	cur, err := h.repo.GetTask(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, "task not found")
		return
	}
	var req taskRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Title != "" {
		cur.Title = req.Title
	}
	cur.Body = req.Body
	cur.DueAt = req.DueAt
	// Treat the presence of completed_at in the body as authoritative —
	// pass nil to mark incomplete, a time to mark done.
	cur.CompletedAt = req.CompletedAt
	saved, err := h.repo.UpdateTask(r.Context(), *cur)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
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
	if err := h.repo.DeleteTask(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) OverdueCount(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	n, err := h.repo.OverdueCount(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]int{"overdue": n})
}

// --- Notes ---

type noteRequest struct {
	Body string `json:"body"`
}

func (h *Handler) CreateNote(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid contactID")
		return
	}
	var req noteRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Body == "" {
		httputil.WriteError(w, http.StatusBadRequest, "body is required")
		return
	}
	out, err := h.repo.CreateNote(r.Context(), domain.Note{
		UserID:    user.ID,
		ContactID: contactID,
		Body:      req.Body,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) ListNotes(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	contactID, err := uuid.Parse(chi.URLParam(r, "contactID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid contactID")
		return
	}
	out, err := h.repo.ListNotes(r.Context(), user.ID, contactID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []domain.Note{}
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"notes": out})
}

func (h *Handler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "noteID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid noteID")
		return
	}
	if err := h.repo.DeleteNote(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
