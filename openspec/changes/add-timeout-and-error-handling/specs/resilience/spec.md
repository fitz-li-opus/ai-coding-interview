## ADDED Requirements

### Requirement: Bounded upstream timeout

The gateway SHALL enforce a bounded timeout on every proxied request to an upstream service. The timeout SHALL default to 5000 milliseconds and SHALL be configurable via the `UPSTREAM_TIMEOUT_MS` environment variable. When the timeout elapses before the upstream returns response headers, the gateway SHALL abort the upstream request and respond to the client with `504 Gateway Timeout` and the uniform error envelope.

#### Scenario: Upstream exceeds the configured timeout
- **WHEN** a proxied request reaches an upstream that does not send response headers within `UPSTREAM_TIMEOUT_MS`
- **THEN** the gateway responds with HTTP status `504`
- **AND** the response body is `{ "error": { "code": "GATEWAY_TIMEOUT", "message": <string>, "request_id": <string> } }`
- **AND** the response `Content-Type` is `application/json`
- **AND** the `X-Request-Id` header matches `request_id` in the body

#### Scenario: Timeout value is overridden via environment
- **WHEN** the gateway process starts with `UPSTREAM_TIMEOUT_MS=1000`
- **THEN** proxied requests time out after 1000 ms instead of the default 5000 ms

### Requirement: Upstream connection failures map to 502

The gateway SHALL respond with `502 Bad Gateway` and the uniform error envelope when the upstream cannot be reached due to connection refusal, DNS resolution failure, or network unreachability. The gateway SHALL NOT surface raw Node error codes, HTML, or empty bodies to the client for these conditions.

#### Scenario: Upstream refuses the TCP connection
- **WHEN** a proxied request targets an upstream that returns `ECONNREFUSED`
- **THEN** the gateway responds with HTTP status `502`
- **AND** the response body is `{ "error": { "code": "BAD_GATEWAY", "message": <string>, "request_id": <string> } }`

#### Scenario: Upstream hostname fails DNS resolution
- **WHEN** a proxied request targets a hostname that resolves to `ENOTFOUND` or `EAI_AGAIN`
- **THEN** the gateway responds with HTTP status `502`
- **AND** the response body uses the uniform error envelope with `code: "BAD_GATEWAY"`

### Requirement: Uniform error envelope

Every gateway-originated error response (timeout, connection failure, uncategorised upstream error) SHALL return a JSON body matching the envelope `{ "error": { "code": <UPPER_SNAKE_CASE_STRING>, "message": <string>, "request_id": <string> } }`. The `request_id` SHALL equal the value set on the incoming request's `X-Request-Id` response header. If no request id has been assigned, the gateway SHALL generate a fresh UUID so the field is never empty.

#### Scenario: Envelope always carries a request_id
- **WHEN** the gateway emits any error envelope
- **THEN** `error.request_id` is a non-empty string
- **AND** it matches the `X-Request-Id` response header when one was set by upstream middleware

#### Scenario: Error codes use stable symbolic names
- **WHEN** the gateway emits an error envelope
- **THEN** `error.code` is one of `BAD_GATEWAY` or `GATEWAY_TIMEOUT` for this change's scope
- **AND** `error.code` is upper snake case

### Requirement: Proxy errors are always handled

The gateway SHALL register an error handler on every proxy route so that no upstream socket error, timeout, or aborted connection reaches the default Node error handler. If the response has already begun streaming (headers already sent) when the error fires, the gateway SHALL terminate the response without attempting to rewrite headers.

#### Scenario: Error fires before headers are sent
- **WHEN** an upstream error occurs and `res.headersSent` is false
- **THEN** the gateway writes the uniform error envelope with the appropriate status
- **AND** no `ERR_HTTP_HEADERS_SENT` error is logged

#### Scenario: Error fires after headers are sent
- **WHEN** an upstream error occurs and `res.headersSent` is true
- **THEN** the gateway destroys the response socket cleanly
- **AND** does not attempt to call `res.writeHead` or `res.json` a second time
