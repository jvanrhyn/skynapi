package weather

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxRepository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository backed by pgxpool.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

// NormaliseCoord rounds a coordinate to the 4 decimal place precision
// used by api.met.no.
func NormaliseCoord(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func (r *pgxRepository) Get(ctx context.Context, lat, lon float64) (*CachedWeather, error) {
	nlat, nlon := NormaliseCoord(lat), NormaliseCoord(lon)

	const query = `
		SELECT lat, lon, cached_at, expires_at, last_modified, response_body
		FROM weather_cache
		WHERE lat = $1 AND lon = $2`

	var w CachedWeather
	err := r.pool.QueryRow(ctx, query, nlat, nlon).Scan(
		&w.Lat, &w.Lon, &w.CachedAt, &w.ExpiresAt, &w.LastModified, &w.Data,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("weather: cache get: %w", err)
	}
	return &w, nil
}

func (r *pgxRepository) Set(ctx context.Context, w *CachedWeather) error {
	nlat, nlon := NormaliseCoord(w.Lat), NormaliseCoord(w.Lon)

	const query = `
		INSERT INTO weather_cache (lat, lon, cached_at, expires_at, last_modified, response_body)
		VALUES ($1, $2, NOW(), $3, $4, $5)
		ON CONFLICT (lat, lon) DO UPDATE SET
			cached_at     = EXCLUDED.cached_at,
			expires_at    = EXCLUDED.expires_at,
			last_modified = EXCLUDED.last_modified,
			response_body = EXCLUDED.response_body`

	_, err := r.pool.Exec(ctx, query, nlat, nlon, w.ExpiresAt, w.LastModified, w.Data)
	if err != nil {
		return fmt.Errorf("weather: cache set: %w", err)
	}
	return nil
}
