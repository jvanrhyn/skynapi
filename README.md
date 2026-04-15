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

The API container needs to reach your Postgres container. Create a shared network once, then start the stack:

```bash
# One-time setup — connect your existing postgres container to the shared network
docker network create skynapi_net
docker network connect skynapi_net dev_postgres

# Start — DB_URL uses key=value DSN (avoids URL special-character issues)
DB_URL='host=dev_postgres dbname=skyn user=postgres password=secret sslmode=disable' \
  docker-compose up -d

# Verify
curl http://localhost:8080/healthz
```

Build args (`VERSION`, `COMMIT`, `BUILD_TIME`) are passed automatically by docker-compose via the environment, or you can set them explicitly:

```bash
VERSION=1.2.3 COMMIT=$(git rev-parse --short HEAD) docker-compose up --build -d
```

## Configuration

Copy `config.yaml.example` to `config.yaml` (git-ignored). All keys can be overridden with environment variables:

| YAML key | Env var | Default | Description |
|----------|---------|---------|-------------|
| `server.port` | `SERVER_PORT` | `8080` | HTTP listen port |
| `db.url` | `DB_URL` | `postgres://localhost/skyn` | PostgreSQL DSN (URL or key=value) |
| `met.user_agent` | `MET_USER_AGENT` | see example | User-Agent sent to api.met.no (required by their ToS) |
| `met.base_url` | `MET_BASE_URL` | `https://api.met.no/…` | MET API base URL |
| `log.level` | `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

> **Note**: Use `key=value` DSN format when the password contains characters special to URLs (e.g. `>`, `*`, `@`):
> ```
> host=localhost dbname=skyn user=postgres password=Ak47>Ninja* sslmode=disable
> ```

## API

Base path: `/v1`

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
      "lat": 52.374, "lon": 4.8897, "timezone": "Europe/Amsterdam" }
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

Returns the raw api.met.no GeoJSON `Feature` response (cached in PostgreSQL). Falls back to stale cache with a `Warning` header if the upstream is temporarily unavailable. Returns `503` only when cache is empty and upstream is down.

**Error responses**: `422` on missing/invalid coordinates, `503` on upstream failure with no cache.

## Migrations

Migrations are plain SQL files in `migrations/`. Apply them in order with `psql` or via the postgres Docker container:

```bash
# Via Docker (if psql is not installed locally)
docker exec -i dev_postgres psql -U postgres -d skyn < migrations/001_pg_trgm.up.sql
docker exec -i dev_postgres psql -U postgres -d skyn < migrations/002_weather_cache.up.sql
```

| Migration | Description |
|-----------|-------------|
| `001_pg_trgm` | Installs `pg_trgm` extension; creates GIN trigram index on `all_countries.name` |
| `002_weather_cache` | Creates `weather_cache` table with `UNIQUE(lat, lon)` and TTL column |

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
