#!/usr/bin/env bash
# Acceptance check against the requirements stated in the top-level README.
# Every check maps to one line of the task description, and fails loudly.
#
#   docker compose up --build -d
#   cd gateway && go run . -config config.json
#   ./verify.sh
set -uo pipefail

GW="${GW:-http://localhost:8088}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
pass=0
fail=0

ok()   { printf '  \033[32mPASS\033[0m  %s\n' "$1"; pass=$((pass + 1)); }
bad()  { printf '  \033[31mFAIL\033[0m  %s\n'   "$1"; fail=$((fail + 1)); [ $# -gt 1 ] && printf '        %s\n' "$2"; }
section() { printf '\n\033[1m%s\033[0m\n' "$1"; }

# The backends echo which service answered, so this proves the request really
# reached the intended one rather than merely returning 200 from somewhere.
expect_service() {
  local path="$1" want="$2"
  local got
  got=$(curl -s --max-time 5 "$GW$path" | grep -o '"service":"[^"]*"' | head -1 | cut -d'"' -f4)
  if [ "$got" = "$want" ]; then
    ok "$path -> $want"
  else
    bad "$path -> $want" "got service=${got:-<no response>}"
  fi
}

expect_status() {
  local path="$1" want="$2" desc="$3" method="${4:-GET}"
  local got
  got=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 -X "$method" "$GW$path")
  if [ "$got" = "$want" ]; then ok "$desc ($got)"; else bad "$desc" "want $want, got $got"; fi
}

section "0. Preconditions"
if curl -s -o /dev/null --max-time 3 "$GW/health"; then
  ok "gateway is reachable at $GW"
else
  bad "gateway is reachable at $GW" "start it with: go run . -config config.json"
  echo; echo "aborting: nothing to test against"; exit 1
fi
for port in 8081 8082 8083; do
  if curl -s -o /dev/null --max-time 3 "http://localhost:$port/health"; then
    ok "backend :$port is up"
  else
    bad "backend :$port is up" "run: docker compose up --build -d"
  fi
done

section "1. One port in, three services out"
expect_service /api/users    user-service
expect_service /api/orders   order-service
expect_service /api/products product-service

section "2. Prefix routing covers sub-paths (/api/users/**)"
expect_service /api/users/123            user-service
expect_service /api/users/123/orders     user-service
expect_service /api/orders/1001          order-service
expect_service /api/products/9?x=1       product-service

section "3. The gateway is transparent"
# Same bytes through the gateway as straight from the backend, modulo the
# timestamp the backend regenerates per request.
strip_ts='s/"timestamp":"[^"]*"//'
direct=$(curl -s --max-time 5 http://localhost:8081/ | sed "$strip_ts")
proxied=$(curl -s --max-time 5 "$GW/api/users" | sed "$strip_ts")
if [ -n "$direct" ] && [ "$direct" = "$proxied" ]; then
  ok "response body is byte-identical to hitting :8081 directly"
else
  bad "response body is byte-identical to hitting :8081 directly" "bodies differ"
fi

method=$(curl -s --max-time 5 -X POST "$GW/api/orders" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
if [ "$method" = "POST" ]; then ok "non-GET methods are forwarded (POST)"; else bad "non-GET methods are forwarded (POST)" "backend saw method=${method:-none}"; fi

section "4. Failure modes are the gateway's own, not a leaked backend error"
expect_status /api/nope       404 "unknown prefix is a gateway 404"
expect_status /nope           404 "unrouted root path is a gateway 404"
expect_status /api/usersfoo   404 "a longer path segment does not match /api/users"

section "5. Repository constraints from the README"
cd "$ROOT"
if git diff --quiet HEAD -- services docker-compose.yml 2>/dev/null; then
  ok "services/ and docker-compose.yml are unmodified"
else
  bad "services/ and docker-compose.yml are unmodified" "$(git diff --name-only HEAD -- services docker-compose.yml | paste -sd, -)"
fi

stray=$(git status --porcelain | awk '{print $2}' | grep -v '^gateway/' || true)
if [ -z "$stray" ]; then
  ok "all new code lives under gateway/"
else
  bad "all new code lives under gateway/" "also changed: $(echo "$stray" | paste -sd, -)"
fi

section "6. The code itself"
if (cd gateway && go vet ./... >/dev/null 2>&1); then ok "go vet is clean"; else bad "go vet is clean"; fi
if (cd gateway && go test ./... -race -count=1 >/dev/null 2>&1); then
  ok "go test -race passes"
else
  bad "go test -race passes" "run: cd gateway && go test ./... -race"
fi

printf '\n\033[1m%d passed, %d failed\033[0m\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
