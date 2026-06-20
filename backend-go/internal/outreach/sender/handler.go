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

// catalogMaxBytes is the per-brand catalog size cap. Enforced at upload time
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
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Put("/", h.Update)
		r.Delete("/", h.Delete)
		r.Post("/catalog", h.UploadCatalog)
		r.Delete("/catalog", h.DeleteCatalog)
		r.Get("/lessons", h.ListLessons)
		r.Delete("/lessons/{lessonID}", h.DeleteLesson)
	})
	return r
}

// ListLessons returns the brand's remembered reply lessons.
func (h *Handler) ListLessons(w http.ResponseWriter, r *http.Request) {
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
	lessons, err := h.repo.ListLessons(r.Context(), user.ID, id)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load lessons")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"lessons": lessons})
}

// DeleteLesson removes a remembered reply lesson.
func (h *Handler) DeleteLesson(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	lessonID, err := uuid.Parse(chi.URLParam(r, "lessonID"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid lesson id")
		return
	}
	if err := h.repo.DeleteLesson(r.Context(), user.ID, lessonID); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete lesson")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// List returns all of the user's brands. Empty array if none yet.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	profiles, err := h.repo.List(r.Context(), user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load profiles")
		return
	}
	if profiles == nil {
		profiles = []domain.SenderProfile{}
	}
	httputil.WriteJSON(w, http.StatusOK, profiles)
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
	p, err := h.repo.GetByID(r.Context(), user.ID, id)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "brand not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, p)
}

type profileRequest struct {
	Name                   string  `json:"name"`
	CompanyName            string  `json:"company_name"`
	ProductDescription     string  `json:"product_description"`
	ValueProp              string  `json:"value_prop"`
	TargetBuyerDescription string  `json:"target_buyer_description"`
	Tone                   string  `json:"tone"`
	Signature              string  `json:"signature"`
	PhysicalAddress        string  `json:"physical_address"`
	TargetIndustries       string  `json:"target_industries"`
	TargetCountries        string  `json:"target_countries"`
	AvoidCountries         string  `json:"avoid_countries"`
	MinDealSizeUSD         *int64  `json:"min_deal_size_usd"`
	TypicalDealSizeUSD     *int64  `json:"typical_deal_size_usd"`
	DealBreakers           string  `json:"deal_breakers"`
	CompetitiveMoats       string  `json:"competitive_moats"`
	DefaultChannelID       *string `json:"default_channel_id"`
}

func (req profileRequest) toDomain(userID uuid.UUID) (domain.SenderProfile, error) {
	tone := req.Tone
	if tone == "" {
		tone = "formal"
	}
	name := req.Name
	if name == "" {
		name = "Default"
	}
	p := domain.SenderProfile{
		Name:                   name,
		UserID:                 userID,
		CompanyName:            req.CompanyName,
		ProductDescription:     req.ProductDescription,
		ValueProp:              req.ValueProp,
		TargetBuyerDescription: req.TargetBuyerDescription,
		Tone:                   tone,
		Signature:              req.Signature,
		PhysicalAddress:        req.PhysicalAddress,
		TargetIndustries:       req.TargetIndustries,
		TargetCountries:        req.TargetCountries,
		AvoidCountries:         req.AvoidCountries,
		MinDealSizeUSD:         req.MinDealSizeUSD,
		TypicalDealSizeUSD:     req.TypicalDealSizeUSD,
		DealBreakers:           req.DealBreakers,
		CompetitiveMoats:       req.CompetitiveMoats,
	}
	if req.DefaultChannelID != nil && *req.DefaultChannelID != "" {
		id, err := uuid.Parse(*req.DefaultChannelID)
		if err != nil {
			return p, errors.New("invalid default_channel_id")
		}
		p.DefaultChannelID = &id
	}
	return p, nil
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req profileRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CompanyName == "" {
		httputil.WriteError(w, http.StatusBadRequest, "company_name is required")
		return
	}
	p, err := req.toDomain(user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := h.repo.Create(r.Context(), p)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create brand")
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, saved)
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
	var req profileRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CompanyName == "" {
		httputil.WriteError(w, http.StatusBadRequest, "company_name is required")
		return
	}
	p, err := req.toDomain(user.ID)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.ID = id
	saved, err := h.repo.Update(r.Context(), p)
	if errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "brand not found")
		return
	}
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save profile")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, saved)
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
		httputil.WriteError(w, http.StatusNotFound, "brand not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete brand")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UploadCatalog accepts multipart form with field "file". Max 10 MB.
// Replaces any existing catalog on this brand. Returns the updated profile.
func (h *Handler) UploadCatalog(w http.ResponseWriter, r *http.Request) {
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

	if err := h.repo.SaveCatalog(r.Context(), user.ID, id, header.Filename, mimeType, data); errors.Is(err, domain.ErrNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "brand not found")
		return
	} else if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save catalog")
		return
	}

	p, _ := h.repo.GetByID(r.Context(), user.ID, id)
	httputil.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) DeleteCatalog(w http.ResponseWriter, r *http.Request) {
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
	if err := h.repo.DeleteCatalog(r.Context(), user.ID, id); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to remove catalog")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
