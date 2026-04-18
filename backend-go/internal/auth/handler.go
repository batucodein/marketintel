package auth

import (
	"net/http"

	"github.com/batuhan/marketintel/internal/platform/httputil"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type registerRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	CompanyName *string `json:"company_name"`
	HomeCountry *string `json:"home_country"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type userResponse struct {
	ID                string  `json:"id"`
	Email             string  `json:"email"`
	CompanyName       *string `json:"company_name"`
	HomeCountry       *string `json:"home_country"`
	SubscriptionTier  string  `json:"subscription_tier"`
	APICallsRemaining int     `json:"api_calls_remaining"`
}

type updateProfileRequest struct {
	CompanyName *string `json:"company_name"`
	HomeCountry *string `json:"home_country"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		httputil.WriteError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	tokens, err := h.svc.Register(r.Context(), req.Email, req.Password, req.CompanyName, req.HomeCountry)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, tokens)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokens, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tokens)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tokens, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tokens)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, userResponse{
		ID:                user.ID.String(),
		Email:             user.Email,
		CompanyName:       user.CompanyName,
		HomeCountry:       user.HomeCountry,
		SubscriptionTier:  user.SubscriptionTier,
		APICallsRemaining: user.APICallsRemaining,
	})
}

func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req updateProfileRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated, err := h.svc.UpdateProfile(r.Context(), user.ID, req.CompanyName, req.HomeCountry)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, userResponse{
		ID:                updated.ID.String(),
		Email:             updated.Email,
		CompanyName:       updated.CompanyName,
		HomeCountry:       updated.HomeCountry,
		SubscriptionTier:  updated.SubscriptionTier,
		APICallsRemaining: updated.APICallsRemaining,
	})
}
