# Python API Gateway

This directory contains the P0 and P1 implementation of the API Gateway described in
the repository-level `req.md`.

## Requirements

- Python 3.11+
- The three backend services running through Docker Compose

## Start the backend services

From the repository root:

```bash
docker compose up --build
```

The services are exposed on host ports 8081, 8082, and 8083.

## Install and run the gateway

In a second terminal:

```bash
cd gateway
python -m venv .venv
source .venv/bin/activate
python -m pip install -r requirements.txt
python main.py
```

The gateway listens on `http://localhost:8080` by default.

For development and testing, install the development dependencies instead:

```bash
python -m pip install -r requirements-dev.txt
pytest -q
```

## Configuration

| Environment variable | Default |
|---|---|
| `GATEWAY_HOST` | `0.0.0.0` |
| `GATEWAY_PORT` | `8080` |
| `USER_SERVICE_URL` | `http://localhost:8081` |
| `ORDER_SERVICE_URL` | `http://localhost:8082` |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` |
| `UPSTREAM_TIMEOUT_SECONDS` | `5` |

Example:

```bash
GATEWAY_PORT=9000 UPSTREAM_TIMEOUT_SECONDS=3 python main.py
```

When the gateway runs in the same Docker network as the services, configure the
upstream URLs as `http://user-service:8080`, `http://order-service:8080`, and
`http://product-service:8080`.

## Verify routing

```bash
curl -i http://localhost:8080/api/users
curl -i http://localhost:8080/api/orders/1001
curl -i 'http://localhost:8080/api/products?category=electronics'
```

The upstream `path` values in these responses should be `/`, `/1001`, and `/`
respectively. The query string in the third request is forwarded unchanged.

Verify method and request-body forwarding:

```bash
curl -i -X POST \
  -H 'Content-Type: application/json' \
  -d '{"example":true}' \
  http://localhost:8080/api/users/123
```

Verify an unknown route:

```bash
curl -i http://localhost:8080/api/unknown
curl -i http://localhost:8080/not-configured
```

Both requests return a JSON `404` response. If an upstream is unavailable or
times out, the gateway returns JSON with status `502` or `504` respectively.

Verify gateway liveness and request ID propagation:

```bash
curl -i http://localhost:8080/health
curl -i -H 'X-Request-ID: demo-request' http://localhost:8080/api/users
```

`/health` reports only the gateway process state and does not depend on any
upstream. Every response includes `X-Request-ID`; error JSON also includes the
same value. Access logs contain request ID, method, path, status, latency, and
the selected target without logging Authorization or Cookie values.

## Implemented scope

Implemented:

- Configurable routing for users, orders, and products
- Exact route matching and prefix removal
- HTTP method, body, query string, and header forwarding
- Upstream status, body, and end-to-end response header forwarding
- Hop-by-hop header filtering
- Shared HTTPX connection pool with a configurable timeout
- JSON errors for 404, 502, 504, and unexpected 500 responses
- Gateway liveness endpoint at `/health`
- Generated or client-supplied request ID propagation
- Access logging with latency and selected target
- MockTransport-based tests that do not require Docker
- Graceful shared-client cleanup during application shutdown

Upstream readiness checks and streaming downstream responses remain P2 features.
