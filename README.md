# skynapi

A Go REST API that combines city search (Geonames PostgreSQL database) with weather forecasts from [api.met.no](https://api.met.no), cached in PostgreSQL to respect rate limits and provide resilience.

## Features

- **City search** — fuzzy name matching via `pg_trgm` + ILIKE fallback, ranked by similarity weighted by population, with pagination
- **Weather** — cache-first forecast retrieval; concurrent requests for one coordinate collapse into a single upstream call; serves stale cache if upstream is unavailable
- **Reverse geocoding** — Nominatim proxied and cached server-side, with coordinates rounded before they leave the service
- **Structured JSON logging** via `slog`
- **Graceful shutdown**, liveness and readiness endpoints, 404/405 handlers
- **Version metadata** injected at build time via `ldflags`

## Prerequisites

| Requirement | Version |
|-------------|---------|
| Go | 1.26.1+ |
| PostgreSQL | 14+ (with `pg_trgm` extension) |
| Geonames data | loaded into `all_countries` table |

## Quick start

### Local

```bash
cp config.yaml.example config.yaml
# Edit config.yaml with your database credentials

make build
./bin/skynapi
```

### Docker

The compose stack includes Postgres, the API, and Caddy:

```bash
docker-compose up -d

# Verify through Caddy
curl http://localhost/healthz
```

The compose stack publishes Caddy on port `80` and Postgres on port `5432` by default. Caddy serves the static landing page at `http://localhost/` and proxies `/healthz` plus API routes under `http://localhost/v1/*`. The API container listens on `8080` inside the Docker network.

Default Docker database settings are `POSTGRES_DB=skyn`, `POSTGRES_USER=skynapi`, and `POSTGRES_PASSWORD=skynapi`. Override them with environment variables or `docker-compose.override.yml`; keep `DB_URL` aligned with those values.

On first startup with an empty Postgres volume, Docker runs [initdb/010-run-migrations.sh](initdb/010-run-migrations.sh), which applies every `migrations/*.up.sql` file in lexical order. This creates the database objects the API expects, including the weather cache, country-code lookup table, and the minimal `all_countries` schema. Migration `000` includes a bundled GeoNames seed. Later data migrations refresh coverage without replacing the original seed.

Build args (`VERSION`, `COMMIT`, `BUILD_TIME`) are passed automatically by docker-compose via the environment, or you can set them explicitly:

```bash
VERSION=1.2.3 COMMIT=$(git rev-parse --short HEAD) docker-compose up --build -d
```

### PaaS deployment

Use [docker-compose.paas.yaml](docker-compose.paas.yaml) for hosted Compose deployments such as Virtuozzo. It pulls prebuilt images from GHCR for the API, Caddy, and the one-shot migrations container.

Set at least these environment variables in the platform:

```bash
GHCR_OWNER=your-github-user-or-org
IMAGE_TAG=latest
POSTGRES_PASSWORD=replace-with-a-strong-password
MET_USER_AGENT="skynapi/1.0 (you@example.com)"
```

The PaaS stack runs `postgres`, then `migrate`, then `skynapi`, with Caddy exposed on `HTTP_PORT` (`80` by default).

## Configuration

Copy `config.yaml.example` to `config.yaml` (git-ignored). All keys can be overridden with environment variables:

| YAML key | Env var | Default | Description |
|----------|---------|---------|-------------|
| `server.port` | `SERVER_PORT` | `8080` | HTTP listen port |
| `server.cors_allowed_origins` | `SERVER_CORS_ALLOWED_ORIGINS` | `http://localhost:8081`, `http://127.0.0.1:8081` | Comma-separated CORS allowlist |
| `server.rate_limit_per_minute` | `SERVER_RATE_LIMIT_PER_MINUTE` | `120` | Per-client-IP request cap per minute; `0` disables |
| `server.trusted_proxy_count` | `SERVER_TRUSTED_PROXY_COUNT` | `1` | Reverse proxies in front of the API — see below |
| `db.url` | `DB_URL` | `postgres://localhost/skyn` | PostgreSQL DSN (URL or key=value) |
| `met.user_agent` | `MET_USER_AGENT` | see example | User-Agent sent to api.met.no (required by their ToS) |
| `met.base_url` | `MET_BASE_URL` | `https://api.met.no/…` | MET API base URL |
| `nominatim.user_agent` | `NOMINATIM_USER_AGENT` | see example | User-Agent sent to Nominatim (required by their usage policy) |
| `nominatim.base_url` | `NOMINATIM_BASE_URL` | `https://nominatim.openstreetmap.org` | Reverse-geocoding base URL |
| `log.level` | `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

A malformed numeric override (`SERVER_PORT=8O80`) fails startup rather than silently falling back to the default.

### Trusted proxies and rate limiting

The rate limiter keys on the client IP, which behind a proxy has to come from `X-Forwarded-For`. Only the entry the *outermost trusted proxy* appended can be believed — everything to its left is supplied by the client. `server.trusted_proxy_count` says how many proxies to count back from the right.

- **`1` (default)** — correct for the shipped Compose stack, where Caddy is the only hop.
- **`0`** — set this when the API is exposed directly. Forwarding headers are then ignored entirely and the TCP peer address is used.

Getting this wrong in either direction has a cost: too high and a client can forge the header to give itself a fresh bucket on every request; too low and every caller shares the proxy's single bucket. `True-Client-IP` and `X-Real-IP` are never trusted — they carry no hop history, so a forged value is indistinguishable from a real one — and the Caddyfile strips them at the edge as well.

> **Note**: Use `key=value` DSN format when the password contains characters special to URLs (e.g. `>`, `*`, `@`):
> ```
> host=localhost dbname=skyn user=postgres password=Ak47>Ninja* sslmode=disable
> ```

## API

API resource routes are under `/v1`. The health endpoint is registered at the root path.

### `GET /healthz`

Liveness. Reports that the process is up and deliberately touches no dependencies.

```jsonc
// 200 OK
{ "status": "ok", "version": "1.2.3" }
```

### `GET /readyz`

Readiness. Pings Postgres, so an instance that cannot serve traffic can be taken out of rotation instead of being sent requests it will fail.

```jsonc
// 200 OK
{ "status": "ready", "version": "1.2.3" }
// 503 Service Unavailable
{ "status": "unavailable", "version": "1.2.3", "error": "database unreachable" }
```

### `GET /v1/cities`

Fuzzy city search against the Geonames `all_countries` table.

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `q` | ✅ | — | Search term (2–100 chars) |
| `page` | | `1` | Page number (1-based) |
| `limit` | | `20` | Results per page (max 100) |

```bash
curl "http://localhost:8080/v1/cities?q=amsterdam&limit=5"
```

```jsonc
// 200 OK
{
  "cities": [
    { "id": 2759794, "name": "Amsterdam", "country": "NL", "region": "NH",
      "country_name": "Netherlands", "lat": 52.374, "lon": 4.8897,
      "timezone": "Europe/Amsterdam" }
  ],
  "total": 12,
  "page": 1,
  "limit": 5
}
```

Results are ranked by trigram similarity weighted by population, so `q=paris` surfaces Paris, France ahead of the several dozen other places called Paris. `total` saturates at 1000 rather than counting every match.

**Error responses**: `422` on missing `q` or a `q` shorter than two characters.

### `GET /v1/weather`

Cache-first weather forecast via [api.met.no locationforecast/2.0](https://api.met.no/weatherapi/locationforecast/2.0/documentation). Coordinates are normalised to 4 decimal places before cache lookup.

| Parameter | Required | Range | Description |
|-----------|----------|-------|-------------|
| `lat` | ✅ | -90 – 90 | Latitude |
| `lon` | ✅ | -180 – 180 | Longitude |

```bash
curl "http://localhost:8080/v1/weather?lat=52.3676&lon=4.9041"
```

Returns the api.met.no GeoJSON `Feature` response verbatim (cached in PostgreSQL — the bytes are never re-serialised, so fields MET adds later reach clients without a code change). Falls back to stale cached data if the upstream is temporarily unavailable. Returns `503` only when cache is empty and upstream is down.

Concurrent requests for the same coordinate share a single upstream fetch, so an expiring cache entry cannot fan out into a burst of identical calls against the MET quota.

Response headers:

| Header | Description |
|--------|-------------|
| `ETag` | Weak validator; send it back as `If-None-Match` to get a `304` |
| `Cache-Control` | `public, max-age=…` until the forecast expires; `no-cache` when stale |
| `X-Weather-Cached-At` | When the forecast was stored (HTTP-date) |
| `X-Weather-Source` | `upstream`, `cache`, or `stale-cache` |

`stale-cache` means api.met.no was unreachable and this is the last forecast we stored; the web UI surfaces it as a "Last known" badge.

**Error responses**: `422` on missing/invalid coordinates, `503` on upstream failure with no cache.

### `GET /v1/reverse`

Resolves coordinates to a place name via Nominatim, cached in PostgreSQL.

| Parameter | Required | Range | Description |
|-----------|----------|-------|-------------|
| `lat` | ✅ | -90 – 90 | Latitude |
| `lon` | ✅ | -180 – 180 | Longitude |

```bash
curl "http://localhost:8080/v1/reverse?lat=-26.2041&lon=28.0473"
```

```jsonc
// 200 OK
{ "label": "Johannesburg, South Africa", "city": "Johannesburg",
  "country": "South Africa", "country_code": "ZA" }
```

This is proxied rather than called from the browser for two reasons: Nominatim's usage policy requires an identifying `User-Agent` that a page cannot set, and coordinates are rounded to 2 decimal places (~1.1 km) before they leave the service, so a user's exact GPS fix is never handed to a third party.

**Error responses**: `422` on missing/invalid coordinates, `503` on upstream failure with no cache.

## City coverage and refreshes

Search includes current GeoNames populated places (cities, towns, villages and
suburbs), even when population is zero or unknown. Exact names and alternative
names rank before fuzzy matches; population resolves otherwise similar results.
Alternative names are matched as whole names, case-insensitively. Administrative
areas, stations, abandoned settlements and historical places are excluded.

Migration `010` refreshes **13,529 South African populated places**, including
Sandton, Fourways and Bryanston. Sandton is classified as `PPLX` (a section of a
populated place) with population zero, which the previous search excluded.
Deploy both the API and migrations images: adding records alone does not remove
that old population filter. On an existing database, apply `009` then `010` with
`psql -v ON_ERROR_STOP=1`; the normal migration runner also applies them.
Index creation can delay writes, so apply during a quiet period.

Generate a new, reviewable refresh for any country using Python 3.9+:

```bash
python3 scripts/refresh_geonames.py --country ZA \
  --output migrations/011_geonames_za_refresh.up.sql
# Or use a previously downloaded country ZIP, without network access:
python3 scripts/refresh_geonames.py --country ZA --archive /path/to/ZA.zip \
  --output /tmp/za-refresh.sql
make test-city-data
```

Choose the next unused migration number and add a matching `.down.sql` explaining
that restoring old data requires a backup (see `010`). The generator refuses to
overwrite existing migrations. It validates the country, record structure, IDs,
coordinates and dates before generating SQL. Each file records the source URL,
retrieval date, archive SHA-256 and attribution. Imports run in a transaction,
upsert by GeoNames ID and preserve rows with newer modification dates. Reapplying
the same refresh is safe. No live database is contacted by the generator.

Use full **country extracts**, rather than population-filtered `cities500` dumps,
to retain suburbs such as Sandton. GeoNames publishes daily extracts; periodically
regenerate and review a new migration for the countries you want to refresh.
This release refreshes South Africa; other countries retain the bundled coverage.
The refresh is additive: it does not delete IDs absent from an extract. Upstream
deletions need a separately reviewed cleanup, so this is not a guarantee of
complete or perfectly current coverage. There is no scheduled refresh job.

Data source: [GeoNames daily extracts](https://download.geonames.org/export/dump/),
[format and coverage](https://download.geonames.org/export/dump/readme.txt),
licensed under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).

PostgreSQL regression tests are opt-in. Use a disposable database with migrations
`000`, `001`, `004`, `009` and `010` applied (create the `skynapi` role first):

```bash
CITY_TEST_DATABASE_URL='postgres://…' go test ./internal/city -race -count=1
```

The tests verify Sandton/Fourways/Bryanston from the refreshed data, zero/null
population, alias lookup, exact-name ranking, population tie-breaking, excluded
features and literal wildcard handling. Search fixtures use a temporary table.

## Migrations

Migrations are plain SQL files in `migrations/`. Apply them in order with `psql` or via the postgres Docker container:

Migrations `003` and `004` grant access to a database role named `skynapi`. Create that role first, or adjust those grants if your app connects as a different database user.

```bash
# Locally — applies every *.up.sql in order
DB_URL="postgres://skynapi:…@localhost:5432/skyn" make migrate-up

# Via Docker (if psql is not installed locally)
for f in migrations/*.up.sql; do
  docker exec -i skyn_postgres psql -v ON_ERROR_STOP=1 -U skynapi -d skyn < "$f"
done
```

| Migration | Description |
|-----------|-------------|
| `000_geonames_schema` | Creates `all_countries` and loads the bundled GeoNames seed |
| `001_pg_trgm` | Installs `pg_trgm` extension; creates GIN trigram indexes on `all_countries.name` and `all_countries.asciiname` |
| `002_weather_cache` | Creates `weather_cache` table with `UNIQUE(lat, lon)` and TTL column |
| `003_permissions` | Grants the `skynapi` app user access to `weather_cache` and its sequence |
| `004_country_codes` | Creates and seeds `country_codes`, used to return `country_name` in city results |
| `005_city_search_population` | Partial trigram indexes restricted to `population > 0`, plus a population index for ranking |
| `006_reverse_geocode_cache` | Creates `reverse_geocode_cache` with `UNIQUE(lat, lon)` |
| `007_permissions_cleanup_geocode` | Grants `DELETE` on `weather_cache` (the eviction job needs it) and full access to `reverse_geocode_cache` |
| `008_weather_cache_raw_body` | Changes `weather_cache.response_body` from `JSONB` to `TEXT` so upstream bytes are preserved exactly and the response `ETag` stays stable |

## Development

```bash
make build      # compile with ldflags → bin/skynapi
make test       # go test ./... -race -count=1
make test-web   # forecast calculation tests (Node.js 22+; no npm install needed)
make lint       # golangci-lint (must be installed separately)
make clean      # remove bin/
```

The browser imports `caddy/html/forecast.mjs` directly as an ES module. It
contains the forecast calculations independently of the DOM; its optional
`now` argument makes date-sensitive tests deterministic. Tests cover timezone
boundaries, daylight saving changes, partial days, unit conversion, and rain
intervals. Serve the site through HTTP (for example, Caddy), rather than opening
the HTML file directly.

The extraction preserves existing display rules: days with fewer than two
samples are omitted, and six-hour precipitation totals are assigned to the
local day on which their interval starts. Partial-day summaries describe only
the available forecast samples.

Work on feature branches and obtain explicit user approval before merging;
see [AGENTS.md](AGENTS.md).

### Project layout

```
cmd/api/          # main entry point
internal/
  config/         # YAML + env config loader
  db/             # pgxpool factory
  server/         # chi router, middleware, client-IP resolution, health/readiness
  city/           # city search (model, repo, service, handler, tests)
  weather/        # weather cache (model, repo, MET client, service, handler, tests)
  geocode/        # reverse geocoding (model, repo, Nominatim client, service, handler, tests)
migrations/       # .up.sql / .down.sql pairs
api/              # openapi.yaml (OpenAPI 3.0)
.http/            # httpYac / REST Client request collection
```

## Testing the API

The `.http/` folder contains pre-built requests covering every endpoint and error path. Requires [httpYac](https://httpyac.github.io/) or a compatible IDE extension.

```bash
npm install -g httpyac
httpyac send .http/skynapi.http --all --env local
```

See [`.http/README.md`](.http/README.md) for full tooling instructions.

## OpenAPI spec

The full spec is at [`api/openapi.yaml`](api/openapi.yaml). View it with:

```bash
npx @redocly/cli preview-docs api/openapi.yaml
# or
docker run -p 8081:80 -e SPEC_URL=/openapi.yaml \
  -v $(pwd)/api:/usr/share/nginx/html swaggerapi/swagger-ui
```
