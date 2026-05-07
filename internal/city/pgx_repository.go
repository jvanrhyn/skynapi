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

// Search performs a fuzzy city search using pg_trgm similarity operators for
// typo-tolerance combined with ILIKE for substring matching. Results are
// ordered by trigram similarity descending, then alphabetically.
//
// Requires: pg_trgm extension + GIN indexes (migrations/001_pg_trgm.up.sql).
func (r *pgxRepository) Search(ctx context.Context, params SearchParams) ([]City, int, error) {
	const query = `
		SELECT
			ac.geonameid,
			ac.name,
			COALESCE(ac.country_code, ''),
			COALESCE(ac.admin1_code, ''),
			ac.latitude,
			ac.longitude,
			COALESCE(ac.timezone, ''),
			COALESCE(cc.name, '') AS country_name,
			COUNT(*) OVER () AS total_count
		FROM all_countries ac
		LEFT JOIN public.country_codes cc ON cc.alpha_2 = ac.country_code
		WHERE
			ac.name        % $1
			OR ac.asciiname % $1
			OR ac.name      ILIKE '%' || $2 || '%' ESCAPE '\'
			OR ac.asciiname ILIKE '%' || $2 || '%' ESCAPE '\'
		ORDER BY
			GREATEST(similarity(ac.name, $1), similarity(ac.asciiname, $1)) DESC,
			ac.name ASC
		LIMIT  $3
		OFFSET $4`

	// $1: raw query for trgm operators (% and _ are not special in trgm).
	// $2: LIKE-escaped query for ILIKE clauses.
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(params.Q)
	offset := (params.Page - 1) * params.Limit

	rows, err := r.pool.Query(ctx, query, params.Q, escaped, params.Limit, offset)
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
			&c.Lat, &c.Lon, &c.Timezone, &c.CountryName, &total,
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
