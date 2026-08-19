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
#
# Known limitation: the database and the objects are not one snapshot. The dump
# is taken first and the objects a moment later, with the API still writable in
# between, so a picture replaced during that window can leave the restored
# database pointing at an object the archive does not have. Nothing in V0.1
# rewrites an object except replacing an avatar, and a nightly run at 4am is
# unlikely to catch one, so this is accepted rather than solved — solving it
# means stopping the API for the length of every backup.
set -euo pipefail

cd "$(dirname "$0")/.."

# Acts on whichever compose project this directory resolves to. With several
# stacks sharing this checkout, export COMPOSE_PROJECT_NAME deliberately —
# nothing here will notice it pointing somewhere unexpected.

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

# Read the archive end to end before counting anything in it. A truncated
# archive still lists the entries it managed to write, so counting without
# reading first turns "half a backup" into a number that looks healthy.
docker run --rm -v "$(cd "$dest" && pwd):/backup:ro" \
	alpine:3 tar tzf /backup/objects.tar.gz > /dev/null ||
	fail "the object archive is truncated or corrupt"

# An empty tar.gz is about 45 bytes, so a size check alone calls a backup of
# nothing a success. Ask the database how many objects there should be: fewer
# files than rows is the shape of a storage path that is not persisting where
# it is being backed up from, which is worth failing the whole job over.
#
# Every bucket has to be counted. Avatars live on profiles rather than in a
# table of their own, and leaving them out means an install whose only media
# is avatars — a brand new one — would pass this check with an empty archive.
stored=$(docker compose exec -T postgres psql -U "$PG_USER" -d "$PG_DB" -tAc "
	SELECT (SELECT count(*) FROM meal_photos)
	     + (SELECT count(*) FROM chat_media)
	     + (SELECT count(*) FROM user_stickers)
	     + (SELECT count(*) FROM profiles WHERE avatar_object_name IS NOT NULL);" |
	tr -d '[:space:]')
# `grep -c` exits 1 when the count is zero, which under `set -e` would kill
# the script here — on an empty archive, the one case this comparison exists
# to catch. The archive itself was already read end to end above, so nothing
# is being hidden by swallowing this status.
archived=$(docker run --rm -v "$(cd "$dest" && pwd):/backup:ro" \
	alpine:3 sh -c 'tar tzf /backup/objects.tar.gz | grep -vc "/$" || true')

# Compared, not just checked for zero: a bucket that stopped persisting while
# the others kept working leaves a non-empty archive that is still missing
# things. Extra files are expected and fine — deleted rows keep their objects,
# and fake-gcs writes a metadata file per bucket.
if [ "${archived:-0}" -lt "${stored:-0}" ]; then
	fail "the database has $stored stored objects but the archive holds only $archived files"
fi

# --- prune ----------------------------------------------------------------
# Only ever removes whole finished backups, and only from this root. The name
# is not enough on its own: "20*" also matches 2019-tax-returns, and an
# external disk is exactly where a directory like that lives. A backup is
# identified by containing a dump, which nothing else does by accident.
find "$DEST_ROOT" -mindepth 1 -maxdepth 1 -type d -mtime +"$KEEP_DAYS" 2>/dev/null |
	while IFS= read -r old; do
		[ -f "$old/database.dump" ] || continue
		rm -rf "$old"
	done

echo "backup complete: $dest"
du -sh "$dest" | awk '{print "  size: " $1}'
echo "  tables in dump: $(grep -c 'TABLE DATA' "$dest/database.toc")"
