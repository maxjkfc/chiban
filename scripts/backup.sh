#!/usr/bin/env bash
#
# Backs up everything that cannot be rebuilt from the repository: the database
# and the stored objects. Both are taken from the running containers, so this
# needs no local postgres client and no knowledge of where Docker keeps its
# volumes.
#
#   scripts/backup.sh [destination]
#
# destination defaults to $CHIBAN_BACKUP_DIR, then to ./backups. Point it at
# the external disk on the Mac mini:
#
#   0 4 * * *  cd /Users/you/code/chiban && scripts/backup.sh /Volumes/Backup/chiban >> /tmp/chiban-backup.log 2>&1
#
# Restore with scripts/restore.sh.
set -euo pipefail

cd "$(dirname "$0")/.."

DEST_ROOT=${1:-${CHIBAN_BACKUP_DIR:-./backups}}
# How many days of backups to keep. A daily job with the default keeps a
# fortnight, which is longer than it takes to notice something went wrong.
KEEP_DAYS=${CHIBAN_BACKUP_KEEP_DAYS:-14}

PG_USER=${POSTGRES_USER:-chiban}
PG_DB=${POSTGRES_DB:-chiban}

stamp=$(date -u +%Y%m%dT%H%M%SZ)
dest="$DEST_ROOT/$stamp"
mkdir -p "$dest"

fail() {
	echo "backup failed: $*" >&2
	# A half-written backup is worse than none: it looks like a backup.
	rm -rf "$dest"
	exit 1
}

# --- database -------------------------------------------------------------
# Custom format, not plain SQL: pg_restore can then rebuild selectively and
# refuses to run a truncated archive instead of replaying half of one.
docker compose exec -T postgres \
	pg_dump -U "$PG_USER" -d "$PG_DB" -Fc > "$dest/database.dump" ||
	fail "pg_dump"

[ -s "$dest/database.dump" ] || fail "the dump is empty"

# Reading the archive's table of contents is the cheapest thing that can tell
# a real backup from a file of the right name. It catches truncation, which is
# what a disk filling up in the middle of the night produces.
#
# The dump goes back into the container as a file rather than down a pipe: a
# custom-format archive has to be seekable, and pg_restore reading a pipe fails
# on the header no matter how good the backup is.
docker compose exec -T postgres sh -c '
	cat > /tmp/verify.dump
	pg_restore --list /tmp/verify.dump
	status=$?
	rm -f /tmp/verify.dump
	exit $status
' < "$dest/database.dump" > "$dest/database.toc" ||
	fail "the dump is not a readable archive"

# --- objects --------------------------------------------------------------
# Ask the running container where its volume actually lives rather than
# guessing the compose project prefix, which changes with the directory name.
volume=$(docker inspect -f \
	'{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' \
	"$(docker compose ps -q fake-gcs)") || fail "could not find the fake-gcs container"
[ -n "$volume" ] || fail "fake-gcs has no /data volume"

docker run --rm \
	-v "$volume:/data:ro" \
	-v "$(cd "$dest" && pwd):/backup" \
	alpine:3 tar czf /backup/objects.tar.gz -C /data . || fail "tar of the object volume"

# An empty tar.gz is about 45 bytes, so a size check alone calls a backup of
# nothing a success. Ask the database whether there should have been anything:
# rows without objects is the shape of a storage path that is not persisting
# where it is being backed up from, which is worth failing the whole job over.
stored=$(docker compose exec -T postgres psql -U "$PG_USER" -d "$PG_DB" -tAc "
	SELECT (SELECT count(*) FROM meal_photos)
	     + (SELECT count(*) FROM chat_media)
	     + (SELECT count(*) FROM user_stickers);" | tr -d '[:space:]')
archived=$(docker run --rm -v "$(cd "$dest" && pwd):/backup:ro" \
	alpine:3 sh -c 'tar tzf /backup/objects.tar.gz | grep -vc "/$"' || true)

if [ "${stored:-0}" -gt 0 ] && [ "${archived:-0}" -eq 0 ]; then
	fail "the database has $stored stored objects but the archive has none — \
the object volume is not where the objects are being written"
fi

# --- prune ----------------------------------------------------------------
# Only ever removes whole finished backups, and only from this root.
find "$DEST_ROOT" -mindepth 1 -maxdepth 1 -type d -name '20*' \
	-mtime +"$KEEP_DAYS" -exec rm -rf {} + 2>/dev/null || true

echo "backup complete: $dest"
du -sh "$dest" | awk '{print "  size: " $1}'
awk 'END {print "  tables in dump: " NR}' "$dest/database.toc"
