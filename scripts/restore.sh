#!/usr/bin/env bash
#
# Restores one backup produced by scripts/backup.sh.
#
#   scripts/restore.sh <backup-directory> [--into <database>]
#
# Without --into this overwrites the live database and the live object volume,
# which is what a real recovery wants. With --into it restores the database
# alongside the live one, which is how you check a backup is good without
# betting the running system on it:
#
#   scripts/restore.sh backups/20260819T040000Z --into chiban_restore_check
#
# The API is stopped while the live data is replaced, so nothing writes into a
# half-restored state.
set -euo pipefail

cd "$(dirname "$0")/.."

BACKUP=${1:?usage: scripts/restore.sh <backup-directory> [--into <database>]}
INTO=""
if [ "${2:-}" = "--into" ]; then
	INTO=${3:?--into needs a database name}
fi

PG_USER=${POSTGRES_USER:-chiban}
PG_DB=${POSTGRES_DB:-chiban}

[ -s "$BACKUP/database.dump" ] || { echo "no database.dump in $BACKUP" >&2; exit 1; }

psql() { docker compose exec -T postgres psql -U "$PG_USER" -v ON_ERROR_STOP=1 "$@"; }

if [ -n "$INTO" ]; then
	# A rehearsal: nothing live is touched, so the API keeps running.
	psql -d postgres -c "DROP DATABASE IF EXISTS \"$INTO\";"
	psql -d postgres -c "CREATE DATABASE \"$INTO\";"
	docker compose exec -T postgres \
		pg_restore -U "$PG_USER" -d "$INTO" --no-owner < "$BACKUP/database.dump"
	echo "restored into $INTO — the live database was not touched"
	psql -d "$INTO" -c "SELECT count(*) AS users FROM users;"
	exit 0
fi

echo "This overwrites the live database and object storage. Ctrl-C within 5s to stop."
sleep 5

# Stop only the writers. Postgres itself has to stay up to be restored into.
docker compose stop api web

# --clean --if-exists inside one transaction: either the old schema is replaced
# by the backup's or nothing changes, never a mixture of the two.
docker compose exec -T postgres \
	pg_restore -U "$PG_USER" -d "$PG_DB" --clean --if-exists --no-owner --single-transaction \
	< "$BACKUP/database.dump"

if [ -s "$BACKUP/objects.tar.gz" ]; then
	volume=$(docker inspect -f \
		'{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' \
		"$(docker compose ps -q fake-gcs)")
	docker compose stop fake-gcs
	# Empty the volume first: leaving strays behind would restore a state that
	# never existed, with objects from two different points in time.
	docker run --rm -v "$volume:/data" -v "$(cd "$BACKUP" && pwd):/backup:ro" \
		alpine:3 sh -c 'rm -rf /data/* /data/..?* 2>/dev/null; tar xzf /backup/objects.tar.gz -C /data'
	docker compose start fake-gcs
fi

docker compose start api web
echo "restore complete from $BACKUP"
