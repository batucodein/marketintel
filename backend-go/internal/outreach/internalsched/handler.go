package internalsched

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// Tickable is anything the scheduler tick can fan out to. Each impl runs
// independently — one failing should not abort the others.
type Tickable interface {
	Tick(ctx context.Context) (TickResult, error)
}

// TickResult is the per-component summary returned in the tick response.
type TickResult struct {
	Component string `json:"component"`
	Processed int    `json:"processed"`
	Errors    int    `json:"errors"`
	Note      string `json:"note,omitempty"`
}

// Handler exposes /internal/scheduler/tick. Components register themselves
// via Register(); each is called sequentially per tick.
type Handler struct {
	components []Tickable
}

func NewHandler() *Handler {
	return &Handler{}
}

// Register adds a component. Order matters: poll inbound BEFORE running the
// sequence engine so reply-detection has the latest inbound messages.
func (h *Handler) Register(t Tickable) {
	h.components = append(h.components, t)
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/scheduler/tick", h.Tick)
	r.Get("/scheduler/healthz", h.Health)
	return r
}

// Health is a token-protected ping for monitoring.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"components": len(h.components),
	})
}

// Tick fans out to every registered component. Always returns 200 with a
// per-component summary so Cloud Scheduler doesn't retry on partial failure.
func (h *Handler) Tick(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	results := make([]TickResult, 0, len(h.components))

	for _, c := range h.components {
		// Bound each component to keep the tick from running away.
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		res, err := c.Tick(ctx)
		cancel()
		if err != nil {
			res.Errors++
			res.Note = err.Error()
			slog.Warn("scheduler tick component failed", "component", res.Component, "error", err)
		}
		results = append(results, res)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"duration_ms": time.Since(start).Milliseconds(),
		"results":     results,
	})
}
