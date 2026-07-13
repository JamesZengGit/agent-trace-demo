#!/usr/bin/env bash
# make bench — measured throughput of the full capture path:
#   loadgen -> proxy -> collector -> WS acks -> NATS JetStream -> processor -> Postgres
# Sweeps offered rates, then compares what the client sent with what actually
# reached storage. Honest numbers on the current machine, not projections.
set -euo pipefail
cd "$(dirname "$0")/.."

PG_DSN=${POSTGRES_DSN:-postgres://agenttrace:agenttrace@localhost:5432/agenttrace}
RATES=${RATES:-"100 300 600 1000 1500"}
DUR=${DUR:-20s}
OUT=docs/bench
mkdir -p "$OUT"

command -v bin/loadgen >/dev/null 2>&1 || go build -o bin/ ./cmd/loadgen

stamp=$(date -u +%Y%m%dT%H%M%SZ)
csv="$OUT/results.csv"
echo "offered_rps,achieved_rps,stored_rps,drain_s,lost,p50_ms,p95_ms,p99_ms" > "$csv"

for rps in $RATES; do
  agent="bench-$stamp-$rps"
  echo "=== offered ${rps} rps for ${DUR} (agent $agent) ==="
  t0=$(date +%s)
  json=$(bin/loadgen -rps "$rps" -duration "$DUR" -agent "$agent" -mode db -workers 64)
  t1=$(date +%s)
  sent=$(echo "$json" | python3 -c "import sys,json; print(json.load(sys.stdin)['sent'])")

  # Drain: at-least-once transport means a backlog is latency, not loss.
  # Wait until the stored count stops moving, then judge.
  prev=-1
  while :; do
    stored=$(psql "$PG_DSN" -tA -c \
      "SELECT COUNT(DISTINCT span_id) FROM trace_detail WHERE agent_id='$agent'")
    [ "$stored" -ge "$sent" ] && break
    [ "$stored" -eq "$prev" ] && break   # two quiet seconds: pipeline is done
    prev=$stored
    sleep 2
  done
  t2=$(date +%s)

  dur=$(echo "$json" | python3 -c "import sys,json; print(json.load(sys.stdin)['duration_s'])")
  ach=$(echo "$json" | python3 -c "import sys,json; print(round(json.load(sys.stdin)['achieved_rps'],1))")
  p50=$(echo "$json" | python3 -c "import sys,json; print(json.load(sys.stdin)['latency_ms']['p50'])")
  p95=$(echo "$json" | python3 -c "import sys,json; print(json.load(sys.stdin)['latency_ms']['p95'])")
  p99=$(echo "$json" | python3 -c "import sys,json; print(json.load(sys.stdin)['latency_ms']['p99'])")
  drain=$((t2 - t1))
  lost=$((sent - stored))
  stored_rps=$(python3 -c "print(round($stored/($dur+$drain),1))")
  echo "sent=$sent stored=$stored lost=$lost drain=${drain}s pipeline=${stored_rps}/s p95=${p95}ms"
  echo "$rps,$ach,$stored_rps,$drain,$lost,$p50,$p95,$p99" >> "$csv"
done

python3 scripts/plot_bench.py "$csv" "$OUT/throughput.svg"
echo
echo "results: $csv"
echo "graph:   $OUT/throughput.svg"
