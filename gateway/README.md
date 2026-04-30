# API Gateway

Bun + Elysia API gateway for the interview services.

## Run

```bash
bun install
bun run dev
```

Gateway listens on `http://localhost:8080` by default.

## Routes

| Gateway route      | Upstream                |
| ------------------ | ----------------------- |
| `/api/users/**`    | `http://localhost:8081` |
| `/api/orders/**`   | `http://localhost:8082` |
| `/api/products/**` | `http://localhost:8083` |

## Configuration

| Env var                | Default                 |
| ---------------------- | ----------------------- |
| `PORT`                 | `8080`                  |
| `REQUEST_TIMEOUT_MS`   | `5000`                  |
| `RATE_LIMIT_ENABLED`   | `true`                  |
| `RATE_LIMIT_MAX`       | `60`                    |
| `RATE_LIMIT_WINDOW_MS` | `60000`                 |
| `USER_SERVICE_URL`     | `http://localhost:8081` |
| `ORDER_SERVICE_URL`    | `http://localhost:8082` |
| `PRODUCT_SERVICE_URL`  | `http://localhost:8083` |

## Features

- Routing: forwards `/api/users/**`, `/api/orders/**`, and `/api/products/**`.
- Rate limiting: in-memory fixed window per client IP and route prefix.
- Timeout handling: returns `504` when an upstream takes longer than `REQUEST_TIMEOUT_MS`.
- Upstream failures: returns `502` when the upstream connection fails.
- Access logs: writes JSON lines with method, path, status, duration, and request id.

## Demo

Start upstream services from the repo root:

```bash
docker compose up --build
```

Then run:

```bash
curl http://localhost:8080/api/users
curl http://localhost:8080/api/orders
curl http://localhost:8080/api/products
curl http://localhost:8080/health
```

Rate limit demo:

```bash
RATE_LIMIT_MAX=2 bun run start
curl http://localhost:8080/api/users
curl http://localhost:8080/api/users
curl -i http://localhost:8080/api/users
```

## Checks

```bash
bun run check
```
