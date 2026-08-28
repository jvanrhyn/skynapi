package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool sizing. The workload is short read queries plus small cache upserts, so
// connections are cheap to hold but should not accumulate unbounded across
// replicas sharing one Postgres.
const (
	maxConns          = 10
	minConns          = 2
	maxConnLifetime   = 30 * time.Minute
	maxConnIdleTime   = 5 * time.Minute
	healthCheckPeriod = 30 * time.Second
	connectTimeout    = 5 * time.Second
)

// NewPool creates a new pgxpool connection pool from the given DSN.
// Sizing and lifetimes are set explicitly rather than left to pgx defaults,
// which derive MaxConns from GOMAXPROCS and never retire idle connections.
// The pool is validated with a Ping before being returned.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.HealthCheckPeriod = healthCheckPeriod
	cfg.ConnConfig.ConnectTimeout = connectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	slog.InfoContext(ctx, "database pool ready",
		"host", cfg.ConnConfig.Host,
		"database", cfg.ConnConfig.Database,
		"max_conns", cfg.MaxConns,
	)
	return pool, nil
}
