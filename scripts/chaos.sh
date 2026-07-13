#!/usr/bin/env bash
# make chaos — reproduces the production orphan incident (~87% of trace
# records stranded when pod restarts reassigned collector ports) and the fix.
#
# Two runs under identical steady load, killing the collector mid-run:
#   run 1: AT_REATTACH=off  -> the proxy's unacked buffer dies with the
#                              connection (the original bug)
#   run 2: AT_REATTACH=on   -> sequence-numbered acks + resend buffer; the
#                              proxy reattaches and replays (the fix)
# Requires the native dev stack (scripts/dev.sh start). Writes a per-second
# recovery timeline and the orphan percentages to docs/chaos/.
set -euo pipefail
cd "$(dirname "$0")/.."

PG_DSN=${POSTGRES_DSN:-postgres://agenttrace:agenttrace@localhost:5432/agenttrace}
RPS=${RPS:-300}
DUR=${DUR:-60}        # seconds of load
KILL_AT=${KILL_AT:-10}    # a long outage, like production: the collector was
RESTART_AT=${RESTART_AT:-52} # effectively unreachable for most of the window
OUT=docs/chaos
mkdir -p "$OUT" .run/logs bin

[ -f .run/collector.pid ] || { echo "native stack not running — run: ./scripts/dev.sh start"; exit 1; }
go build -o bin/ ./cmd/...

restart_proxy() { # $1 = on|off
  if [ -f .run/proxy.pid ]; then kill "$(cat .run/proxy.pid)" 2>/dev/null || true; fi
  sleep 0.5
  ( export COLLECTOR_URL=ws://localhost:7100/ingest POLICY_FILE=configs/policy.yaml AT_REATTACH=$1
    exec bin/proxy ) >.run/logs/proxy.log 2>&1 &
  echo $! > .run/proxy.pid
  sleep 1
}

kill_collector() {
  kill "$(cat .run/collector.pid)" 2>/dev/null || true
  echo "    t=${KILL_AT}s collector KILLED"
}

restart_collector() {
  ( export NATS_URL=nats://localhost:4222
    exec bin/collector ) >.run/logs/collector.log 2>&1 &
  echo $! > .run/collector.pid
  echo "    t=${RESTART_AT}s collector restarted"
}

run_phase() { # $1 = off|on
  local mode=$1
  local agent="chaos-$mode-$(date -u +%H%M%S)"
  local csv="$OUT/timeline-$mode.csv"
  echo "=== phase AT_REATTACH=$mode: ${RPS} rps for ${DUR}s, kill at ${KILL_AT}s ==="
  restart_proxy "$mode"

  bin/loadgen -rps "$RPS" -duration "${DUR}s" -agent "$agent" -mode db -workers 64 \
    > "$OUT/loadgen-$mode.json" &
  local lg=$!

  # Sample until the load is done AND the pipeline has drained its backlog —
  # a replay burst is latency, not loss, and the graph must show the
  # convergence. Plateau = 4 consecutive identical samples.
  echo "second,stored" > "$csv"
  plateau=0 prev_stored=-1
  for ((t=1; t<=DUR+180; t++)); do
    sleep 1
    if [ "$t" -eq "$KILL_AT" ]; then kill_collector; fi
    if [ "$t" -eq "$RESTART_AT" ]; then restart_collector; fi
    stored=$(psql "$PG_DSN" -tA -c \
      "SELECT COUNT(DISTINCT span_id) FROM trace_detail WHERE agent_id='$agent'")
    echo "$t,$stored" >> "$csv"
    if [ "$t" -gt "$DUR" ]; then
      if [ "$stored" -eq "$prev_stored" ]; then
        plateau=$((plateau+1))
        [ "$plateau" -ge 4 ] && break
      else
        plateau=0
      fi
    fi
    prev_stored=$stored
  done
  wait "$lg" || true

  local sent stored pct
  sent=$(python3 -c "import json; print(json.load(open('$OUT/loadgen-$mode.json'))['sent'])")
  stored=$(psql "$PG_DSN" -tA -c \
    "SELECT COUNT(DISTINCT span_id) FROM trace_detail WHERE agent_id='$agent'")
  pct=$(python3 -c "print(round(100*(1-$stored/max($sent,1)),1))")
  echo "    sent=$sent stored=$stored orphaned=${pct}%"
  echo "$mode,$sent,$stored,$pct" >> "$OUT/summary.csv"
}

echo "mode,sent,stored,orphaned_pct" > "$OUT/summary.csv"
run_phase off
sleep 3
run_phase on
restart_proxy on   # leave the stack healthy

python3 scripts/plot_chaos.py "$OUT" "$OUT/recovery.svg" "$KILL_AT" "$RESTART_AT"
echo
column -s, -t "$OUT/summary.csv"
echo "graph: $OUT/recovery.svg"
