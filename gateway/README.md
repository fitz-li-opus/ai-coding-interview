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

| Env var               | Default                 |
| --------------------- | ----------------------- |
| `PORT`                | `8080`                  |
| `REQUEST_TIMEOUT_MS`  | `5000`                  |
| `USER_SERVICE_URL`    | `http://localhost:8081` |
| `ORDER_SERVICE_URL`   | `http://localhost:8082` |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` |

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

## Checks

```bash
bun run check
```
