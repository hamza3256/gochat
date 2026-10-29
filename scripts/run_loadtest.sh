#!/usr/bin/env bash
set -euo pipefail

ADDR="${1:-127.0.0.1:8080}"
CONNECTIONS="${2:-1000}"
DURATION="${3:-5s}"
MODE="${4:-echo}"

echo "=== GoChat Load Test ==="
echo "Target:      $ADDR"
echo "Connections: $CONNECTIONS"
echo "Duration:    $DURATION"
echo "Mode:        $MODE"
echo "========================"

./bin/loadtest \
  -addr "$ADDR" \
  -connections "$CONNECTIONS" \
  -duration "$DURATION" \
  -mode "$MODE"
