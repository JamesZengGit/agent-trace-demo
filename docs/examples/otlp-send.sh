#!/usr/bin/env bash
# Sends the OTLP sample to the AgentTrace ingest adapter with fresh
# timestamps (the trace appears in the dashboard's live window).
set -euo pipefail
cd "$(dirname "$0")"
API=${API:-http://localhost:7000}
NOW=$(date +%s)
sed -e "s/TS_START/${NOW}000000000/" -e "s/TS_END/${NOW}800000000/" otlp-sample.json \
  | curl -s -X POST "$API/otel/v1/traces" -H 'Content-Type: application/json' -d @-
echo
