# API Gateway

## Run

```bash
npm install
npm start
```

The gateway listens on `http://localhost:8080` by default.

## Routes

- `GET /api/users` -> `http://localhost:8081`
- `GET /api/orders` -> `http://localhost:8082`
- `GET /api/products` -> `http://localhost:8083`

Paths are forwarded as-is, so the backend response should show the same API path.

## Configuration

| Variable | Default |
| --- | --- |
| `PORT` | `8080` |
| `USER_SERVICE_URL` | `http://localhost:8081` |
| `ORDER_SERVICE_URL` | `http://localhost:8082` |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` |
| `REQUEST_TIMEOUT_MS` | `5000` |
| `API_KEY` | empty, auth disabled |
| `RATE_LIMIT_WINDOW_MS` | `60000` |
| `RATE_LIMIT_MAX_REQUESTS` | `100` |

## Demo

```bash
curl http://localhost:8080/api/users
curl http://localhost:8080/api/orders
curl http://localhost:8080/api/products
curl http://localhost:8080/api/unknown
```
