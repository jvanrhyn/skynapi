# HTTP Test Files

This folder contains HTTP request files for manual and automated API testing.

## Files

| File | Purpose |
|------|---------|
| `skynapi.http` | All endpoint tests — health, cities, weather, error cases |
| `http-client.env.json` | Environment variable sets (local / staging / production) |

## Tooling

The `.http` format is compatible with several tools:

### httpYac (recommended)

```bash
# Install CLI
npm install -g httpyac

# Run all requests against local env
httpyac send skynapi.http --all --env local

# Run a single named request
httpyac send skynapi.http --name weather_amsterdam --env local
```

### VS Code Extensions

- **httpYac** — `anweber.vscode-httpyac` (recommended, uses `http-client.env.json`)
- **REST Client** — `humao.rest-client` (compatible, uses `@variable` syntax)

### JetBrains HTTP Client

Built-in to IntelliJ / GoLand — open `skynapi.http` and click the green ▶ gutter icon.

## Requests

| Name | Method | Path | Expected |
|------|--------|------|---------|
| `healthz` | GET | `/healthz` | 200 `{status: "ok"}` |
| `cities_basic` | GET | `/v1/cities?q=amsterdam` | 200 city list |
| `cities_paginated` | GET | `/v1/cities?q=cape+town&page=1&limit=5` | 200 paged |
| `cities_short` | GET | `/v1/cities?q=ny&page=1&limit=10` | 200 (trgm fallback) |
| `cities_page2` | GET | `/v1/cities?q=london&page=2&limit=10` | 200 |
| `cities_missing_q` | GET | `/v1/cities` | **422** |
| `cities_empty_q` | GET | `/v1/cities?q=` | **422** |
| `weather_amsterdam` | GET | `/v1/weather?lat=52.3676&lon=4.9041` | 200 MET forecast |
| `weather_cape_town` | GET | `/v1/weather?lat=-33.9249&lon=18.4241` | 200 MET forecast |
| `weather_zero_zero` | GET | `/v1/weather?lat=0&lon=0` | 200 (Gulf of Guinea) |
| `weather_north_pole` | GET | `/v1/weather?lat=90&lon=0` | 200 |
| `weather_south_pole` | GET | `/v1/weather?lat=-90&lon=0` | 200 |
| `weather_missing_lat` | GET | `/v1/weather?lon=4.9041` | **422** |
| `weather_missing_lon` | GET | `/v1/weather?lat=52.3676` | **422** |
| `weather_bad_lat` | GET | `/v1/weather?lat=not-a-number&lon=4.9041` | **422** |
| `weather_lat_out_of_range` | GET | `/v1/weather?lat=91&lon=4.9041` | **422** |
| `not_found` | GET | `/v1/nonexistent` | **404** |
| `method_not_allowed` | POST | `/v1/cities` | **405** |
