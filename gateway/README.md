# Gateway (Bun)

Minimal API Gateway built with Bun's built-in `Bun.serve` + `fetch`. Zero runtime dependencies.

## Run

```bash
# 1. Start backends
docker compose up --build -d

# 2. Start gateway
cd gateway
bun run start            # listens on :8080
# or: bun run dev        # with hot reload
```

Override defaults with env vars: `PORT`, `USER_SERVICE_URL`, `ORDER_SERVICE_URL`, `PRODUCT_SERVICE_URL`.

## Routing

| Prefix              | Upstream              |
|---------------------|-----------------------|
| `/api/users/**`     | http://localhost:8081 |
| `/api/orders/**`    | http://localhost:8082 |
| `/api/products/**`  | http://localhost:8083 |

Prefix is stripped before forwarding (`/api/users/42` → upstream `/42`).
Gateway's own `/health` is served locally without hitting any upstream.

## Manual verification (curl)

```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/users
curl http://localhost:8080/api/orders
curl http://localhost:8080/api/products
```

## Tests (no docker needed)

`bun test` spins up in-process stub services on 8081/8082/8083 and exercises the gateway handler.

```bash
bun test
# or equivalent via bunx:
bunx --bun test
```

## Smoke test against a running stack

Hits a live gateway + real docker services:

```bash
bun run smoke
# override target: GATEWAY_URL=http://localhost:8080 bun run smoke
```
