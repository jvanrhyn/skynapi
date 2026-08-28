-- Reverse-geocode cache. Keyed by coordinates rounded to 2 decimal places
-- (~1.1 km), which is precise enough to name a settlement, coarse enough that
-- a user's exact GPS fix never leaves this system, and gives nearby lookups a
-- shared cache entry. One row per location; upsert on refresh.
CREATE TABLE IF NOT EXISTS reverse_geocode_cache (
    id           SERIAL PRIMARY KEY,
    lat          NUMERIC(8, 2) NOT NULL,
    lon          NUMERIC(8, 2) NOT NULL,
    cached_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    label        TEXT          NOT NULL,
    city         TEXT          NOT NULL DEFAULT '',
    country      TEXT          NOT NULL DEFAULT '',
    country_code TEXT          NOT NULL DEFAULT '',
    CONSTRAINT uq_reverse_geocode_coords UNIQUE (lat, lon)
);

CREATE INDEX IF NOT EXISTS idx_reverse_geocode_cached_at
    ON reverse_geocode_cache (cached_at);
