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
	appdb "github.com/jvanrhyn/skynapi/internal/db"

	"github.com/jvanrhyn/skynapi/internal/city"
	"github.com/jvanrhyn/skynapi/internal/config"
	"github.com/jvanrhyn/skynapi/internal/server"
	"github.com/jvanrhyn/skynapi/internal/weather"
)

// Injected at build time via ldflags.
var (
	Version    = "dev"
	CommitHash = "unknown"
	BuildTime  = "unknown"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load("config.yaml")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.Log.Level)

	slog.Info("starting skynapi",
		"version", Version,
		"commit", CommitHash,
		"built", BuildTime,
	)

	pool, err := appdb.NewPool(ctx, cfg.DB.URL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	cityRepo := city.NewRepository(pool)
	citySvc := city.NewService(cityRepo)
	cityHandler := city.NewHandler(citySvc)

	weatherRepo := weather.NewRepository(pool)
	weatherClient := weather.NewClient(cfg.MET.BaseURL, cfg.MET.UserAgent)
	weatherSvc := weather.NewService(weatherRepo, weatherClient)
	weatherHandler := weather.NewHandler(weatherSvc)

	srv := server.New(cfg.Server.Port, Version, cfg.Server.CORSAllowedOrigins)

	srv.Mux().Route("/v1", func(r chi.Router) {
		cityHandler.RegisterRoutes(r)
		weatherHandler.RegisterRoutes(r)
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

func setupLogger(level string) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
