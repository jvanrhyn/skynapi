REVOKE USAGE, SELECT ON SEQUENCE reverse_geocode_cache_id_seq FROM skynapi;
REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE reverse_geocode_cache FROM skynapi;
REVOKE DELETE ON TABLE weather_cache FROM skynapi;
