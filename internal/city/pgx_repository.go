package city

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxRepository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository backed by pgxpool.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

// Search performs a fuzzy city search using pg_trgm similarity operators.
// It matches against name, asciiname, country_code, and admin1_code.
// Results are ordered alphabetically and paginated.
func (r *pgxRepository) Search(ctx context.Context, params SearchParams) ([]City, int, error) {
	const query = `
		SELECT
			geonameid,
			name,
			COALESCE(country_code, ''),
			COALESCE(admin1_code, ''),
			latitude,
			longitude,
			COALESCE(timezone, ''),
			COUNT(*) OVER () AS total_count
		FROM all_countries
		WHERE
			name        % $1
			OR asciiname % $1
			OR name      ILIKE '%' || $1 || '%'
		ORDER BY
			GREATEST(similarity(name, $1), similarity(asciiname, $1)) DESC,
			name ASC
		LIMIT  $2
		OFFSET $3`

	offset := (params.Page - 1) * params.Limit

	rows, err := r.pool.Query(ctx, query, params.Q, params.Limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("city: search query: %w", err)
	}
	defer rows.Close()

	var cities []City
	var total int

	for rows.Next() {
		var c City
		if err := rows.Scan(
			&c.GeonameID, &c.Name, &c.CountryCode, &c.Region,
			&c.Lat, &c.Lon, &c.Timezone, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("city: scan row: %w", err)
		}
		cities = append(cities, c)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("city: rows error: %w", err)
	}

	return cities, total, nil
}
