#!/bin/sh
# Waits for Postgres, applies pending event-store migrations, then execs the
# given command (default: go run ./cmd/server) — mirrors bin/db_migrate.sh.
set -eu

host="${DB_HOST:-postgres}"
port="${DB_PORT:-5432}"
user="${DB_USER:-money_flow}"

until pg_isready -h "$host" -p "$port" -U "$user" >/dev/null 2>&1; do
  echo "entrypoint: waiting for postgres at $host:$port..."
  sleep 1
done

go run ./cmd/migrate up

exec "$@"
