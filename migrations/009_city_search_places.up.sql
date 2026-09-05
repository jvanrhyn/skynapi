-- Current populated places include suburbs with unknown/zero population.
-- Match the repository predicate exactly so these partial indexes are usable.
CREATE INDEX IF NOT EXISTS idx_all_countries_name_trgm_places
 ON public.all_countries USING gin (name public.gin_trgm_ops)
 WHERE feature_class = 'P' AND feature_code IN
 ('PPL','PPLA','PPLA2','PPLA3','PPLA4','PPLA5','PPLC','PPLF','PPLG','PPLL','PPLR','PPLS','PPLX','STLMT');
CREATE INDEX IF NOT EXISTS idx_all_countries_ascii_trgm_places
 ON public.all_countries USING gin (asciiname public.gin_trgm_ops)
 WHERE feature_class = 'P' AND feature_code IN
 ('PPL','PPLA','PPLA2','PPLA3','PPLA4','PPLA5','PPLC','PPLF','PPLG','PPLL','PPLR','PPLS','PPLX','STLMT');
CREATE INDEX IF NOT EXISTS idx_all_countries_alias_trgm_places
 ON public.all_countries USING gin ((',' || COALESCE(alternatenames, '') || ',') public.gin_trgm_ops)
 WHERE feature_class = 'P' AND feature_code IN
 ('PPL','PPLA','PPLA2','PPLA3','PPLA4','PPLA5','PPLC','PPLF','PPLG','PPLL','PPLR','PPLS','PPLX','STLMT');
