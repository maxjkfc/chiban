#!/usr/bin/env bash
#
# Checks that nothing which has no authentication of its own is reachable from
# outside this machine. Public traffic is supposed to arrive through Cloudflare
# Tunnel, which reaches the services over the compose network — no container
# needs a port open to the LAN for that to work.
#
#   scripts/check-exposure.sh
#
# Two checks, because they fail differently: the first reads intent from
# docker-compose.yml and catches a published port losing its 127.0.0.1 prefix
# in review; the second asks the network what is actually listening, which is
# the only thing that can catch a container started some other way.
set -euo pipefail

cd "$(dirname "$0")/.."

# Assumes one deployment per checkout. Running several stacks from this
# directory with different COMPOSE_PROJECT_NAME values would have this probe
# the containers of whichever one the environment names, without saying so.
status=0

echo "== published ports in docker-compose.yml =="
# The parser below reads the short "host:port:port" form. The long form spreads
# the address over `target:`/`published:` keys, which it would walk straight
# past — under-checking while still printing a pass, which is the failure this
# whole script exists to prevent. Refuse to guess.
if grep -qE '^[[:space:]]*-[[:space:]]*target:' docker-compose.yml; then
	echo "  long-form port syntax found; this script only reads the short form" >&2
	echo "  refusing to report a result it did not actually check" >&2
	exit 1
fi

# A published port is safe when it is bound to loopback. Anything else is
# either "0.0.0.0:5432:5432" or the bare "5432:5432" shorthand, both of which
# listen on every interface.
while IFS= read -r line; do
	port=$(echo "$line" | sed -E 's/^[[:space:]]*-[[:space:]]*"?([^"]*)"?[[:space:]]*$/\1/')
	case "$port" in
	127.0.0.1:* | localhost:*)
		echo "  ok        $port"
		;;
	*)
		echo "  EXPOSED   $port"
		status=1
		;;
	esac
# Comments and blank lines sit between `ports:` and its entries, so the block
# ends at the next key at the same or lower indentation, not at the first line
# that is not a list item. Ending it early silently checks fewer ports than
# there are and still reports a pass.
done < <(awk '/^[[:space:]]*ports:[[:space:]]*$/{inports=1; next}
	!inports{next}
	/^[[:space:]]*#/{next}
	/^[[:space:]]*$/{next}
	/^[[:space:]]*-/{print; next}
	{inports=0}' docker-compose.yml)

echo
echo "== what is actually listening =="
# Every active interface, not a guess at two names: a Mac mini on a USB or
# Thunderbolt adapter is not on en0, and probing the wrong interface would
# report a pass without having asked the network anything.
lan_ip=""
for iface in $(ifconfig -l 2>/dev/null); do
	case "$iface" in
	lo* | gif* | stf* | utun* | awdl* | llw*)
		continue
		;;
	esac
	addr=$(ipconfig getifaddr "$iface" 2>/dev/null || true)
	if [ -n "$addr" ]; then
		lan_ip=$addr
		break
	fi
done

if [ -z "$lan_ip" ]; then
	# An unrun check is not a passed one, so this fails the script rather than
	# printing a pass for a probe that never happened.
	echo "  SKIPPED — no address on any active interface, so nothing was probed"
	echo "  (the compose check above still ran)"
	status=1
else
	echo "  probing $lan_ip"
	for entry in "postgres:${POSTGRES_PORT:-15432}" "fake-gcs:${FAKE_GCS_PORT:-14443}"; do
		name=${entry%%:*}
		port=${entry##*:}
		if nc -z -G 2 "$lan_ip" "$port" 2>/dev/null; then
			echo "  EXPOSED   $name answers on $lan_ip:$port"
			status=1
		else
			echo "  ok        $name does not answer on $lan_ip:$port"
		fi
	done
fi

echo
if [ "$status" -eq 0 ]; then
	echo "nothing unauthenticated is reachable from outside this machine"
else
	echo "FAILED — something is reachable that should not be, or a check could not run" >&2
fi
exit "$status"
