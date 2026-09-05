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

// maxCountedMatches bounds the total reported by Search. Counting every match
// for a broad query means walking the whole trigram result set just to serve
// one page, so the reported total saturates here instead.
const maxCountedMatches = 1000

// Search current populated places, including suburbs without census counts.
// Exact names and aliases rank first; population breaks relevance ties.
// The CTE shares the match set with the bounded count (at most 1000).
// Partial trigram indexes are provided by migration 009.
const searchQuery = `
	WITH matched AS (
	    SELECT
	        ac.geonameid,
	        ac.name,
	        ac.country_code,
	        ac.admin1_code,
	        ac.latitude,
	        ac.longitude,
	        ac.timezone,
	        COALESCE(ac.population, 0) AS population,
	        (lower(COALESCE(ac.name, '')) = lower($1) OR lower(COALESCE(ac.asciiname, '')) = lower($1)
	         OR (',' || COALESCE(ac.alternatenames, '') || ',')
	              ILIKE '%,' || $2 || ',%' ESCAPE '\') AS exact_match,
	        GREATEST(similarity(ac.name, $1), similarity(ac.asciiname, $1))
	            * (1 + ln(GREATEST(COALESCE(ac.population, 0), 1)) / 20) AS score
	    FROM all_countries ac
	    WHERE
	        (ac.name        % $1
	         OR ac.asciiname % $1
	         OR ac.name      ILIKE '%' || $2 || '%' ESCAPE '\'
	         OR ac.asciiname ILIKE '%' || $2 || '%' ESCAPE '\'
	         OR (',' || COALESCE(ac.alternatenames, '') || ',')
	              ILIKE '%,' || $2 || ',%' ESCAPE '\')
	        AND ac.feature_class = 'P'
	        AND ac.feature_code IN
	        ('PPL','PPLA','PPLA2','PPLA3','PPLA4','PPLA5','PPLC','PPLF','PPLG','PPLL','PPLR','PPLS','PPLX','STLMT')
	)
	SELECT
	    m.geonameid,
	    m.name,
	    COALESCE(m.country_code, ''),
	    COALESCE(m.admin1_code, ''),
	    m.latitude,
	    m.longitude,
	    COALESCE(m.timezone, ''),
	    COALESCE(cc.name, '') AS country_name,
	    (SELECT count(*) FROM (SELECT 1 FROM matched LIMIT $5) capped) AS total_count
	FROM matched m
	LEFT JOIN public.country_codes cc ON cc.alpha_2 = m.country_code
	ORDER BY
	    m.exact_match DESC,
	    m.score DESC,
	    m.population DESC,
	    m.name ASC,
	    m.geonameid ASC
	LIMIT  $3
	OFFSET $4`

// likeEscaper neutralises LIKE wildcards in user input. The trigram operators
// take the raw query instead, since % and _ carry no special meaning there.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (r *pgxRepository) Search(ctx context.Context, params SearchParams) ([]City, int, error) {
	offset := (params.Page - 1) * params.Limit

	rows, err := r.pool.Query(ctx, searchQuery,
		params.Q,                      // $1 raw, for the trigram operators
		likeEscaper.Replace(params.Q), // $2 escaped, for the ILIKE fallbacks
		params.Limit,                  // $3
		offset,                        // $4
		maxCountedMatches,             // $5
	)
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
