package discovery

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// UploadExcel is the new Excel-only entry point. Replaces the old 4-stage flow.
// POST /discover/ (multipart/form-data, field: "file")
func (h *Handler) UploadExcel(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	fileName := ""
	if header != nil {
		fileName = header.Filename
	}

	search, err := h.repo.CreateSearch(r.Context(), user.ID, "discovery", []byte(`{}`))
	if err != nil {
		slog.Error("create search failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create search")
		return
	}

	ctx := ai.WithUserID(r.Context(), user.ID)
	ctx = ai.WithSearchID(ctx, search.ID)

	result, err := h.pipeline.ProcessExcel(ctx, search.ID, user.ID, file, fileName)
	if err != nil {
		slog.Error("process excel failed", "search_id", search.ID, "error", err)
		_ = h.repo.UpdateSearchStatus(r.Context(), search.ID, "failed", nil)
		httputil.WriteError(w, http.StatusBadRequest, "upload failed: "+err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"search_id":       search.ID,
		"status":          "enriching",
		"markets":         result.Markets,
		"total_importers": result.TotalImporters,
		"total_shipments": result.TotalShipments,
		"warnings":        result.Warnings,
	})
}

// Preview reads the first row + up to 5 sample rows from the uploaded
// file, asks the AI mapper to suggest a header→canonical mapping, and
// returns everything to the frontend so the user can confirm/override
// before triggering the actual import. No DB writes happen here.
//
// POST /discover/preview (multipart/form-data: "file")
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	sample, err := ReadHeaderSample(file)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "could not read excel: "+err.Error())
		return
	}

	preview, err := h.mapper.Map(r.Context(), user.ID, sample)
	if err != nil {
		slog.Error("preview: mapping failed", "user_id", user.ID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "ai mapping failed")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, preview)
}

// ImportWithMapping accepts both the uploaded file AND the user-confirmed
// {header → canonical_key} mapping, persists the mapping next to a new
// search row, and runs the full pipeline using the dynamic mapping.
//
// POST /discover/import (multipart/form-data: "file", "mapping" (JSON), "ai_confidence" (optional JSON))
func (h *Handler) ImportWithMapping(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	mappingRaw := r.FormValue("mapping")
	if mappingRaw == "" {
		httputil.WriteError(w, http.StatusBadRequest, "mapping is required")
		return
	}
	var mapping map[string]string
	if err := json.Unmarshal([]byte(mappingRaw), &mapping); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "mapping must be a JSON object {header:canonical}")
		return
	}

	var aiConfidence map[string]float64
	if conf := r.FormValue("ai_confidence"); conf != "" {
		_ = json.Unmarshal([]byte(conf), &aiConfidence)
	}
	var userOverrides map[string]string
	if uo := r.FormValue("user_overrides"); uo != "" {
		_ = json.Unmarshal([]byte(uo), &userOverrides)
	}

	// Server-side sanity: require all Required canonical keys to be mapped.
	mappedKeys := map[string]bool{}
	for _, v := range mapping {
		mappedKeys[v] = true
	}
	for _, req := range RequiredKeys() {
		if !mappedKeys[req] {
			httputil.WriteError(w, http.StatusBadRequest, "missing required field: "+req)
			return
		}
	}

	fileName := ""
	if header != nil {
		fileName = header.Filename
	}

	// Persist the entire file in memory so we can read it twice (once for
	// the mapping persistence path below, again for the pipeline). 50 MB
	// is the upload cap so this is safe.
	buf, err := io.ReadAll(file)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "could not read file: "+err.Error())
		return
	}

	search, err := h.repo.CreateSearch(r.Context(), user.ID, "discovery", []byte(`{}`))
	if err != nil {
		slog.Error("create search failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create search")
		return
	}

	if err := h.repo.SaveColumnMapping(r.Context(), search.ID, mapping, aiConfidence, userOverrides); err != nil {
		slog.Warn("could not persist column mapping", "search_id", search.ID, "error", err)
		// Non-fatal — the pipeline can still run.
	}

	ctx := ai.WithUserID(r.Context(), user.ID)
	ctx = ai.WithSearchID(ctx, search.ID)

	result, err := h.pipeline.ProcessExcelWithMapping(ctx, search.ID, user.ID, bytes.NewReader(buf), fileName, mapping)
	if err != nil {
		slog.Error("import failed", "search_id", search.ID, "error", err)
		_ = h.repo.UpdateSearchStatus(r.Context(), search.ID, "failed", nil)
		httputil.WriteError(w, http.StatusBadRequest, "import failed: "+err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"search_id":       search.ID,
		"status":          "enriching",
		"markets":         result.Markets,
		"total_importers": result.TotalImporters,
		"total_shipments": result.TotalShipments,
		"warnings":        result.Warnings,
	})
}

// GetMarkets returns the list of markets created by a specific upload.
// GET /discover/{searchID}/markets
func (h *Handler) GetMarkets(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	searchID, err := parseSearchID(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid search ID")
		return
	}

	search, err := h.repo.GetSearch(r.Context(), searchID)
	if err != nil {
		httputil.MapDomainError(w, err)
		return
	}
	if search.UserID != user.ID {
		httputil.WriteError(w, http.StatusForbidden, "not your search")
		return
	}

	var state ExcelSearchState
	_ = json.Unmarshal(search.Query, &state)

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"search_id":        searchID,
		"status":           search.Status,
		"source_file_name": state.SourceFileName,
		"uploaded_at":      state.UploadedAt,
		"market_ids":       state.MarketIDs,
		"warnings":         state.Warnings,
	})
}
