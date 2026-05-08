#!/bin/sh
set -eu

for migration in /migrations/*.up.sql; do
  echo "Applying ${migration}"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -f "$migration"
done

