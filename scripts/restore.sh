#!/usr/bin/env bash
#
# Restores one backup produced by scripts/backup.sh.
#
#   scripts/restore.sh <backup-directory> [--into <database>]
#
# Without --into this overwrites the live database and the live object volume,
# which is what a real recovery wants. With --into it restores the database
# alongside the live one and checks the object archive without unpacking it,
# which is how you check a backup is good without betting the running system
# on it:
#
#   scripts/restore.sh backups/20260819T040000Z --into chiban_restore_check
#
# The API is stopped while the live data is replaced, so nothing writes into a
# half-restored state.
set -euo pipefail

cd "$(dirname "$0")/.."

# Acts on whichever compose project this directory resolves to. With several
# stacks sharing this checkout, export COMPOSE_PROJECT_NAME deliberately —
# nothing here will notice it pointing somewhere unexpected.

BACKUP=${1:?usage: scripts/restore.sh <backup-directory> [--into <database>]}
INTO=""
if [ "${2:-}" = "--into" ]; then
	INTO=${3:?--into needs a database name}
fi

PG_USER=${POSTGRES_USER:-chiban}
PG_DB=${POSTGRES_DB:-chiban}

[ -s "$BACKUP/database.dump" ] || { echo "no database.dump in $BACKUP" >&2; exit 1; }

psql() { docker compose exec -T postgres psql -U "$PG_USER" -v ON_ERROR_STOP=1 "$@"; }

# Reads the archive end to end without writing anything. This has to happen
# before the live volume is touched: a restore that clears /data and then
# discovers the archive is truncated has destroyed the only copy of the data it
# was trying to replace.
verify_objects() {
	docker run --rm -v "$(cd "$BACKUP" && pwd):/backup:ro" \
		alpine:3 tar tzf /backup/objects.tar.gz > /dev/null
}

if [ -n "$INTO" ]; then
	# A rehearsal: nothing live is touched, so the API keeps running.
	psql -d postgres -c "DROP DATABASE IF EXISTS \"$INTO\";"
	psql -d postgres -c "CREATE DATABASE \"$INTO\";"
	docker compose exec -T postgres \
		pg_restore -U "$PG_USER" -d "$INTO" --no-owner < "$BACKUP/database.dump"
	echo "restored into $INTO — the live database was not touched"
	psql -d "$INTO" -c "SELECT count(*) AS users FROM users;"

	# Half a rehearsal is worse than none: a backup whose objects are gone
	# would otherwise pass this and be trusted.
	if [ -s "$BACKUP/objects.tar.gz" ]; then
		if verify_objects; then
			echo "the object archive reads cleanly end to end"
		else
			echo "THE OBJECT ARCHIVE IS UNREADABLE — this backup cannot be restored" >&2
			exit 1
		fi
	else
		echo "THIS BACKUP HAS NO OBJECT ARCHIVE — the database alone is not a restore" >&2
		exit 1
	fi
	exit 0
fi

echo "This overwrites the live database and object storage. Ctrl-C within 5s to stop."
sleep 5

# Check the archive before anything is stopped or emptied, so a bad backup
# costs nothing but the time it took to read it.
if [ -s "$BACKUP/objects.tar.gz" ]; then
	verify_objects || {
		echo "the object archive is unreadable; nothing has been changed" >&2
		exit 1
	}
else
	echo "no objects.tar.gz in $BACKUP; nothing has been changed" >&2
	exit 1
fi

# From here on the services are down, so every exit path has to bring them
# back. Leaving a failed recovery attempt as an outage of its own is the last
# thing anyone needs while recovering.
restart_services() {
	docker compose start fake-gcs api web >/dev/null 2>&1 || true
}
trap restart_services EXIT

# Stop only the writers. Postgres itself has to stay up to be restored into.
docker compose stop api web

# --clean --if-exists inside one transaction: either the old schema is replaced
# by the backup's or nothing changes, never a mixture of the two.
docker compose exec -T postgres \
	pg_restore -U "$PG_USER" -d "$PG_DB" --clean --if-exists --no-owner --single-transaction \
	< "$BACKUP/database.dump"

volume=$(docker inspect -f \
	'{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' \
	"$(docker compose ps -q fake-gcs)")
[ -n "$volume" ] || { echo "fake-gcs has no /data volume" >&2; exit 1; }

docker compose stop fake-gcs
# Empty the volume first: leaving strays behind would restore a state that
# never existed, with objects from two different points in time. The archive
# was read end to end above, so this is only reached for one that unpacks.
docker run --rm -v "$volume:/data" -v "$(cd "$BACKUP" && pwd):/backup:ro" \
	alpine:3 sh -c 'rm -rf /data/* /data/.[!.]* /data/..?* 2>/dev/null; tar xzf /backup/objects.tar.gz -C /data'

echo "restore complete from $BACKUP"
