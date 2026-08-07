#!/usr/bin/env bash
#
# End-to-end demo of the gateway.
#
# Prerequisites:
#   docker compose up --build -d     (from the repo root)
#   cd gateway && go run .
#
# Section 6 stops order-service to show a real 502 and starts it again. The trap
# restores it even if the script is interrupted.

set -uo pipefail

GW=${GW:-http://localhost:8080}
REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

bold() { printf '\n\033[1m%s\033[0m\n' "$1"; }
run() { printf '  $ %s\n' "$*"; eval "$@"; }

# Prints "service=... path=..." from a backend response, so the demo shows what
# the *backend* saw rather than just that the gateway returned 200.
echo_backend() {
  python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit('  <not a backend response>')
print('  service=%-16s backend saw path=%s' % (d['service'], d['path']))
"
}

SCRATCH=$(mktemp -d)
cleanup() {
  pkill -f "$SCRATCH/slow_upstream.py" 2>/dev/null
  pkill -f "$SCRATCH/slow-routes.json" 2>/dev/null
  rm -rf "$SCRATCH"
  docker compose -f "$REPO_ROOT/docker-compose.yml" start order-service >/dev/null 2>&1
}
trap cleanup EXIT

# ---------------------------------------------------------------- preflight
bold "0. Preflight"
if ! curl -sf -m 3 "$GW/health" >/dev/null; then
  echo "  gateway is not up on $GW — run 'go run .' in gateway/ first" >&2
  exit 1
fi
echo "  gateway is up on $GW"

# ---------------------------------------------------------------- routing
bold "1. Routing — one port in, three services out"
echo "  The backends echo the path they received, so 'backend saw path' is the"
echo "  proof that /api/<svc> was stripped before forwarding."
for path in /api/users/ /api/users/123 /api/users/123/orders /api/orders/1001 /api/products/; do
  printf '\n  GET %s\n' "$path"
  curl -s -m 5 "$GW$path" | echo_backend
done

# ---------------------------------------------------------------- health
bold "2. The gateway's own /health is reserved ahead of the route table"
run "curl -s $GW/health"; echo

# ---------------------------------------------------------------- 404s
bold "3. No matching route -> 404, not a silent misroute"
run "curl -s -w '  [HTTP %{http_code}]\\n' $GW/api/nope"

bold "4. Prefix matching is segment-aware"
echo "  A plain string prefix would route /api/usersecret to user-service."
run "curl -s -w '  [HTTP %{http_code}]\\n' $GW/api/usersecret"

# ---------------------------------------------------------------- request id
bold "5. X-Request-Id — generated, propagated upstream, returned to the client"
echo "  Generated when the client sends none:"
run "curl -s -D - -o /dev/null $GW/api/users/ | grep -i x-request-id"
echo "  A client-supplied id is honoured instead:"
run "curl -s -D - -o /dev/null -H 'X-Request-Id: trace-42' $GW/api/users/ | grep -i x-request-id"
echo "  Error bodies carry it too, so a failure greps straight to its log line:"
run "curl -s $GW/api/nope"

# ---------------------------------------------------------------- 502
bold "6. Upstream down -> 502 bad_gateway"
run "docker compose -f $REPO_ROOT/docker-compose.yml stop order-service"
run "curl -s -w '  [HTTP %{http_code}]\\n' $GW/api/orders/1"
echo "  Other routes are unaffected — one dead backend does not take the gateway down:"
run "curl -s -w '  [HTTP %{http_code}]\\n' -o /dev/null $GW/api/users/"
run "docker compose -f $REPO_ROOT/docker-compose.yml start order-service"

# ---------------------------------------------------------------- 504
bold "7. Upstream hangs -> 504 gateway_timeout at the configured deadline"
echo "  This needs an upstream that accepts the connection and then never"
echo "  answers. A throwaway gateway on :8090 is pointed at such a listener on"
echo "  a fresh port, so the demo does not have to break a real backend."
cat > "$SCRATCH/slow-routes.json" <<'JSON'
{
  "listen": ":8090",
  "routes": [
    { "path": "/api/slow/**", "target": "http://127.0.0.1:9099", "timeout_ms": 1500 }
  ]
}
JSON
echo "  route: /api/slow/** -> 127.0.0.1:9099, timeout_ms 1500"

# An upstream that accepts the connection and then sits on it. Not netcat: a
# backgrounded `nc -l` reads EOF from stdin and closes the connection at once,
# which produces a 502 rather than the hang this section needs.
cat > "$SCRATCH/slow_upstream.py" <<'PY'
import http.server
import time


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        time.sleep(30)

    def log_message(self, *args):
        pass


http.server.HTTPServer(("127.0.0.1", 9099), Handler).serve_forever()
PY
python3 "$SCRATCH/slow_upstream.py" >"$SCRATCH/upstream.log" 2>&1 &
disown
go run . -config "$SCRATCH/slow-routes.json" >"$SCRATCH/gw.log" 2>&1 &
disown

wait_for() { # wait_for <url>
  for _ in $(seq 1 20); do
    curl -sf -m 1 -o /dev/null "$1" 2>/dev/null && return 0
    sleep 0.5
  done
  return 1
}
if ! wait_for http://localhost:8090/health; then
  echo "  the throwaway gateway did not start; log follows:" >&2
  cat "$SCRATCH/gw.log" >&2
  exit 1
fi
# The slow upstream never answers, so probe the TCP port rather than HTTP.
if ! nc -z 127.0.0.1 9099 2>/dev/null; then
  echo "  the slow upstream did not bind :9099; log follows:" >&2
  cat "$SCRATCH/upstream.log" >&2
  exit 1
fi

run "curl -s -w '  [HTTP %{http_code}] after %{time_total}s\\n' http://localhost:8090/api/slow/x"
echo "  (timeout_ms is 1500 — the elapsed time should match)"

bold "Done — all backends restored."
