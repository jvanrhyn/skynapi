-- City search only ever considers rows with a population, but the trigram
-- indexes from 001 cover the whole table, so that filter is applied as a
-- recheck after the index scan instead of narrowing it. Partial indexes fold
-- the predicate into the scan.
--
-- On a fully loaded Geonames table this rewrites two large indexes. Run it
-- during a quiet window, or add CONCURRENTLY by hand outside a transaction.
CREATE INDEX IF NOT EXISTS idx_all_countries_name_trgm_pop
    ON public.all_countries USING gin (name public.gin_trgm_ops)
    WHERE population > 0;

CREATE INDEX IF NOT EXISTS idx_all_countries_ascii_trgm_pop
    ON public.all_countries USING gin (asciiname public.gin_trgm_ops)
    WHERE population > 0;

-- Ranking now breaks similarity ties by population.
CREATE INDEX IF NOT EXISTS idx_all_countries_population
    ON public.all_countries (population DESC)
    WHERE population > 0;
