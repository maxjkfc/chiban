#!/usr/bin/env bash
#
# Builds and starts the stack on the Mac mini, then proves the deployment is
# the one this checkout describes.
#
#   scripts/deploy.sh [--pull]
#
# `up -d --build` says nothing about whether it worked. Two failures in
# particular are silent: a web image that was not rebuilt keeps serving the
# previous UI at a URL that answers 200, and a migration that did not apply
# leaves an API that starts fine and then 500s on the one endpoint that needs
# the new column. Both are found here rather than by a person on a phone.
#
# Every expectation is derived from the checkout, never hard-coded: the
# migration version comes from apps/api/migrations, the CSS markers from the
# utilities declared in globals.css. A check that needs editing whenever the
# code changes is a check that gets left behind and starts lying.
#
# Pulling is opt-in. Deploying whatever a `git pull` happened to fetch is how
# you ship a commit nobody read; without --pull this deploys exactly what is
# checked out, and says which commit that is.
set -euo pipefail

cd "$(dirname "$0")/.."

pull=0
for arg in "$@"; do
	case "$arg" in
	--pull) pull=1 ;;
	*)
		echo "usage: scripts/deploy.sh [--pull]" >&2
		exit 2
		;;
	esac
done

# .env supplies the credentials compose interpolates. Read the few values this
# script needs the same way compose does, defaults included, so a deployment
# that renamed the database is still checked against the right one.
# shellcheck disable=SC1091
[ -f .env ] && set -a && . ./.env && set +a
PG_USER=${POSTGRES_USER:-chiban}
APP_DB=${POSTGRES_DB:-chiban}
EDGE=${EDGE_PORT:-8090}

compose=(docker compose -f docker-compose.yml -f docker-compose.deploy.yml --profile tunnel)

if [ "$pull" -eq 1 ]; then
	# Only on a clean tree: merging into a dirty checkout mid-deploy leaves the
	# running stack and the files it was built from disagreeing.
	if [ -n "$(git status --porcelain)" ]; then
		echo "working tree is dirty; commit or stash before --pull" >&2
		exit 1
	fi
	git pull --ff-only origin main
fi

echo "deploying $(git rev-parse --short HEAD) — $(git log -1 --pretty=%s)"
if [ -n "$(git status --porcelain)" ]; then
	echo "  (working tree has uncommitted changes; they are being deployed too)"
fi
echo

# --build is not optional. docker-compose.deploy.yml sets the frontend's API
# base to an empty string through a build argument, so a reused image carries
# whatever base it was built with — most likely somebody's localhost.
echo "== build and start =="
"${compose[@]}" up -d --build

echo
echo "== waiting for the API =="
# The API applies migrations on startup, so "healthy" is also "migrated". A
# fixed sleep would either be too short on a cold build or waste time on a warm
# one, so this waits for the thing it actually depends on.
deadline=$((SECONDS + 120))
until curl -fsS -o /dev/null "http://127.0.0.1:${EDGE}/api/v1/auth/me" \
	-w '%{http_code}' 2>/dev/null | grep -q . || [ "$SECONDS" -ge "$deadline" ]; do
	sleep 2
done
# 401 is the healthy answer for an unauthenticated caller, so the check is that
# the endpoint responds at all, not that it succeeds.
api_code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${EDGE}/api/v1/auth/me" || echo 000)
if [ "$api_code" = "000" ]; then
	echo "  the API never answered within 120s" >&2
	echo
	"${compose[@]}" logs --tail 40 api >&2
	exit 1
fi
echo "  ok        API answers (${api_code})"

status=0

echo
echo "== migrations =="
# The newest file in the repo has to be the newest row in goose's table. Asking
# for a specific column name instead would need editing on every migration and
# would quietly stop checking the newest one.
want=$(find apps/api/migrations -name '[0-9]*.sql' -exec basename {} \; |
	sed -E 's/^0*([0-9]+)_.*/\1/' | sort -n | tail -1)
got=$("${compose[@]}" exec -T postgres \
	psql -U "$PG_USER" -d "$APP_DB" -tAc \
	'SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied' 2>/dev/null |
	tr -d '[:space:]')

if [ -z "$got" ]; then
	echo "  FAILED    could not read goose_db_version from $APP_DB" >&2
	status=1
elif [ "$got" -lt "$want" ]; then
	echo "  FAILED    database is at $got, this checkout needs $want" >&2
	status=1
else
	echo "  ok        migrations applied through $got"
fi

echo
echo "== the site is serving this checkout's CSS =="
# A stale web image is the failure this catches: it answers 200 with the
# previous UI, which no status code can distinguish from a good deploy.
#
# The markers are the utilities globals.css declares, because they survive the
# build as literal class names. Colour values do not: the production build
# downlevels oklch() to a hex and a lab() fallback, so grepping the served CSS
# for the source token finds nothing however correct the deployment is.
markers=$(grep -oE '^@utility [a-z-]+' apps/web/app/globals.css | awk '{print $2}')
if [ -z "$markers" ]; then
	echo "  SKIPPED   globals.css declares no @utility to look for" >&2
	status=1
else
	href=$(curl -fsS "http://127.0.0.1:${EDGE}/login" |
		grep -oE 'href="/_next/[^"]+\.css"' | sed -E 's/href="([^"]*)"/\1/' | head -1)
	if [ -z "$href" ]; then
		echo "  FAILED    /login references no stylesheet" >&2
		status=1
	else
		css=$(curl -fsS "http://127.0.0.1:${EDGE}${href}")
		for marker in $markers; do
			# The trailing class means `.tape` cannot be matched by `.tapered`.
			if printf '%s' "$css" | grep -qE "\.${marker}[,{ ]"; then
				echo "  ok        .$marker"
			else
				echo "  MISSING   .$marker — the web image is older than this checkout" >&2
				status=1
			fi
		done
	fi
fi

echo
echo "== path routing =="
# One origin, split by path: the site on everything else, the API on /api/*.
# Getting the site's HTML back from an /api/ URL means Caddy is sending API
# traffic to the frontend, which fails much later and much more confusingly.
site_code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${EDGE}/login" || echo 000)
if [ "$site_code" = "200" ]; then
	echo "  ok        /login -> $site_code"
else
	echo "  FAILED    /login -> $site_code, want 200" >&2
	status=1
fi
if [ "$api_code" = "401" ]; then
	echo "  ok        /api/v1/auth/me -> $api_code"
else
	echo "  FAILED    /api/v1/auth/me -> $api_code, want 401" >&2
	status=1
fi

echo
echo "== nothing unauthenticated is reachable =="
scripts/check-exposure.sh || status=1

echo
if [ "$status" -eq 0 ]; then
	echo "deployed $(git rev-parse --short HEAD) and verified"
	echo
	echo "still worth doing by hand, on a phone, through the real domain:"
	echo "  record a meal, share it, and reply from a second account —"
	echo "  live chat updates are what prove the WebSocket crossed the tunnel."
else
	echo "FAILED — the stack is up but something is not what this checkout says" >&2
	echo "  docker compose -f docker-compose.yml -f docker-compose.deploy.yml logs api" >&2
fi
exit "$status"
