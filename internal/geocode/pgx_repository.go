package geocode

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (r *pgxRepository) Get(ctx context.Context, lat, lon float64) (*CachedPlace, error) {
	const query = `
		SELECT lat, lon, cached_at, label, city, country, country_code
		FROM reverse_geocode_cache
		WHERE lat = $1 AND lon = $2`

	var p CachedPlace
	err := r.pool.QueryRow(ctx, query, NormaliseCoord(lat), NormaliseCoord(lon)).Scan(
		&p.Lat, &p.Lon, &p.CachedAt,
		&p.Place.Label, &p.Place.City, &p.Place.Country, &p.Place.CountryCode,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("geocode: cache get: %w", err)
	}
	return &p, nil
}

func (r *pgxRepository) Set(ctx context.Context, p *CachedPlace) error {
	const query = `
		INSERT INTO reverse_geocode_cache (lat, lon, cached_at, label, city, country, country_code)
		VALUES ($1, $2, NOW(), $3, $4, $5, $6)
		ON CONFLICT (lat, lon) DO UPDATE SET
		    cached_at    = EXCLUDED.cached_at,
		    label        = EXCLUDED.label,
		    city         = EXCLUDED.city,
		    country      = EXCLUDED.country,
		    country_code = EXCLUDED.country_code`

	_, err := r.pool.Exec(ctx, query,
		NormaliseCoord(p.Lat), NormaliseCoord(p.Lon),
		p.Place.Label, p.Place.City, p.Place.Country, p.Place.CountryCode,
	)
	if err != nil {
		return fmt.Errorf("geocode: cache set: %w", err)
	}
	return nil
}

func (r *pgxRepository) DeleteStale(ctx context.Context, retention time.Duration) (int64, error) {
	const query = `DELETE FROM reverse_geocode_cache WHERE cached_at < $1`

	tag, err := r.pool.Exec(ctx, query, time.Now().Add(-retention))
	if err != nil {
		return 0, fmt.Errorf("geocode: delete stale cache: %w", err)
	}
	return tag.RowsAffected(), nil
}
