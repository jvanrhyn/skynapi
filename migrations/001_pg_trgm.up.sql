-- Add trigram support to the existing all_countries table for fuzzy city search.
-- Requires superuser to create the extension (typically a one-time operation per DB).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- GIN index for trigram similarity on name and asciiname fields.
CREATE INDEX IF NOT EXISTS idx_all_countries_name_trgm   ON public.all_countries USING GIN (name   gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_all_countries_ascii_trgm  ON public.all_countries USING GIN (asciiname gin_trgm_ops);



