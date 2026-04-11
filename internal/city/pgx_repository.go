package city

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxRepository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository backed by pgxpool.
func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgxRepository{pool: pool}
}

// Search performs a city search using ILIKE pattern matching against name and
// asciiname. Results are ranked: exact prefix matches first, then contains
// matches, then alphabetically. Install the pg_trgm extension and run
// migrations/001_pg_trgm.up.sql to add GIN indexes for faster ILIKE queries.
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
			name      ILIKE '%' || $1 || '%' ESCAPE '\'
			OR asciiname ILIKE '%' || $1 || '%' ESCAPE '\'
		ORDER BY
			CASE
				WHEN name      ILIKE $1 || '%' ESCAPE '\' THEN 0
				WHEN asciiname ILIKE $1 || '%' ESCAPE '\' THEN 1
				ELSE 2
			END,
			name ASC
		LIMIT  $2
		OFFSET $3`

	// Escape LIKE metacharacters so user input like "%" or "_" cannot act as
	// wildcards and cause expensive match-all scans.
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(params.Q)
	offset := (params.Page - 1) * params.Limit

	rows, err := r.pool.Query(ctx, query, escaped, params.Limit, offset)
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
