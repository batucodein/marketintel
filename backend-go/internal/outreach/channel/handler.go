package channel

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/crypto"
	"github.com/batuhan/marketintel/internal/platform/httputil"
	"github.com/batuhan/marketintel/internal/platform/oauth"
)

// Handler serves /outreach/channels/* endpoints.
type Handler struct {
	repo         Repository
	gmail        *oauth.GmailOAuth
	cipher       *crypto.Cipher
	oauthState   *stateStore // short-lived CSRF state for OAuth round-trip
	frontendURL  string      // where to redirect back after OAuth success
}

func NewHandler(repo Repository, gmail *oauth.GmailOAuth, cipher *crypto.Cipher, frontendURL string) *Handler {
	return &Handler{
		repo:        repo,
		gmail:       gmail,
		cipher:      cipher,
		oauthState:  newStateStore(),
		frontendURL: frontendURL,
	}
}

// Routes mounts authenticated /outreach/channels endpoints.
// The Gmail callback is public — mount via PublicRoutes separately.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/default", h.SetDefault)
	r.Get("/gmail/auth-url", h.GmailAuthURL)
	return r
}

// PublicRoutes returns the routes that must NOT be behind auth middleware.
// Google calls our callback directly with no Authorization header; we use
// the OAuth `state` to identify the user. Mount at /outreach/channels.
func (h *Handler) PublicRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/gmail/callback", h.GmailCallback)
	return r
}

// List returns the user's connected channels (tokens stripped).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	chs, err := h.repo.List(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list channels")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, chs)
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
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete channel")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SetDefault(w http.ResponseWriter, r *http.Request) {
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
	if err := h.repo.SetDefault(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to set default")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GmailAuthURL returns a URL the frontend should redirect the user to.
// After consent, Google calls GmailCallback on this service.
func (h *Handler) GmailAuthURL(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.gmail == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Gmail OAuth not configured")
		return
	}

	state := randomState()
	h.oauthState.put(state, user.ID)

	url := h.gmail.AuthCodeURL(state, "")
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"auth_url": url})
}

// GmailCallback receives the redirect from Google after the user consents.
// Exchanges code -> tokens, fetches the user's email, and persists a
// user_channels row. Then redirects back to the frontend.
func (h *Handler) GmailCallback(w http.ResponseWriter, r *http.Request) {
	// Google may send ?error=... on consent denial.
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		h.redirectToSettings(w, r, "error="+errParam)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		httputil.WriteError(w, http.StatusBadRequest, "missing code or state")
		return
	}

	userID, ok := h.oauthState.take(state)
	if !ok {
		httputil.WriteError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}

	token, err := h.gmail.Exchange(r.Context(), code)
	if err != nil {
		h.redirectToSettings(w, r, "error=exchange_failed")
		return
	}

	info, err := h.gmail.FetchUserInfo(r.Context(), token)
	if err != nil {
		h.redirectToSettings(w, r, "error=userinfo_failed")
		return
	}
	if info.Email == "" {
		h.redirectToSettings(w, r, "error=no_email")
		return
	}

	// Encrypt tokens.
	accessCipher, err := h.cipher.Encrypt(token.AccessToken)
	if err != nil {
		h.redirectToSettings(w, r, "error=encrypt_access")
		return
	}
	refreshCipher, err := h.cipher.Encrypt(token.RefreshToken)
	if err != nil {
		h.redirectToSettings(w, r, "error=encrypt_refresh")
		return
	}

	// Upsert: if we already have a channel with this from_email, update tokens.
	existing, err := h.repo.GetByFromEmail(r.Context(), userID, info.Email)
	if err == nil {
		expires := token.Expiry
		_ = h.repo.UpdateTokens(r.Context(), existing.ID, accessCipher, refreshCipher, &expires)
	} else if errors.Is(err, domain.ErrNotFound) {
		// Create new channel.
		label := info.Email
		if info.Name != "" {
			label = fmt.Sprintf("%s (%s)", info.Name, info.Email)
		}
		expiresAt := token.Expiry
		_, err := h.repo.Create(r.Context(), domain.UserChannel{
			UserID:                  userID,
			Type:                    domain.ChannelTypeGmailOAuth,
			DisplayLabel:            label,
			FromEmail:               info.Email,
			OAuthAccessTokenCipher:  &accessCipher,
			OAuthRefreshTokenCipher: &refreshCipher,
			OAuthExpiresAt:          &expiresAt,
			OAuthScope:              ptrNonEmpty(token.Extra("scope")),
			Enabled:                 true,
			IsDefault:               false,
		})
		if err != nil {
			h.redirectToSettings(w, r, "error=persist_failed")
			return
		}
	} else {
		h.redirectToSettings(w, r, "error=lookup_failed")
		return
	}

	h.redirectToSettings(w, r, "connected=1")
}

func (h *Handler) redirectToSettings(w http.ResponseWriter, r *http.Request, qs string) {
	dest := h.frontendURL + "/outreach/settings/channels"
	if qs != "" {
		dest += "?" + qs
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// --- helpers ------------------------------------------------------------

func randomState() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func ptrNonEmpty(v any) *string {
	if s, ok := v.(string); ok && s != "" {
		return &s
	}
	return nil
}

// stateStore holds OAuth CSRF states in memory with a short TTL.
// Acceptable for single-instance Cloud Run + short OAuth round-trip.
// For multi-instance, move to Redis.
type stateStore struct {
	mu   sync.Mutex
	data map[string]stateEntry
}

type stateEntry struct {
	userID  uuid.UUID
	expires time.Time
}

func newStateStore() *stateStore { return &stateStore{data: make(map[string]stateEntry)} }

func (s *stateStore) put(state string, userID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[state] = stateEntry{userID: userID, expires: time.Now().Add(10 * time.Minute)}
	// Quick GC pass.
	for k, v := range s.data {
		if time.Now().After(v.expires) {
			delete(s.data, k)
		}
	}
}

func (s *stateStore) take(state string) (uuid.UUID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[state]
	if !ok {
		return uuid.Nil, false
	}
	delete(s.data, state)
	if time.Now().After(e.expires) {
		return uuid.Nil, false
	}
	return e.userID, true
}
