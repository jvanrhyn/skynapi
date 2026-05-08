# skynapi

A Go REST API that combines city search (Geonames PostgreSQL database) with weather forecasts from [api.met.no](https://api.met.no), cached in PostgreSQL to respect rate limits and provide resilience.

## Features

- **City search** — fuzzy name matching via `pg_trgm` + ILIKE fallback, with pagination
- **Weather** — cache-first forecast retrieval; serves stale cache if upstream is unavailable
- **Structured JSON logging** via `slog`
- **Graceful shutdown**, health endpoint, 404/405 handlers
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

On first startup with an empty Postgres volume, Docker runs [initdb/010-run-migrations.sh](initdb/010-run-migrations.sh), which applies every `migrations/*.up.sql` file in lexical order. This creates the database objects the API expects, including the weather cache, country-code lookup table, and the minimal `all_countries` schema. City search returns useful data only after Geonames rows have been loaded into `all_countries`.

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
| `db.url` | `DB_URL` | `postgres://localhost/skyn` | PostgreSQL DSN (URL or key=value) |
| `met.user_agent` | `MET_USER_AGENT` | see example | User-Agent sent to api.met.no (required by their ToS) |
| `met.base_url` | `MET_BASE_URL` | `https://api.met.no/…` | MET API base URL |
| `log.level` | `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

> **Note**: Use `key=value` DSN format when the password contains characters special to URLs (e.g. `>`, `*`, `@`):
> ```
> host=localhost dbname=skyn user=postgres password=Ak47>Ninja* sslmode=disable
> ```

## API

API resource routes are under `/v1`. The health endpoint is registered at the root path.

### `GET /healthz`

Returns server health and version. No auth required.

```jsonc
// 200 OK
{ "status": "ok", "version": "1.2.3" }
```

### `GET /v1/cities`

Fuzzy city search against the Geonames `all_countries` table.

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `q` | ✅ | — | Search term (1–100 chars) |
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

**Error responses**: `422` on missing/invalid `q`.

### `GET /v1/weather`

Cache-first weather forecast via [api.met.no locationforecast/2.0](https://api.met.no/weatherapi/locationforecast/2.0/documentation). Coordinates are normalised to 4 decimal places before cache lookup.

| Parameter | Required | Range | Description |
|-----------|----------|-------|-------------|
| `lat` | ✅ | -90 – 90 | Latitude |
| `lon` | ✅ | -180 – 180 | Longitude |

```bash
curl "http://localhost:8080/v1/weather?lat=52.3676&lon=4.9041"
```

Returns the raw api.met.no GeoJSON `Feature` response (cached in PostgreSQL). Falls back to stale cached data if the upstream is temporarily unavailable. Returns `503` only when cache is empty and upstream is down.

**Error responses**: `422` on missing/invalid coordinates, `503` on upstream failure with no cache.

## Migrations

Migrations are plain SQL files in `migrations/`. Apply them in order with `psql` or via the postgres Docker container:

Migrations `003` and `004` grant access to a database role named `skynapi`. Create that role first, or adjust those grants if your app connects as a different database user.

```bash
# Via Docker (if psql is not installed locally)
docker exec -i skyn_postgres psql -U skynapi -d skyn < migrations/000_geonames_schema.up.sql
docker exec -i skyn_postgres psql -U skynapi -d skyn < migrations/001_pg_trgm.up.sql
docker exec -i skyn_postgres psql -U skynapi -d skyn < migrations/002_weather_cache.up.sql
docker exec -i skyn_postgres psql -U skynapi -d skyn < migrations/003_permissions.up.sql
docker exec -i skyn_postgres psql -U skynapi -d skyn < migrations/004_country_codes.up.sql
```

| Migration | Description |
|-----------|-------------|
| `000_geonames_schema` | Creates the minimal `all_countries` table schema expected by city search |
| `001_pg_trgm` | Installs `pg_trgm` extension; creates GIN trigram indexes on `all_countries.name` and `all_countries.asciiname` |
| `002_weather_cache` | Creates `weather_cache` table with `UNIQUE(lat, lon)` and TTL column |
| `003_permissions` | Grants the `skynapi` app user access to `weather_cache` and its sequence |
| `004_country_codes` | Creates and seeds `country_codes`, used to return `country_name` in city results |

## Development

```bash
make build      # compile with ldflags → bin/skynapi
make test       # go test ./... -race -count=1
make lint       # golangci-lint (must be installed separately)
make clean      # remove bin/
```

### Project layout

```
cmd/api/          # main entry point
internal/
  config/         # YAML + env config loader
  db/             # pgxpool factory
  server/         # chi router, middleware, health handler
  city/           # city search (model, repo, service, handler, tests)
  weather/        # weather cache (model, repo, MET client, service, handler, tests)
migrations/       # .up.sql / .down.sql pairs
api/              # openapi.yaml (OpenAPI 3.0)
.http/            # httpYac / REST Client request collection
```

## Testing the API

The `.http/` folder contains 18 pre-built requests covering all endpoints and error paths. Requires [httpYac](https://httpyac.github.io/) or a compatible IDE extension.

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
