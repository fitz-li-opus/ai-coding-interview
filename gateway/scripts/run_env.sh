#!/usr/bin/env bash
# Shared helpers to start the three mock services and the gateway locally.
# Sourced by dev.sh and e2e.sh. Requires Go and curl; does not use Docker.
#
# Tunable env vars (all optional):
#   GATEWAY_PORT  gateway listen port           (default 8080)
#   PORT_USER     user-service port             (default 8081)
#   PORT_ORDER    order-service port            (default 8082)
#   PORT_PRODUCT  product-service port          (default 8083)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATEWAY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
ROOT_DIR="$(cd "$GATEWAY_DIR/.." && pwd)"

PORT_USER="${PORT_USER:-8081}"
PORT_ORDER="${PORT_ORDER:-8082}"
PORT_PRODUCT="${PORT_PRODUCT:-8083}"
GATEWAY_PORT="${GATEWAY_PORT:-8080}"
GATEWAY_URL="http://localhost:${GATEWAY_PORT}"

WORK="$(mktemp -d)"
PIDS=()

env_cleanup() {
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  rm -rf "$WORK"
}

_env_wait() {
  local url="$1" tries="${2:-100}" i=0
  while [ "$i" -lt "$tries" ]; do
    if curl -fsS -o /dev/null "$url" 2>/dev/null; then
      return 0
    fi
    sleep 0.1
    i=$((i + 1))
  done
  return 1
}

# Builds a service from a copy of its main.go with the listen port patched,
# so the read-only services/ directory is never modified.
_env_build_service() {
  local name="$1" port="$2"
  mkdir -p "$WORK/$name"
  sed "s/:8080/:$port/g" "$ROOT_DIR/services/$name/main.go" > "$WORK/$name/main.go"
  (cd "$WORK/$name" && GO111MODULE=off go build -o server main.go)
}

env_start() {
  echo "==> building mock services" >&2
  _env_build_service user-service "$PORT_USER"
  _env_build_service order-service "$PORT_ORDER"
  _env_build_service product-service "$PORT_PRODUCT"

  "$WORK/user-service/server" >"$WORK/user-service.log" 2>&1 &
  PID_USER=$!
  "$WORK/order-service/server" >"$WORK/order-service.log" 2>&1 &
  PID_ORDER=$!
  "$WORK/product-service/server" >"$WORK/product-service.log" 2>&1 &
  PID_PRODUCT=$!
  PIDS+=("$PID_USER" "$PID_ORDER" "$PID_PRODUCT")

  echo "==> waiting for services" >&2
  _env_wait "http://localhost:$PORT_USER/health" || { echo "user-service not ready" >&2; cat "$WORK/user-service.log" >&2; return 1; }
  _env_wait "http://localhost:$PORT_ORDER/health" || { echo "order-service not ready" >&2; cat "$WORK/order-service.log" >&2; return 1; }
  _env_wait "http://localhost:$PORT_PRODUCT/health" || { echo "product-service not ready" >&2; cat "$WORK/product-service.log" >&2; return 1; }

  echo "==> building gateway" >&2
  (cd "$GATEWAY_DIR" && go build -o "$WORK/gateway" .)

  echo "==> starting gateway on :$GATEWAY_PORT" >&2
  GATEWAY_LISTEN=":$GATEWAY_PORT" \
  GATEWAY_UPSTREAM_USERS="http://localhost:$PORT_USER" \
  GATEWAY_UPSTREAM_ORDERS="http://localhost:$PORT_ORDER" \
  GATEWAY_UPSTREAM_PRODUCTS="http://localhost:$PORT_PRODUCT" \
    "$WORK/gateway" -config "$GATEWAY_DIR/config/routes.json" >"$WORK/gateway.log" 2>&1 &
  GATEWAY_PID=$!
  PIDS+=("$GATEWAY_PID")

  _env_wait "$GATEWAY_URL/healthz" || { echo "gateway not ready" >&2; cat "$WORK/gateway.log" >&2; return 1; }
  echo "==> environment ready at $GATEWAY_URL" >&2
}

env_stop_service() {
  local name="$1"
  case "$name" in
    user-service) kill "$PID_USER" 2>/dev/null || true ;;
    order-service) kill "$PID_ORDER" 2>/dev/null || true ;;
    product-service) kill "$PID_PRODUCT" 2>/dev/null || true ;;
  esac
  sleep 0.3
}
