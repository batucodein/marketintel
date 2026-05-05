package sender

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// catalogMaxBytes is the per-user catalog size cap. Enforced at upload time
// so a corrupt/huge PDF can't blow up the DB.
const catalogMaxBytes = 10 * 1024 * 1024 // 10 MB

type Handler struct {
	repo Repository
}

func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Get)
	r.Put("/", h.Upsert)
	r.Post("/catalog", h.UploadCatalog)
	r.Delete("/catalog", h.DeleteCatalog)
	return r
}

// UploadCatalog accepts multipart form with field "file". Max 10 MB.
// Replaces any existing catalog. Returns the updated profile metadata.
func (h *Handler) UploadCatalog(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := r.ParseMultipartForm(catalogMaxBytes + 1<<20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	if header.Size > catalogMaxBytes {
		httputil.WriteError(w, http.StatusRequestEntityTooLarge, "catalog exceeds 10 MB limit")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, catalogMaxBytes+1))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "failed to read file: "+err.Error())
		return
	}
	if len(data) > catalogMaxBytes {
		httputil.WriteError(w, http.StatusRequestEntityTooLarge, "catalog exceeds 10 MB limit")
		return
	}
	if len(data) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "empty file")
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	if err := h.repo.SaveCatalog(r.Context(), user.ID, header.Filename, mimeType, data); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save catalog")
		return
	}

	p, _ := h.repo.Get(r.Context(), user.ID)
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) DeleteCatalog(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.repo.DeleteCatalog(r.Context(), user.ID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to remove catalog")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Get returns the user's sender profile. If none exists yet, returns an
// empty default so the frontend can always render a form.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	p, err := h.repo.Get(r.Context(), user.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	if p == nil {
		p = &domain.SenderProfile{UserID: user.ID, Tone: "formal"}
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

type upsertRequest struct {
	CompanyName            string  `json:"company_name"`
	ProductDescription     string  `json:"product_description"`
	ValueProp              string  `json:"value_prop"`
	TargetBuyerDescription string  `json:"target_buyer_description"`
	Tone                   string  `json:"tone"`
	Signature              string  `json:"signature"`
	PhysicalAddress        string  `json:"physical_address"`
	DefaultChannelID       *string `json:"default_channel_id"`
}

func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req upsertRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CompanyName == "" {
		httputil.WriteError(w, http.StatusBadRequest, "company_name is required")
		return
	}
	if req.Tone == "" {
		req.Tone = "formal"
	}

	p := domain.SenderProfile{
		UserID:                 user.ID,
		CompanyName:            req.CompanyName,
		ProductDescription:     req.ProductDescription,
		ValueProp:              req.ValueProp,
		TargetBuyerDescription: req.TargetBuyerDescription,
		Tone:                   req.Tone,
		Signature:              req.Signature,
		PhysicalAddress:        req.PhysicalAddress,
	}
	if req.DefaultChannelID != nil && *req.DefaultChannelID != "" {
		id, err := uuid.Parse(*req.DefaultChannelID)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "invalid default_channel_id")
			return
		}
		p.DefaultChannelID = &id
	}

	saved, err := h.repo.Upsert(r.Context(), p)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save profile")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
}
