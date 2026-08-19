#!/usr/bin/env bash
#
# Creates the Cloudflare Tunnel this deployment runs on, and writes the config
# it needs. Run once per machine; after that `docker compose ... --profile
# tunnel up` is all it takes.
#
#   cloudflared tunnel login          # once per account, opens a browser
#   scripts/setup-tunnel.sh chiban.maxjkfc.com [tunnel-name]
#
# Everything this writes except the credentials JSON is committed, so the
# routing stays reviewable. The JSON is the key to the tunnel and is ignored
# by git.
set -euo pipefail

cd "$(dirname "$0")/.."

HOSTNAME_ARG=${1:?usage: scripts/setup-tunnel.sh <hostname> [tunnel-name]}
TUNNEL_NAME=${2:-chiban-macmini}
DEST=deploy/cloudflared

command -v cloudflared >/dev/null || {
	echo "cloudflared is not installed: brew install cloudflared" >&2
	exit 1
}
[ -f "$HOME/.cloudflared/cert.pem" ] || {
	echo "not logged in yet: run 'cloudflared tunnel login' first" >&2
	exit 1
}

mkdir -p "$DEST"

# Reuse the tunnel if it is already there, so re-running this is harmless and
# does not orphan tunnels in the account.
id=$(cloudflared tunnel list --output json |
	python3 -c "
import json,sys
name = sys.argv[1]
print(next((t['id'] for t in json.load(sys.stdin) if t['name'] == name), ''))
" "$TUNNEL_NAME")

if [ -z "$id" ]; then
	echo "creating tunnel $TUNNEL_NAME"
	cloudflared tunnel create "$TUNNEL_NAME" >/dev/null
	id=$(cloudflared tunnel list --output json |
		python3 -c "
import json,sys
print(next(t['id'] for t in json.load(sys.stdin) if t['name'] == sys.argv[1]))
" "$TUNNEL_NAME")
else
	echo "reusing tunnel $TUNNEL_NAME ($id)"
fi

# Works for a tunnel made here and for one made in the dashboard, which is the
# difference between this being a setup step and a migration. cloudflared
# refuses to overwrite, so an existing file is left as it is — re-running this
# script has to be safe or nobody will run it when something looks wrong.
if [ -f "$DEST/$id.json" ]; then
	echo "credentials already present, keeping them"
else
	cloudflared tunnel token --cred-file "$DEST/$id.json" "$TUNNEL_NAME" >/dev/null
	chmod 600 "$DEST/$id.json"
fi

cat > "$DEST/config.yml" <<EOF
# Which hostname reaches what. This is the whole routing table for public
# traffic, and it lives here rather than in the Cloudflare dashboard so that it
# is reviewed, diffed and rebuilt like the rest of the deployment.
#
# \`http://\` is deliberate: Cloudflare terminates TLS, and this hop runs inside
# the compose network. Asking for https here would make cloudflared attempt a
# TLS handshake against Caddy's plain listener and every request would 502.
#
# Written by scripts/setup-tunnel.sh. The credentials JSON named below is the
# key to the tunnel and is never committed.
tunnel: $id
credentials-file: /etc/cloudflared/$id.json

ingress:
  - hostname: $HOSTNAME_ARG
    service: http://caddy:80
  # Anything addressed to this tunnel that is not the hostname above is not
  # part of this product and gets nothing.
  - service: http_status:404
EOF

# Points the name at this tunnel. Already-correct records are left alone;
# a record pointing somewhere else has to be repointed by hand on purpose.
cloudflared tunnel route dns "$TUNNEL_NAME" "$HOSTNAME_ARG" 2>&1 |
	sed 's/^/  /' || echo "  (record already exists — check it points at $TUNNEL_NAME)"

echo
echo "done. $HOSTNAME_ARG -> $TUNNEL_NAME -> caddy:80"
echo "start it with:"
echo "  docker compose -f docker-compose.yml -f docker-compose.deploy.yml --profile tunnel up -d --build"
