-- Weather forecast cache. Keyed by coordinates normalised to 4 decimal places
-- to match api.met.no precision. One row per unique location; upsert on refresh.
CREATE TABLE IF NOT EXISTS weather_cache (
    id            SERIAL PRIMARY KEY,
    lat           NUMERIC(8, 4)  NOT NULL,
    lon           NUMERIC(8, 4)  NOT NULL,
    cached_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ,
    last_modified TIMESTAMPTZ,
    response_body JSONB          NOT NULL,
    CONSTRAINT uq_weather_coords UNIQUE (lat, lon)
);

CREATE INDEX IF NOT EXISTS idx_weather_cache_expires ON weather_cache (expires_at);
