package weather

import (
	"encoding/json"
	"errors"
	"hash/fnv"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	// Let browsers and the edge proxy absorb repeat reads until the upstream
	// forecast expires. Stale data (expiry in the past) is marked non-cacheable.
	if ttl := time.Until(cacheExpiry(result.ExpiresAt)); ttl > 0 {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(ttl.Seconds())))
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	// A repeat viewer whose copy still matches gets a 304 instead of ~100 KB.
	etag := weakETag(result.Data)
	w.Header().Set("ETag", etag)
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
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

// cacheExpiry returns the given expiry, or a zero (past) time when nil so the
// caller treats missing expiry as non-cacheable.
func cacheExpiry(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// weakETag derives a validator from the response body. It is weak because the
// same forecast may be served from cache and from upstream with byte-identical
// content but different freshness metadata.
func weakETag(data []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return `W/"` + strconv.FormatUint(h.Sum64(), 16) + `"`
}

// matchesETag reports whether an If-None-Match header covers etag. Weak
// comparison is used, per RFC 9110 section 13.1.2.
func matchesETag(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == want {
			return true
		}
	}
	return false
}
