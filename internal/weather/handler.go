package weather

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-playground/validator/v10"
)

// Handler handles HTTP requests for the weather resource.
type Handler struct {
	svc Service
}

// NewHandler returns a configured *Handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers weather routes on the provided mux.
// Expected to be called with a /v1 sub-router.
func (h *Handler) RegisterRoutes(mux interface {
	Get(pattern string, handlerFn http.HandlerFunc)
}) {
	mux.Get("/weather", h.getWeather)
}

func (h *Handler) getWeather(w http.ResponseWriter, r *http.Request) {
	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")

	lat, latErr := strconv.ParseFloat(latStr, 64)
	lon, lonErr := strconv.ParseFloat(lonStr, 64)

	if latErr != nil || lonErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "lat and lon must be valid decimal numbers",
		})
		return
	}

	result, err := h.svc.GetWeather(r.Context(), lat, lon)
	if err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":   "validation failed",
				"details": ve.Error(),
			})
			return
		}
		if errors.Is(err, ErrUpstreamUnavailable) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "weather service temporarily unavailable",
			})
			return
		}
		slog.ErrorContext(r.Context(), "weather handler error", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if result.CachedAt != nil {
		w.Header().Set("X-Weather-Cached-At", result.CachedAt.UTC().Format(http.TimeFormat))
	}
	if result.Source != "" {
		w.Header().Set("X-Weather-Source", result.Source)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Data)
	_, _ = w.Write([]byte("\n"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
