#!/usr/bin/env bash
# End-to-end check: build the gateway, run it on :8080, curl through it against the real services.
# Prerequisites: the services are up (`docker compose up --build -d` in the repo root),
# :8080 is free, and go, curl and jq are installed. Exits 1 if any case fails.
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
GW=http://localhost:8080
TMP="$(mktemp -d)"
BIN="$TMP/gateway"
LOG="$TMP/gateway.log"
fails=0
pid=

expect() { # name, expected substring, actual
  if [[ "$3" == *"$2"* ]]; then echo "PASS  $1"; else echo "FAIL  $1 | expected: $2 | got: $3"; fails=$((fails + 1)); fi
}
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
field() { curl -s "${@:2}" | jq -r ".$1"; }  # field <json key> <curl args...>

start_gateway() { # start_gateway [ENV=value ...]
  env "$@" "$BIN" >>"$LOG" 2>&1 &
  pid=$!
  for _ in $(seq 50); do curl -s -o /dev/null "$GW/health" && return 0; sleep 0.1; done
  echo "gateway did not start:"; cat "$LOG"; exit 1
}
stop_gateway() { [ -n "$pid" ] && kill "$pid" 2>/dev/null; wait "$pid" 2>/dev/null; pid=; }
trap stop_gateway EXIT

for p in 8081 8082 8083; do
  curl -sf -m 2 "localhost:$p/health" >/dev/null || { echo "service on :$p is down; run: docker compose up --build -d"; exit 1; }
done
curl -s -o /dev/null -m 1 "$GW" && { echo "something already listens on :8080"; exit 1; }
(cd "$HERE" && go build -o "$BIN" .) || { echo "FAIL  build gateway"; exit 1; }
start_gateway

echo "== routing"
expect "/api/users -> user-service"            user-service    "$(field service "$GW/api/users")"
expect "/api/users -> prefix stripped to /"    '/'             "$(field path "$GW/api/users")"
expect "/api/orders/1001 -> order-service"     order-service   "$(field service "$GW/api/orders/1001?status=shipped")"
expect "/api/orders/1001 -> path /1001"        /1001           "$(field path "$GW/api/orders/1001?status=shipped")"
expect "/api/products/ -> product-service"     product-service "$(field service "$GW/api/products/")"
expect "POST /api/products keeps method"       POST            "$(field method -X POST -d '{"name":"Mouse"}' "$GW/api/products")"
expect "/api/users/health -> service health"   healthy         "$(field status "$GW/api/users/health")"

echo "== no route"
expect "/api/usersX -> 404 (prefix boundary)"  404   "$(code "$GW/api/usersX")"
expect "/api/user -> 404"                      404   "$(code "$GW/api/user")"
expect "/ -> 404"                              404   "$(code "$GW/")"
expect "404 body is JSON error"                'no route' "$(field error "$GW/nope")"
expect "/api/users/../orders -> 301 /api/orders" "301 $GW/api/orders" \
  "$(curl -s --path-as-is -o /dev/null -w '%{http_code} %{redirect_url}' "$GW/api/users/../orders")"

echo "== gateway features"
expect "gateway /health"                       gateway "$(field service "$GW/health")"
expect "X-Request-Id generated"                'X-Request-Id: ' "$(curl -s -D - -o /dev/null "$GW/api/users")"
expect "X-Request-Id kept from client"         'X-Request-Id: demo-123' "$(curl -s -D - -o /dev/null -H 'X-Request-ID: demo-123' "$GW/api/users")"
expect "50 concurrent requests all 200"        ' 50 200' \
  "$(seq 50 | xargs -P 20 -I{} curl -s -o /dev/null -w '%{http_code}\n' "$GW/api/products/{}" | sort | uniq -c | tr -s ' ')"

echo "== upstream down (orders pointed at a closed port)"
stop_gateway
start_gateway ORDER_SERVICE_URL=http://127.0.0.1:1
expect "orders down -> 502"                    502 "$(code "$GW/api/orders")"
expect "502 body is JSON error"                'upstream' "$(field error "$GW/api/orders")"
expect "users still 200 while orders is down"  200 "$(code "$GW/api/users")"

echo "== graceful shutdown"
kill -TERM "$pid"; wait "$pid"; rc=$?; pid=
expect "SIGTERM -> exit 0"                     0 "$rc"
expect "shutdown logged"                       'shutting down' "$(cat "$LOG")"

echo "== gateway log (tail)"
tail -n 6 "$LOG"
echo "== $fails failed"
exit $((fails > 0))
