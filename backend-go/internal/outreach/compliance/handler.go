package compliance

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// ContactSuppressor is the slice of contact-repo behaviour the handler needs.
// Defined as an interface so the package doesn't take a hard dep on the
// concrete contact repository.
type ContactSuppressor interface {
	MarkUnsubscribed(ctx context.Context, contactID uuid.UUID, reason string) error
	GetByID(ctx context.Context, contactID uuid.UUID) (*domain.Contact, error)
}

type Handler struct {
	channels  channel.Repository
	contacts  ContactSuppressor
}

func NewHandler(channels channel.Repository, contacts ContactSuppressor) *Handler {
	return &Handler{channels: channels, contacts: contacts}
}

// Routes are mounted PUBLICLY (no auth) — recipients click the link in their
// email client without ever logging in.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Show)
	r.Post("/", h.Confirm)
	return r
}

// resolveToken finds the channel that owns the unsubscribed contact and
// verifies the token's signature. Tries every enabled channel — there are
// few per user and the HMAC check is cheap.
func (h *Handler) resolveToken(ctx context.Context, raw string) (Token, *domain.UserChannel, *domain.Contact, bool) {
	if raw == "" {
		return Token{}, nil, nil, false
	}
	channels, err := h.channels.ListEnabled(ctx)
	if err != nil {
		return Token{}, nil, nil, false
	}
	for i := range channels {
		tok, ok := VerifyToken(channels[i], raw)
		if !ok {
			continue
		}
		c, err := h.contacts.GetByID(ctx, tok.ContactID)
		if err != nil || c == nil {
			return Token{}, nil, nil, false
		}
		return tok, &channels[i], c, true
	}
	return Token{}, nil, nil, false
}

// Show renders a tiny confirmation page. We don't auto-unsubscribe on GET
// because some email clients prefetch links.
func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	_, _, contact, ok := h.resolveToken(r.Context(), tok)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<html><body><h2>Invalid or expired unsubscribe link</h2><p>Please reply to the original email asking to be removed.</p></body></html>`))
		return
	}
	if contact.UnsubscribedAt != nil {
		_, _ = w.Write([]byte(`<html><body><h2>You are already unsubscribed</h2><p>You will not receive further emails.</p></body></html>`))
		return
	}
	_, _ = w.Write([]byte(`<html><body>
<h2>Unsubscribe</h2>
<p>Click below to stop receiving emails from this sender.</p>
<form method="post" action="?token=` + tok + `">
<button type="submit" style="font-size:16px;padding:10px 20px;">Confirm unsubscribe</button>
</form>
</body></html>`))
}

// Confirm marks the contact unsubscribed. Returns 200 with a success page.
// Idempotent — calling twice is fine.
func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	_, _, contact, ok := h.resolveToken(r.Context(), tok)
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "invalid token")
		return
	}
	if contact.UnsubscribedAt == nil {
		_ = h.contacts.MarkUnsubscribed(r.Context(), contact.ID, "one_click")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<html><body><h2>Unsubscribed</h2><p>You will not receive further emails. Thank you.</p></body></html>`))
}
