package geocode

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
)

// Handler handles HTTP requests for the reverse-geocode resource.
type Handler struct {
	svc Service
}

// NewHandler returns a configured *Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers geocode routes on the provided mux.
// Expected to be called with a /v1 sub-router.
func (h *Handler) RegisterRoutes(mux interface {
	Get(pattern string, handlerFn http.HandlerFunc)
}) {
	mux.Get("/reverse", h.reverse)
}

func (h *Handler) reverse(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")

	if latStr == "" || lonStr == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "lat and lon are required",
		})
		return
	}

	lat, latErr := strconv.ParseFloat(latStr, 64)
	lon, lonErr := strconv.ParseFloat(lonStr, 64)
	if latErr != nil || lonErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "lat and lon must be valid decimal numbers",
		})
		return
	}

	place, err := h.svc.Reverse(r.Context(), lat, lon)
	if err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":   "validation failed",
				"details": ve.Error(),
			})
			return
		}
		if errors.Is(err, ErrUnavailable) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "reverse geocoding temporarily unavailable",
			})
			return
		}
		slog.ErrorContext(r.Context(), "geocode handler error", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	// A place name for a rounded coordinate is effectively static.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	writeJSON(w, http.StatusOK, place)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
