// Package handler provides HTTP handlers for the banner rotation service.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/VadimVikt/banner-rotation/internal/service"
	"github.com/go-chi/chi/v5"
)

// Handlers holds dependencies for HTTP handlers.
type Handlers struct {
	svc *service.Service
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(svc *service.Service) *Handlers {
	return &Handlers{svc: svc}
}

// RegisterRoutes registers all HTTP routes on the provided chi router.
func (h *Handlers) RegisterRoutes(r *chi.Mux) {
	r.Post("/slots", h.CreateSlot)
	r.Post("/slots/{slotID}/banners", h.AddBanner)
	r.Delete("/slots/{slotID}/banners/{bannerID}", h.RemoveBanner)
	r.Post("/slots/{slotID}/pick", h.PickBanner)
	r.Post("/slots/{slotID}/banners/{bannerID}/click", h.RegisterClick)
}

// CreateSlot handles POST /slots
// Body: {"id": "...", "description": "..."}
func (h *Handlers) CreateSlot(w http.ResponseWriter, r *http.Request) {
	if r.Context().Err() != nil {
		writeJSONError(w, http.StatusRequestTimeout, "request timed out")
		return
	}

	var req struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if req.ID == "" {
		writeJSONError(w, http.StatusBadRequest, "slot id is required")
		return
	}

	if err := h.svc.CreateSlot(req.ID, req.Description); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create slot: %v", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// AddBanner handles POST /slots/:slotID/banners
// Body: {"banner_id": "...", "description": "..."}
func (h *Handlers) AddBanner(w http.ResponseWriter, r *http.Request) {
	if r.Context().Err() != nil {
		writeJSONError(w, http.StatusRequestTimeout, "request timed out")
		return
	}

	slotID := chi.URLParam(r, "slotID")
	if slotID == "" {
		writeJSONError(w, http.StatusBadRequest, "slotID is required")
		return
	}

	var req struct {
		BannerID    string `json:"banner_id"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if req.BannerID == "" {
		writeJSONError(w, http.StatusBadRequest, "banner_id is required")
		return
	}

	if err := h.svc.AddBannerWithDescription(slotID, req.BannerID, req.Description); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to add banner: %v", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// RemoveBanner handles DELETE /slots/:slotID/banners/:bannerID
func (h *Handlers) RemoveBanner(w http.ResponseWriter, r *http.Request) {
	if r.Context().Err() != nil {
		writeJSONError(w, http.StatusRequestTimeout, "request timed out")
		return
	}

	slotID := chi.URLParam(r, "slotID")
	bannerID := chi.URLParam(r, "bannerID")
	if slotID == "" {
		writeJSONError(w, http.StatusBadRequest, "slotID is required")
		return
	}
	if bannerID == "" {
		writeJSONError(w, http.StatusBadRequest, "bannerID is required")
		return
	}

	if err := h.svc.RemoveBanner(slotID, bannerID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to remove banner: %v", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PickBanner handles POST /slots/:slotID/pick?groupID=...
// Response: {"banner_id": "..."}
func (h *Handlers) PickBanner(w http.ResponseWriter, r *http.Request) {
	if r.Context().Err() != nil {
		writeJSONError(w, http.StatusRequestTimeout, "request timed out")
		return
	}

	slotID := chi.URLParam(r, "slotID")
	groupID := r.URL.Query().Get("groupID")
	if slotID == "" {
		writeJSONError(w, http.StatusBadRequest, "slotID is required")
		return
	}

	// Add timeout to context for downstream operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	bannerID, err := h.svc.PickBanner(ctx, slotID, groupID)
	if err != nil {
		if errors.Is(err, service.ErrNoBanners) {
			writeJSONError(w, http.StatusNotFound, "no banners available for slot")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to pick banner: %v", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"banner_id": bannerID}); err != nil {
		//nolint:errcheck // headers already sent; nothing more we can do
		_ = err
	}
}

// RegisterClick handles POST /slots/:slotID/banners/:bannerID/click?groupID=...
// Response: 204 No Content
func (h *Handlers) RegisterClick(w http.ResponseWriter, r *http.Request) {
	if r.Context().Err() != nil {
		writeJSONError(w, http.StatusRequestTimeout, "request timed out")
		return
	}

	slotID := chi.URLParam(r, "slotID")
	bannerID := chi.URLParam(r, "bannerID")
	groupID := r.URL.Query().Get("groupID")
	if slotID == "" {
		writeJSONError(w, http.StatusBadRequest, "slotID is required")
		return
	}
	if bannerID == "" {
		writeJSONError(w, http.StatusBadRequest, "bannerID is required")
		return
	}

	// Add timeout to context for downstream operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.svc.RegisterClick(ctx, slotID, bannerID, groupID); err != nil {
		if errors.Is(err, service.ErrUnknownBanner) {
			writeJSONError(w, http.StatusNotFound, "banner not found in slot")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to register click: %v", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, code int, msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	//nolint:errcheck // best-effort: status code already sent
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
