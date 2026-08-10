# API Gateway

A small Go reverse proxy for the three services provided by this repository.
The runtime gateway always forwards to the configured real upstream services;
it does not contain service data or mock service implementations.

## Run

Start the three backend services from the repository root, then run:

```bash
cd gateway
go run .
```

The gateway listens on `:8080` by default.

## Routes

| Public route | Default upstream | Forwarded path |
|---|---|---|
| `/api/users[/...]` | `http://localhost:8081` | matched prefix removed |
| `/api/orders[/...]` | `http://localhost:8082` | matched prefix removed |
| `/api/products[/...]` | `http://localhost:8083` | matched prefix removed |

For example, `/api/users/42?expand=roles` is forwarded to
`http://localhost:8081/42?expand=roles`.

## Configuration

| Environment variable | Default |
|---|---|
| `GATEWAY_ADDR` | `:8080` |
| `USER_SERVICE_URL` | `http://localhost:8081` |
| `ORDER_SERVICE_URL` | `http://localhost:8082` |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` |
| `UPSTREAM_TIMEOUT` | `5s` |
| `MAX_REQUEST_BODY_BYTES` | `1048576` |

Upstream URLs must use HTTP or HTTPS and cannot contain credentials, query
parameters, or fragments. The timeout must be positive and at most 60
seconds. The request body limit must be between 1 byte and 10 MiB.

## Verify

```bash
curl -i http://localhost:8080/api/users
curl -i http://localhost:8080/api/orders
curl -i http://localhost:8080/api/products
curl -i http://localhost:8080/health
```

Unmatched routes return JSON `404` responses. Unavailable upstreams return
JSON `502` responses, and upstream timeouts return JSON `504` responses.

Run the automated suite with:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

The tests use in-process HTTP servers so they are deterministic and do not
depend on locally running services. The curl commands above are the live
integration check against the repository's actual backend services.
