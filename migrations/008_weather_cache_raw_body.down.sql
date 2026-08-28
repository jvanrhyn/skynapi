ALTER TABLE weather_cache
    ALTER COLUMN response_body TYPE JSONB USING response_body::JSONB;
