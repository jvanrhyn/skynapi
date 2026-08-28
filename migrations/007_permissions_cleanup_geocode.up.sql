-- 003 granted the app user SELECT, INSERT and UPDATE on weather_cache, but the
-- periodic cache eviction issues DELETE. Without this grant the cleanup job
-- fails on every tick and only shows up as a logged error.
GRANT DELETE ON TABLE weather_cache TO skynapi;

-- Same minimum access for the reverse-geocode cache created in 006, which is
-- likewise created by the superuser running migrations.
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE reverse_geocode_cache TO skynapi;
GRANT USAGE, SELECT ON SEQUENCE reverse_geocode_cache_id_seq TO skynapi;
