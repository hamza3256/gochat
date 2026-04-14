#!/usr/bin/env bash
set -euo pipefail

ADDR="${1:-ws://localhost:8080/ws}"
CONNECTIONS="${2:-100000}"
RAMP_RATE="${3:-5000}"
DURATION="${4:-60s}"

echo "=== GoChat Load Test ==="
echo "Target:      $ADDR"
echo "Connections: $CONNECTIONS"
echo "Ramp rate:   $RAMP_RATE/sec"
echo "Duration:    $DURATION"
echo "========================"

./bin/loadtest \
  -addr "$ADDR" \
  -connections "$CONNECTIONS" \
  -ramp-rate "$RAMP_RATE" \
  -msg-rate 1 \
  -duration "$DURATION"
