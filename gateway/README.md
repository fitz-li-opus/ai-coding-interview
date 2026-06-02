# API Gateway

Lightweight Go API gateway for the interview scaffold. It listens on one client-facing port and proxies the required API route families to the provided backend services.

## Commands

Run tests:

```bash
go test ./...
```

Run the gateway:

```bash
go run .
```

Run with custom configuration:

```bash
PORT=8080 \
USER_SERVICE_URL=http://localhost:8081 \
ORDER_SERVICE_URL=http://localhost:8082 \
PRODUCT_SERVICE_URL=http://localhost:8083 \
UPSTREAM_TIMEOUT_MS=2000 \
RATE_LIMIT_REQUESTS=100 \
RATE_LIMIT_WINDOW_MS=60000 \
go run .
```

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `8080` | Gateway listen port. |
| `USER_SERVICE_URL` | `http://localhost:8081` | Upstream URL for `/api/users` requests. |
| `ORDER_SERVICE_URL` | `http://localhost:8082` | Upstream URL for `/api/orders` requests. |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` | Upstream URL for `/api/products` requests. |
| `UPSTREAM_TIMEOUT_MS` | `2000` | Per-request upstream timeout in milliseconds. |
| `RATE_LIMIT_REQUESTS` | `100` | Process-local request limit per window. |
| `RATE_LIMIT_WINDOW_MS` | `60000` | Process-local rate limit window in milliseconds. |

Invalid upstream URLs, timeout values, or rate limit values fail gateway startup.

## Routes

| Gateway route | Upstream |
| --- | --- |
| `/api/users` and `/api/users/**` | `USER_SERVICE_URL` |
| `/api/orders` and `/api/orders/**` | `ORDER_SERVICE_URL` |
| `/api/products` and `/api/products/**` | `PRODUCT_SERVICE_URL` |

The gateway strips the `/api/<resource>` prefix before forwarding. For example, `/api/users/health` forwards to `/health` on the user service. Near-miss paths such as `/api/users-extra` do not match.

## Manual Verification

Start the backend services from the repository root:

```bash
docker compose up --build
```

Start the gateway from `gateway/`:

```bash
go run .
```

Verify routing:

```bash
curl http://localhost:8080/api/users/
curl http://localhost:8080/api/orders/
curl http://localhost:8080/api/products/
curl http://localhost:8080/api/users/health
curl http://localhost:8080/api/orders/health
curl http://localhost:8080/api/products/health
```

Verify unmatched routes:

```bash
curl -i http://localhost:8080/api/inventory/
curl -i http://localhost:8080/ready
```

Verify a low rate limit:

```bash
RATE_LIMIT_REQUESTS=1 RATE_LIMIT_WINDOW_MS=60000 go run .
curl -i http://localhost:8080/api/users/
curl -i http://localhost:8080/api/users/
```

The second request should return `429 Too Many Requests`.

## Minimal Behavior

- Single gateway entry point on `PORT`.
- Strict route matching for users, orders, and products.
- Prefix rewriting before upstream proxying.
- Reverse proxy forwarding of method, query string, body, and normal request headers.
- `404 Not Found` for unsupported routes.
- `5xx` gateway response for unavailable upstreams.
- `429 Too Many Requests` when the process-local rate limit is exceeded.

The gateway does not expose `/ready` and does not aggregate backend readiness. Backend health remains available through proxied routes such as `/api/users/health`.

## Scalability Considerations

The gateway keeps route configuration static and request handling independent of shared external state, so multiple instances can run behind a load balancer with equivalent environment configuration.

The rate limiter is process-local. In a horizontal deployment, this means the effective global limit is the per-instance limit multiplied by the number of gateway instances unless a shared limiter is introduced.

Future production additions could include service discovery, dynamic config reload, distributed rate limiting, centralized observability, bounded retries for idempotent requests, authentication, authorization, TLS termination, request body limits, and caching.
