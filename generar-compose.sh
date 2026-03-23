#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Uso: $0 <archivo_salida> <cantidad_clientes>" >&2
  exit 1
fi

OUT="$1"
N="$2"

if ! [[ "$N" =~ ^[0-9]+$ ]] || [[ "$N" -lt 0 ]]; then
  echo "Error: <cantidad_clientes> debe ser un entero >= 0. Recibido: $N" >&2
  exit 1
fi

cat > "$OUT" <<'YAML'
name: tp0
services:
  server:
    container_name: server
    image: server:latest
    entrypoint: python3 /main.py
    environment:
      - PYTHONUNBUFFERED=1
    volumes:
      - ./server/config.ini:/config.ini
    networks:
      - testing_net

YAML

if [[ "$N" -gt 0 ]]; then
  for i in $(seq 1 "$N"); do
    DNI=$((42123123 + i))
    NUMERO=$((7000 + i))

    cat >> "$OUT" <<YAML
  client${i}:
    container_name: client${i}
    image: client:latest
    entrypoint: /client
    environment:
      - CLI_ID=${i}
      - AGENCY_DATASET=/data/agency-${i}.csv
    networks:
      - testing_net
    volumes:
      - ./client/config.yaml:/config.yaml
      - ./.data/agency-${i}.csv:/data/agency-${i}.csv:ro
    depends_on:
      - server

YAML
  done
fi

cat >> "$OUT" <<'YAML'
networks:
  testing_net:
    ipam:
      driver: default
      config:
        - subnet: 172.25.125.0/24
YAML