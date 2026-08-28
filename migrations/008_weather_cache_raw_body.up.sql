-- The forecast body is stored and served as an opaque blob — nothing queries
-- inside it. JSONB was therefore costing a parse on every write and a re-render
-- on every read, and its normalisation (key reordering, whitespace and number
-- canonicalisation) meant the bytes served from cache differed from the bytes
-- received upstream. That made the response ETag change once per refresh for no
-- real content change, and defeated serving the upstream payload verbatim.
--
-- TEXT preserves the bytes exactly. The client already verifies the payload is
-- well-formed JSON before it is stored, so the JSONB type check is not lost.
ALTER TABLE weather_cache
    ALTER COLUMN response_body TYPE TEXT USING response_body::TEXT;
