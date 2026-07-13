#!/usr/bin/env bash
# Native dev runner: local Postgres + nats-server + all six Go services,
# no Docker required. PIDs land in .run/ so `dev.sh stop` and the chaos
# script can manage individual services.
set -euo pipefail
cd "$(dirname "$0")/.."

RUN_DIR=.run
LOG_DIR=.run/logs
NATS_BIN=${NATS_BIN:-$RUN_DIR/nats-server}
PG_DSN=${POSTGRES_DSN:-postgres://agenttrace:agenttrace@localhost:5432/agenttrace}

ensure_nats() {
  if [ ! -x "$NATS_BIN" ]; then
    echo "downloading nats-server..."
    mkdir -p "$RUN_DIR"
    curl -sL https://github.com/nats-io/nats-server/releases/download/v2.10.24/nats-server-v2.10.24-linux-amd64.tar.gz \
      | tar -xz -C "$RUN_DIR" --strip-components=1 --wildcards '*/nats-server'
  fi
}

ensure_db() {
  if ! psql "$PG_DSN" -c 'SELECT 1' >/dev/null 2>&1; then
    echo "creating role/database agenttrace on local Postgres (sudo)..."
    sudo -u postgres psql -c "CREATE ROLE agenttrace LOGIN PASSWORD 'agenttrace'" 2>/dev/null || true
    sudo -u postgres psql -c "CREATE DATABASE agenttrace OWNER agenttrace" 2>/dev/null || true
  fi
}

start_svc() { # name binary [env...]
  local name=$1; shift
  local bin=$1; shift
  ( export "$@" 2>/dev/null || true
    exec "$bin" ) >"$LOG_DIR/$name.log" 2>&1 &
  echo $! > "$RUN_DIR/$name.pid"
  echo "  $name (pid $!)"
}

start() {
  mkdir -p "$RUN_DIR" "$LOG_DIR" bin
  ensure_nats
  ensure_db
  echo "building..."
  go build -o bin/ ./cmd/...
  echo "starting:"
  ( exec "$NATS_BIN" -js -sd "$RUN_DIR/jetstream" ) >"$LOG_DIR/nats.log" 2>&1 &
  echo $! > "$RUN_DIR/nats.pid"; echo "  nats (pid $!)"
  sleep 0.5
  start_svc mocksvc   bin/mocksvc
  start_svc collector bin/collector NATS_URL=nats://localhost:4222
  sleep 0.5
  start_svc proxy     bin/proxy COLLECTOR_URL=ws://localhost:7100/ingest POLICY_FILE=configs/policy.yaml AT_REATTACH="${AT_REATTACH:-on}"
  start_svc processor bin/processor NATS_URL=nats://localhost:4222 POSTGRES_DSN="$PG_DSN"
  start_svc api       bin/api NATS_URL=nats://localhost:4222 POSTGRES_DSN="$PG_DSN"
  if [ "${NO_FLEET:-}" != "1" ]; then
    start_svc fleet   bin/fleet PROXY_URL=http://localhost:8080 MOCKSVC_URL=http://localhost:9100 FLEET_SPEED="${FLEET_SPEED:-1}"
  fi
  echo "api:       http://localhost:7000/healthz"
  echo "dashboard: cd dashboard && npm run dev   (or make up for the container build)"
}

stop() {
  for f in "$RUN_DIR"/*.pid; do
    [ -e "$f" ] || continue
    kill "$(cat "$f")" 2>/dev/null || true
    rm -f "$f"
  done
  echo "stopped."
}

case "${1:-start}" in
  start) start ;;
  stop)  stop ;;
  *) echo "usage: dev.sh start|stop"; exit 1 ;;
esac
