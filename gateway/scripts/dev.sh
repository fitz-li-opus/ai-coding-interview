#!/usr/bin/env bash
# Starts mock services + gateway locally (no Docker) and prints manual test
# commands. Press Ctrl-C to stop everything.
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/run_env.sh"
trap 'echo; echo "==> shutting down"; env_cleanup' EXIT INT TERM

env_start

cat <<EOF

Gateway is running: $GATEWAY_URL

Try it:
  curl -s $GATEWAY_URL/api/users
  curl -s $GATEWAY_URL/api/orders
  curl -s $GATEWAY_URL/api/products
  curl -s -i $GATEWAY_URL/healthz
  curl -s -i $GATEWAY_URL/readyz
  curl -s $GATEWAY_URL/metrics
  curl -s -i -X POST $GATEWAY_URL/api/users     # 405
  curl -s -i $GATEWAY_URL/nope                  # 404

Press Ctrl-C to stop.
EOF

wait "$GATEWAY_PID"
