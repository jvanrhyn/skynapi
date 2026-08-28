package server_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jvanrhyn/skynapi/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// The handlers log every request; keep it out of the test output.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func newTestServer(t *testing.T, opts server.Options) http.Handler {
	t.Helper()
	if opts.Version == "" {
		opts.Version = "test"
	}
	srv := server.New(opts)
	srv.Mux().Get("/v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return srv.Mux()
}

// A forged forwarding header must not give a caller its own rate-limit bucket.
// This is the regression that made the limiter — the guard on the api.met.no
// quota — bypassable with a single header.
func TestRateLimit_NotBypassableBySpoofedHeaders(t *testing.T) {
	const limit = 3

	tests := []struct {
		name   string
		forge  func(r *http.Request, i int)
		reason string
	}{
		{
			name:   "no forgery",
			forge:  func(r *http.Request, _ int) {},
			reason: "baseline: one client, one bucket",
		},
		{
			name:   "rotating True-Client-IP",
			forge:  func(r *http.Request, i int) { r.Header.Set("True-Client-IP", fmt.Sprintf("10.9.0.%d", i)) },
			reason: "single-value header carries no hop history and must be ignored",
		},
		{
			name:   "rotating X-Real-IP",
			forge:  func(r *http.Request, i int) { r.Header.Set("X-Real-IP", fmt.Sprintf("10.9.1.%d", i)) },
			reason: "as above",
		},
		{
			name: "rotating client-supplied X-Forwarded-For prefix",
			forge: func(r *http.Request, i int) {
				// What a client can actually achieve behind Caddy: it controls
				// the left of the list, the proxy appends the real address.
				r.Header.Set("X-Forwarded-For", fmt.Sprintf("10.9.2.%d, 203.0.113.7", i))
			},
			reason: "only the entry the trusted proxy appended may be used",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestServer(t, server.Options{RateLimitPerMinute: limit, TrustedProxyCount: 1})

			var lastStatus int
			for i := 0; i < limit+2; i++ {
				req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
				req.RemoteAddr = "203.0.113.7:44444"
				if tc.forge == nil {
					t.Fatal("forge must be set")
				}
				// Behind one trusted proxy the real address always arrives as
				// the final X-Forwarded-For entry.
				req.Header.Set("X-Forwarded-For", "203.0.113.7")
				tc.forge(req, i)

				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				lastStatus = rec.Code
			}
			assert.Equal(t, http.StatusTooManyRequests, lastStatus,
				"requests past the limit must be rejected — %s", tc.reason)
		})
	}
}

// Distinct clients must still get distinct buckets, or the limiter would be a
// shared ceiling that one busy caller could use to lock everyone else out.
func TestRateLimit_SeparateBucketsPerRealClient(t *testing.T) {
	h := newTestServer(t, server.Options{RateLimitPerMinute: 2, TrustedProxyCount: 1})

	exhaust := func(ip string) int {
		var last int
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
			req.Header.Set("X-Forwarded-For", ip)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			last = rec.Code
		}
		return last
	}

	assert.Equal(t, http.StatusTooManyRequests, exhaust("198.51.100.1"))

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.2")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "a different client must have its own bucket")
}

// With no trusted proxy configured, forwarding headers must be ignored entirely.
func TestTrustedProxyCountZero_IgnoresForwardingHeaders(t *testing.T) {
	h := newTestServer(t, server.Options{RateLimitPerMinute: 2, TrustedProxyCount: 0})

	var last int
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
		req.RemoteAddr = "203.0.113.9:5555"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		last = rec.Code
	}
	assert.Equal(t, http.StatusTooManyRequests, last)
}

// The weather handler's freshness headers are useless cross-origin unless CORS
// exposes them; browsers strip unlisted headers without an error.
func TestCORS_ExposesWeatherHeaders(t *testing.T) {
	h := newTestServer(t, server.Options{AllowedOrigins: []string{"http://localhost:8081"}})

	req := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Header names are case-insensitive and the CORS library canonicalises them.
	exposed := strings.ToLower(rec.Header().Get("Access-Control-Expose-Headers"))
	for _, want := range []string{"x-weather-cached-at", "x-weather-source", "etag"} {
		assert.Contains(t, exposed, want)
	}
}

func TestHealthAndReadiness(t *testing.T) {
	t.Run("healthz ignores dependencies", func(t *testing.T) {
		h := newTestServer(t, server.Options{
			ReadyCheck: func(context.Context) error { return errors.New("db down") },
		})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("readyz fails when the database is unreachable", func(t *testing.T) {
		h := newTestServer(t, server.Options{
			ReadyCheck: func(context.Context) error { return errors.New("db down") },
		})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Contains(t, rec.Body.String(), "unavailable")
	})

	t.Run("readyz passes when the database answers", func(t *testing.T) {
		h := newTestServer(t, server.Options{ReadyCheck: func(context.Context) error { return nil }})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "ready")
	})
}

func TestNotFoundAndMethodNotAllowed(t *testing.T) {
	h := newTestServer(t, server.Options{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "not found")

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/ping", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	require.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json"))
}
