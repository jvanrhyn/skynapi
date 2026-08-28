package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
)

// handlerTimeout bounds how long a handler may run. It sits below the server's
// WriteTimeout so a slow dependency surfaces as a 503 rather than a dropped
// connection, and cannot pin a database connection for the full write window.
const handlerTimeout = 20 * time.Second

// Options configures a Server.
type Options struct {
	Port           int
	Version        string
	AllowedOrigins []string

	// RateLimitPerMinute caps requests per client IP each minute.
	// A value <= 0 disables rate limiting.
	RateLimitPerMinute int

	// TrustedProxyCount is how many reverse proxies sit in front of this
	// server. See clientIP for how it is applied.
	TrustedProxyCount int

	// ReadyCheck reports whether dependencies are usable. It backs /readyz;
	// when nil, /readyz behaves the same as /healthz.
	ReadyCheck func(context.Context) error
}

// Server wraps the HTTP server and router.
type Server struct {
	http *http.Server
	mux  *chi.Mux
}

// New creates a configured Server. Register your routes on the returned
// *chi.Mux before calling ListenAndServe.
func New(opts Options) *Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(clientIP(opts.TrustedProxyCount))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: opts.AllowedOrigins,
		AllowedMethods: []string{"GET", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Content-Type", "If-None-Match"},
		// Without this a cross-origin caller cannot read the freshness
		// metadata the weather handler sets; the browser strips it silently.
		ExposedHeaders: []string{"X-Weather-Cached-At", "X-Weather-Source", "ETag"},
		MaxAge:         300,
	}))
	// Logging sits above the limiter so rejected requests are still recorded —
	// otherwise a 429 short-circuits before anything reaches stdout.
	r.Use(slogMiddleware)
	if opts.RateLimitPerMinute > 0 {
		r.Use(httprate.LimitBy(
			opts.RateLimitPerMinute,
			time.Minute,
			func(r *http.Request) (string, error) {
				return httprate.CanonicalizeIP(r.RemoteAddr), nil
			},
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				slog.WarnContext(r.Context(), "rate limit exceeded",
					"client", r.RemoteAddr, "path", r.URL.Path)
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			}),
		))
	}
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(handlerTimeout))

	r.Get("/healthz", healthHandler(opts.Version))
	r.Get("/readyz", readyHandler(opts.Version, opts.ReadyCheck))

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	return &Server{
		mux: r,
		http: &http.Server{
			Addr:         fmt.Sprintf(":%d", opts.Port),
			Handler:      r,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}
}

// Mux returns the underlying chi router so callers can register routes.
func (s *Server) Mux() *chi.Mux { return s.mux }

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	slog.Info("server listening", "addr", s.http.Addr)
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// healthHandler is the liveness probe: it reports that the process is up and
// serving, and deliberately touches no dependencies.
func healthHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version,
		})
	}
}

// readyHandler is the readiness probe: it reports whether this instance can
// actually serve traffic, so an orchestrator can take a database-less instance
// out of rotation instead of routing requests it will fail.
func readyHandler(version string, check func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := check(ctx); err != nil {
				slog.ErrorContext(ctx, "readiness check failed", "error", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"status":  "unavailable",
					"version": version,
					"error":   "database unreachable",
				})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ready",
			"version": version,
		})
	}
}

func slogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
