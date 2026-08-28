#!/usr/bin/env bash
# End-to-end demo of the gateway. Run the backends and the gateway first:
#   docker compose up --build -d
#   cd gateway && go run . -config config.json
set -uo pipefail

GW="${GW:-http://localhost:8088}"
COMPOSE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

hr() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
show() { printf '$ %s\n' "$*"; eval "$*"; printf '\n'; }

hr "1. Routing: one port in, three services out"
# The backends echo the path they received, which proves the prefix was stripped.
show "curl -s $GW/api/users"
show "curl -s $GW/api/orders"
show "curl -s $GW/api/products"

hr "2. Path is rewritten, query and sub-paths survive"
show "curl -s '$GW/api/users/123?limit=10'"

hr "3. Every response carries a request id for tracing"
show "curl -s -D - -o /dev/null $GW/api/users | grep -i -E 'HTTP/|X-Request-Id'"

hr "4. Unknown prefix is a gateway 404, not a backend error"
show "curl -s $GW/api/nope"

hr "5. Gateway health shows the live upstream pool"
show "curl -s $GW/health"

hr "6. Failover: kill user-service, the gateway takes it out of rotation"
show "docker compose -f '$COMPOSE_DIR/docker-compose.yml' stop user-service"
echo "waiting 7s for the health checker to notice..."
sleep 7
show "curl -s -o /dev/null -w 'status=%{http_code}\n' $GW/api/users"
show "curl -s $GW/health"
echo ">> orders and products are untouched:"
show "curl -s -o /dev/null -w 'status=%{http_code}\n' $GW/api/orders"

hr "7. Recovery is automatic, no gateway restart"
show "docker compose -f '$COMPOSE_DIR/docker-compose.yml' start user-service"
echo "waiting 8s for the health checker to notice..."
sleep 8
show "curl -s -o /dev/null -w 'status=%{http_code}\n' $GW/api/users"
show "curl -s $GW/health"
