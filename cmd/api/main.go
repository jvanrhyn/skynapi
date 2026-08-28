package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jvanrhyn/skynapi/internal/city"
	"github.com/jvanrhyn/skynapi/internal/config"
	appdb "github.com/jvanrhyn/skynapi/internal/db"
	"github.com/jvanrhyn/skynapi/internal/geocode"
	"github.com/jvanrhyn/skynapi/internal/server"
	"github.com/jvanrhyn/skynapi/internal/weather"
)

// Injected at build time via ldflags.
var (
	Version    = "dev"
	CommitHash = "unknown"
	BuildTime  = "unknown"
)

const (
	// cacheCleanupInterval is how often stale cache rows are evicted.
	cacheCleanupInterval = 6 * time.Hour
	// weatherRetention is the age past which unused forecast rows are removed.
	weatherRetention = 7 * 24 * time.Hour
	// geocodeRetention is longer: place names for a coordinate rarely change,
	// so entries are worth keeping well beyond a forecast's usefulness.
	geocodeRetention = 90 * 24 * time.Hour
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// run holds the whole lifecycle so every deferred cleanup executes; main only
// translates a failure into an exit code.
func run() error {
	ctx := context.Background()

	cfg, err := config.Load("config.yaml")
	if err != nil {
		return err
	}

	setupLogger(cfg.Log.Level)

	slog.Info("starting skynapi",
		"version", Version,
		"commit", CommitHash,
		"built", BuildTime,
	)

	pool, err := appdb.NewPool(ctx, cfg.DB.URL)
	if err != nil {
		return err
	}
	defer pool.Close()

	cityRepo := city.NewRepository(pool)
	cityHandler := city.NewHandler(city.NewService(cityRepo))

	weatherRepo := weather.NewRepository(pool)
	weatherClient := weather.NewClient(cfg.MET.BaseURL, cfg.MET.UserAgent)
	weatherHandler := weather.NewHandler(weather.NewService(weatherRepo, weatherClient))

	geocodeRepo := geocode.NewRepository(pool)
	geocodeClient := geocode.NewClient(cfg.Nominatim.BaseURL, cfg.Nominatim.UserAgent)
	geocodeHandler := geocode.NewHandler(geocode.NewService(geocodeRepo, geocodeClient))

	// Periodically evict cache rows not used within their retention window to
	// bound table growth from distinct-coordinate lookups.
	cleanupCtx, stopCleanup := context.WithCancel(ctx)
	defer stopCleanup()
	go runCacheCleanup(cleanupCtx, "weather", weatherRepo.DeleteStale, cacheCleanupInterval, weatherRetention)
	go runCacheCleanup(cleanupCtx, "geocode", geocodeRepo.DeleteStale, cacheCleanupInterval, geocodeRetention)

	srv := server.New(server.Options{
		Port:               cfg.Server.Port,
		Version:            Version,
		AllowedOrigins:     cfg.Server.CORSAllowedOrigins,
		RateLimitPerMinute: cfg.Server.RateLimitPerMinute,
		TrustedProxyCount:  cfg.Server.TrustedProxyCount,
		ReadyCheck:         dbReadyCheck(pool),
	})

	srv.Mux().Route("/v1", func(r chi.Router) {
		cityHandler.RegisterRoutes(r)
		weatherHandler.RegisterRoutes(r)
		geocodeHandler.RegisterRoutes(r)
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-quit:
		slog.Info("shutting down")
	}

	shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		return err
	}
	slog.Info("server stopped")
	return nil
}

// dbReadyCheck backs /readyz. Every endpoint that matters needs Postgres, so an
// instance that cannot reach it should be taken out of rotation rather than
// served requests it will fail.
func dbReadyCheck(pool *pgxpool.Pool) func(context.Context) error {
	return func(ctx context.Context) error { return pool.Ping(ctx) }
}

// runCacheCleanup evicts stale rows on a fixed interval until the context is
// cancelled. deleteStale reports how many rows were removed.
func runCacheCleanup(ctx context.Context, name string, deleteStale func(context.Context, time.Duration) (int64, error), interval, retention time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := deleteStale(ctx, retention)
			if err != nil {
				slog.ErrorContext(ctx, "cache cleanup failed", "cache", name, "error", err)
				continue
			}
			if deleted > 0 {
				slog.InfoContext(ctx, "cache cleanup", "cache", name, "deleted", deleted)
			}
		}
	}
}

func setupLogger(level string) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
