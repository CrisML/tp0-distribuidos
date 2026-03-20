#!/bin/sh
set -eu

ACTION="action: test_echo_server"
SUCCESS="$ACTION | result: success"
FAIL="$ACTION | result: fail"

SERVER_CONTAINER="${SERVER_CONTAINER:-server}"
SERVER_HOST="${SERVER_HOST:-server}"
SERVER_PORT="${SERVER_PORT:-12345}"

if ! docker ps --format '{{.Names}}' | grep -qx "$SERVER_CONTAINER"; then
  echo "$FAIL"
  exit 1
fi

NETWORK="$(docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{println $k}}{{end}}' "$SERVER_CONTAINER" 2>/dev/null | head -n 1 || true)"
if [ -z "$NETWORK" ]; then
  echo "$FAIL"
  exit 1
fi

MSG="tp0-echo-test-$(date +%s)"

REPLY="$(
  docker run --rm --network "$NETWORK" \
    -e MSG="$MSG" \
    alpine:3.19 sh -lc '
      set -e
      apk add --no-cache netcat-openbsd >/dev/null
      printf "%s\n" "$MSG" | nc -w 2 '"$SERVER_HOST"' '"$SERVER_PORT"' | head -n 1
    ' 2>/dev/null || true
)"

if [ "$REPLY" = "$MSG" ]; then
  echo "$SUCCESS"
  exit 0
fi

echo "$FAIL"
exit 1