#!/usr/bin/env bash
# End-to-end integration test: starts the three mock services and the gateway
# locally (no Docker), asserts routing/rewrite/error handling, then tears down.
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/run_env.sh"
trap 'env_cleanup' EXIT INT TERM

env_start

fail=0

assert_contains() {
  local desc="$1" url="$2" needle="$3" body
  body="$(curl -fsS "$url" 2>/dev/null || true)"
  if printf '%s' "$body" | grep -q "$needle"; then
    echo "PASS  $desc"
  else
    echo "FAIL  $desc -> $body"
    fail=1
  fi
}

assert_status() {
  local desc="$1" method="$2" url="$3" want="$4" code
  code="$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$url")"
  if [ "$code" = "$want" ]; then
    echo "PASS  $desc ($code)"
  else
    echo "FAIL  $desc want $want got $code"
    fail=1
  fi
}

assert_status_in() {
  local desc="$1" method="$2" url="$3" want="$4" code
  code="$(curl -s -o /dev/null -w '%{http_code}' -X "$method" "$url")"
  case " $want " in
    *" $code "*) echo "PASS  $desc ($code)" ;;
    *) echo "FAIL  $desc want one of [$want] got $code"; fail=1 ;;
  esac
}

echo "==> routing"
assert_contains "users routed to user-service" "$GATEWAY_URL/api/users" '"service":"user-service"'
assert_contains "orders routed to order-service" "$GATEWAY_URL/api/orders" '"service":"order-service"'
assert_contains "products routed to product-service" "$GATEWAY_URL/api/products" '"service":"product-service"'

echo "==> path rewrite (strip_prefix /api)"
assert_contains "upstream sees /users" "$GATEWAY_URL/api/users" '"path":"/users"'
assert_contains "upstream sees /orders" "$GATEWAY_URL/api/orders" '"path":"/orders"'

echo "==> errors and admin endpoints"
assert_status "unknown route -> 404" GET "$GATEWAY_URL/nope" 404
assert_status "bad method -> 405" POST "$GATEWAY_URL/api/users" 405
assert_status "healthz -> 200" GET "$GATEWAY_URL/healthz" 200
assert_status "readyz -> 200" GET "$GATEWAY_URL/readyz" 200
assert_contains "metrics exposed" "$GATEWAY_URL/metrics" "gateway_requests_total"

echo "==> failover / isolation"
env_stop_service user-service
assert_status_in "user-service down -> 5xx" GET "$GATEWAY_URL/api/users" "502 503 504"
assert_contains "order-service unaffected" "$GATEWAY_URL/api/orders" '"service":"order-service"'

echo
if [ "$fail" -eq 0 ]; then
  echo "E2E: ALL CHECKS PASSED"
else
  echo "E2E: FAILURES DETECTED"
fi
exit "$fail"
