-- Revoke permissions before dropping to avoid dependency errors.
REVOKE SELECT ON TABLE public.country_codes FROM skynapi;
DROP TABLE IF EXISTS public.country_codes;
