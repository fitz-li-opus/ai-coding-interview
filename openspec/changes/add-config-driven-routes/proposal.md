## Why

Routes are currently a hard-coded array in `gateway/index.js`. Every time a service is added, removed, or its upstream URL changes, we must edit code and rebuild the container. A production gateway should load its routing table from an external config so that ops can change routes without a code review, and so the same binary can serve multiple environments (dev/staging/prod) by swapping a file or env var.

## What Changes

- Add `gateway/config/routes.yaml` (default path) with a schema:
  ```yaml
  routes:
    - prefix: /api/users
      target: http://user-service:8080
      timeout_ms: 5000       # optional, overrides global
      strip_prefix: false    # optional, default false
  ```
- Load the config at startup; validate with a small schema check (required: `prefix`, `target`; both strings; `prefix` must start with `/`).
- Allow overriding the config path via `ROUTES_CONFIG` env var.
- Fail fast with a clear error message if the file is missing or invalid — do **not** fall back to hard-coded defaults (silent fallbacks hide config drift).
- Keep YAML support as primary (human-friendly); accept JSON if the file ends in `.json` (same shape).

## Capabilities

### New Capabilities
- `route-config`: external configuration file that defines gateway routes and their per-route options.

### Modified Capabilities
- `routing`: route table is now loaded from config instead of being a hard-coded literal. Behavior of a matched route is unchanged.

## Impact

- `gateway/index.js`: replace the `routes` literal with a loader call; add `config/loadRoutes.js`.
- `gateway/package.json`: add `js-yaml` (~30 KB, zero-dep) for YAML parsing.
- `gateway/Dockerfile`: `COPY config/ ./config/`.
- `docker-compose.yml`: optionally mount a host config file via a volume so ops can edit without rebuilding.
- Existing curl contracts unchanged.
