# Gateway Development Guide

## Architecture

- `main.go`: thin entrypoint only.
- `internal/bootstrap`: startup + graceful shutdown.
- `internal/config`: YAML config and env override.
- `internal/di`: DI container wiring.
- `internal/gateway`: handler, proxy, middleware.
- `config/gateway.dev.yaml`: local runtime config.

## Why This Structure

- Keeps business/routing logic out of entrypoint.
- Makes dependencies explicit and easier to unit test.
- Supports environment-agnostic config using YAML + env override.

## Risk Fixes Included

- Uses `ReverseProxy.Rewrite` instead of legacy `Director`.
- Drops inbound forwarded headers by default and rebuilds using `SetXForwarded`.
- Fixes readiness concurrency model using channel + `WaitGroup` to avoid race on shared slice.

## Test Commands

```bash
cd gateway
go test ./...
go test -race ./...
```

## k6 Load Test

```bash
k6 run tests/k6/gateway.js
```

