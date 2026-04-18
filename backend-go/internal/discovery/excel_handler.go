package discovery

import (
	"encoding/json"
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
