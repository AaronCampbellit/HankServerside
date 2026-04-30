#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
SERVICE="${POSTGRES_SERVICE:-postgres}"

echo "Stopping Hank cloud and PostgreSQL before enabling checksums..."
docker compose -f "$COMPOSE_FILE" stop cloud db-ops "$SERVICE"

echo "Enabling PostgreSQL data checksums. This requires the database to stay stopped."
docker compose -f "$COMPOSE_FILE" run --rm --no-deps --entrypoint sh "$SERVICE" -c 'su-exec postgres pg_checksums --enable -D /var/lib/postgresql/data'

echo "Starting PostgreSQL, db-ops, and cloud..."
docker compose -f "$COMPOSE_FILE" up -d "$SERVICE" db-ops cloud

echo "Checksum enablement requested. Check /dashboard/storage after PostgreSQL is healthy."
