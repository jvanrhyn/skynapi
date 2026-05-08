-- Add trigram support to the existing all_countries table for fuzzy city search.
-- Requires superuser to create the extension (typically a one-time operation per DB).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

create index if not exists idx_city_name
    on public.all_countries (name);

create index  if not exists idx_all_countries_name_trgm
    on public.all_countries using gin (name public.gin_trgm_ops);

create index if not exists  idx_all_countries_ascii_trgm
    on public.all_countries using gin (asciiname public.gin_trgm_ops);

grant delete, insert, select, update on public.all_countries to skynapi;