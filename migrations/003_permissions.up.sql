-- Grant the skynapi app user the minimum required access to the weather_cache table
-- and its SERIAL sequence. The table was created by the postgres superuser (migration 002),
-- so the app user must be granted access explicitly.
GRANT SELECT, INSERT, UPDATE ON TABLE weather_cache TO skynapi;
GRANT USAGE, SELECT ON SEQUENCE weather_cache_id_seq TO skynapi;
