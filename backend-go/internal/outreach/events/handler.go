package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	broker *Broker
}

func NewHandler(broker *Broker) *Handler {
	return &Handler{broker: broker}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Stream)
	return r
}

// Stream upgrades the response to an SSE stream and forwards events for
// the authenticated user until the client disconnects. Browsers reconnect
// EventSource automatically with a small backoff if the stream ends.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Lift the global WriteTimeout (set on the http.Server) so an SSE
	// stream can stay open for as long as the client wants.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// SSE response headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	// Disable buffering on Cloud Run / proxies — stream chunks must hit
	// the client immediately.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Without flushing the response is buffered until the handler
		// returns — useless for SSE.
		return
	}

	ch, cancel := h.broker.Subscribe(user.ID)
	defer cancel()

	// Initial comment + retry hint so the client knows the connection is
	// alive and how long to wait before reconnecting.
	fmt.Fprintf(w, ": connected\nretry: 5000\n\n")
	flusher.Flush()

	// Heartbeat keeps Cloud Run from killing an idle connection at the
	// 60s default timeout (we run with 30 min, but proxies in between
	// can still close on idle).
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, ev); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, ev Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, body)
	return err
}

// Stub to make the package usable from tests without a real ResponseWriter.
var _ = context.Background
