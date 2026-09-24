"use strict";

const assert = require("node:assert/strict");
const http = require("node:http");
const test = require("node:test");
const { createGateway } = require("../gateway");

async function listen(server) {
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve();
    });
  });
  return `http://127.0.0.1:${server.address().port}`;
}

async function closeServer(server) {
  if (!server.listening) {
    return;
  }
  await new Promise((resolve) => {
    server.close(resolve);
    server.closeAllConnections();
  });
}

async function startUpstream(t, handler) {
  const server = http.createServer(handler);
  const url = await listen(server);
  t.after(() => closeServer(server));
  return { server, url };
}

async function startGateway(t, config, dependencies = {}) {
  const server = createGateway(config, {
    logger: () => {},
    ...dependencies,
  });
  const url = await listen(server);
  t.after(() => closeServer(server));
  return { server, url };
}

function request(url, options = {}) {
  return new Promise((resolve, reject) => {
    const req = http.request(
      url,
      {
        method: options.method || "GET",
        headers: options.headers || {},
      },
      (res) => {
        const chunks = [];
        res.on("data", (chunk) => chunks.push(chunk));
        res.on("end", () => {
          resolve({
            status: res.statusCode,
            headers: res.headers,
            body: Buffer.concat(chunks).toString("utf8"),
          });
        });
        res.on("error", reject);
      },
    );

    req.on("error", reject);
    req.setTimeout(options.timeoutMs || 2000, () => {
      req.destroy(new Error("client request timed out"));
    });

    if (options.body !== undefined) {
      req.write(options.body);
    }
    req.end();
  });
}

function jsonResponse(res, statusCode, body, headers = {}) {
  const payload = JSON.stringify(body);
  res.writeHead(statusCode, {
    "content-type": "application/json",
    "content-length": Buffer.byteLength(payload),
    ...headers,
  });
  res.end(payload);
}

function route(prefix, targets, overrides = {}) {
  const {
    retry: retryOverrides,
    breaker: breakerOverrides,
    ...routeOverrides
  } = overrides;

  return {
    prefix,
    targets: Array.isArray(targets) ? targets : [targets],
    timeoutMs: 200,
    maxInFlight: 10,
    maxSockets: 5,
    ...routeOverrides,
    retry: {
      maxAttempts: 2,
      backoffMs: 1,
      ...(retryOverrides || {}),
    },
    breaker: {
      windowMs: 1000,
      minRequests: 100,
      failureRatio: 0.5,
      openMs: 100,
      halfOpenProbes: 1,
      ...(breakerOverrides || {}),
    },
  };
}

function gatewayConfig(routes, overrides = {}) {
  return {
    globalMaxInFlight: 100,
    routes,
    ...overrides,
  };
}

async function unavailableUrl() {
  const server = http.createServer();
  const url = await listen(server);
  await closeServer(server);
  return url;
}

function parseJson(response) {
  return JSON.parse(response.body);
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

test("routes API prefixes to their matching backend", async (t) => {
  const users = await startUpstream(t, (req, res) => {
    jsonResponse(res, 200, { service: "users" });
  });
  const orders = await startUpstream(t, (req, res) => {
    jsonResponse(res, 200, { service: "orders" });
  });
  const products = await startUpstream(t, (req, res) => {
    jsonResponse(res, 200, { service: "products" });
  });

  const gateway = await startGateway(
    t,
    gatewayConfig([
      route("/api/users", users.url),
      route("/api/orders", orders.url),
      route("/api/products", products.url),
    ]),
  );

  const cases = [
    ["/api/users/", "users"],
    ["/api/orders/123", "orders"],
    ["/api/products", "products"],
  ];

  for (const [path, service] of cases) {
    const response = await request(`${gateway.url}${path}`);
    assert.equal(response.status, 200);
    assert.equal(parseJson(response).service, service);
  }
});

test("does not match a path that is only a string-prefix match", async (t) => {
  const users = await startUpstream(t, (req, res) => {
    res.end("unexpected");
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", users.url)]),
  );

  const response = await request(`${gateway.url}/api/users2`);
  assert.equal(response.status, 404);
  assert.equal(parseJson(response).error.code, "route_not_found");
});

test("preserves method, path, query, body, request headers, and response", async (t) => {
  const upstream = await startUpstream(t, (req, res) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      jsonResponse(
        res,
        201,
        {
          method: req.method,
          path: req.url,
          body: Buffer.concat(chunks).toString("utf8"),
          testHeader: req.headers["x-test"],
        },
        { "x-upstream": "present" },
      );
    });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", upstream.url)]),
  );

  const response = await request(`${gateway.url}/api/users/items?q=one`, {
    method: "POST",
    headers: {
      "content-type": "text/plain",
      "x-test": "yes",
    },
    body: "hello",
  });

  assert.equal(response.status, 201);
  assert.equal(response.headers["x-upstream"], "present");
  assert.deepEqual(parseJson(response), {
    method: "POST",
    path: "/api/users/items?q=one",
    body: "hello",
    testHeader: "yes",
  });
});

test("removes hop-by-hop request and response headers", async (t) => {
  let receivedHeaders = null;
  const upstream = await startUpstream(t, (req, res) => {
    receivedHeaders = req.headers;
    jsonResponse(
      res,
      200,
      { ok: true },
      {
        connection: "x-up-response",
        "x-up-response": "remove-me",
        "x-up-keep": "keep-me",
      },
    );
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", upstream.url)]),
  );

  const response = await request(`${gateway.url}/api/users/`, {
    headers: {
      connection: "x-remove",
      "x-remove": "remove-me",
      "x-keep": "keep-me",
    },
  });

  assert.equal(receivedHeaders["x-remove"], undefined);
  assert.equal(receivedHeaders["x-keep"], "keep-me");
  assert.equal(response.headers["x-up-response"], undefined);
  assert.equal(response.headers["x-up-keep"], "keep-me");
});

test("generates and returns trace context when it is missing", async (t) => {
  let receivedHeaders = null;
  const upstream = await startUpstream(t, (req, res) => {
    receivedHeaders = req.headers;
    jsonResponse(res, 200, { ok: true });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", upstream.url)]),
  );

  const response = await request(`${gateway.url}/api/users/`);

  assert.match(
    response.headers["x-request-id"],
    /^[0-9a-f-]{36}$/,
  );
  assert.match(
    receivedHeaders.traceparent,
    /^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/,
  );
  assert.equal(
    receivedHeaders["x-request-id"],
    response.headers["x-request-id"],
  );
});

test("preserves a valid incoming trace context", async (t) => {
  const traceparent = `00-${"a".repeat(32)}-${"b".repeat(16)}-01`;
  let receivedHeaders = null;
  const upstream = await startUpstream(t, (req, res) => {
    receivedHeaders = req.headers;
    jsonResponse(res, 200, { ok: true });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", upstream.url)]),
  );

  const response = await request(`${gateway.url}/api/users/`, {
    headers: {
      traceparent,
      "x-request-id": "request-123",
    },
  });

  assert.equal(receivedHeaders.traceparent, traceparent);
  assert.equal(receivedHeaders["x-request-id"], "request-123");
  assert.equal(response.headers["x-request-id"], "request-123");
});

test("returns 504 when an upstream exceeds the route deadline", async (t) => {
  const upstream = await startUpstream(t, () => {});
  const gateway = await startGateway(
    t,
    gatewayConfig([
      route("/api/users", upstream.url, {
        timeoutMs: 50,
        retry: { maxAttempts: 1 },
      }),
    ]),
  );

  const startedAt = Date.now();
  const response = await request(`${gateway.url}/api/users/`);

  assert.equal(response.status, 504);
  assert.equal(parseJson(response).error.code, "upstream_timeout");
  assert.ok(Date.now() - startedAt < 1000);
});

test("returns 503 quickly when a backend refuses connections", async (t) => {
  const target = await unavailableUrl();
  const gateway = await startGateway(
    t,
    gatewayConfig([
      route("/api/users", target, {
        timeoutMs: 100,
        retry: { maxAttempts: 1 },
      }),
    ]),
  );

  const startedAt = Date.now();
  const response = await request(`${gateway.url}/api/users/`);

  assert.equal(response.status, 503);
  assert.equal(parseJson(response).error.code, "upstream_unavailable");
  assert.ok(Date.now() - startedAt < 1000);
});

test("isolates a failed route from a healthy route", async (t) => {
  const failedTarget = await unavailableUrl();
  const healthy = await startUpstream(t, (req, res) => {
    jsonResponse(res, 200, { service: "orders" });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([
      route("/api/users", failedTarget, {
        retry: { maxAttempts: 1 },
      }),
      route("/api/orders", healthy.url),
    ]),
  );

  const failed = await request(`${gateway.url}/api/users/`);
  const ok = await request(`${gateway.url}/api/orders/`);

  assert.equal(failed.status, 503);
  assert.equal(ok.status, 200);
  assert.equal(parseJson(ok).service, "orders");
});

test("opens the breaker and recovers after a half-open probe", async (t) => {
  const clock = {
    value: 1000,
    now() {
      return this.value;
    },
    advance(ms) {
      this.value += ms;
    },
  };

  let failing = true;
  let hits = 0;
  const upstream = await startUpstream(t, (req, res) => {
    hits += 1;
    jsonResponse(res, failing ? 503 : 200, {
      failing,
    });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([
      route("/api/users", upstream.url, {
        retry: { maxAttempts: 1 },
        breaker: {
          minRequests: 2,
          failureRatio: 0.5,
          openMs: 100,
          halfOpenProbes: 1,
        },
      }),
    ]),
    { now: () => clock.now() },
  );

  assert.equal((await request(`${gateway.url}/api/users/`)).status, 503);
  assert.equal((await request(`${gateway.url}/api/users/`)).status, 503);
  assert.equal(hits, 2);

  const openResponse = await request(`${gateway.url}/api/users/`);
  assert.equal(openResponse.status, 503);
  assert.equal(parseJson(openResponse).error.code, "upstream_unavailable");
  assert.equal(hits, 2);

  failing = false;
  clock.advance(101);

  assert.equal((await request(`${gateway.url}/api/users/`)).status, 200);
  assert.equal((await request(`${gateway.url}/api/users/`)).status, 200);
  assert.equal(hits, 4);
});

test("does not count a 4xx response as a breaker failure", async (t) => {
  let hits = 0;
  const upstream = await startUpstream(t, (req, res) => {
    hits += 1;
    jsonResponse(res, 404, { missing: true });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([
      route("/api/users", upstream.url, {
        retry: { maxAttempts: 1 },
        breaker: {
          minRequests: 2,
          failureRatio: 0.5,
        },
      }),
    ]),
  );

  for (let index = 0; index < 4; index += 1) {
    const response = await request(`${gateway.url}/api/users/`);
    assert.equal(response.status, 404);
  }
  assert.equal(hits, 4);
});

test("passes through a 500 response without retrying", async (t) => {
  let failedHits = 0;
  let healthyHits = 0;
  const failed = await startUpstream(t, (req, res) => {
    failedHits += 1;
    jsonResponse(res, 500, { error: "boom" });
  });
  const healthy = await startUpstream(t, (req, res) => {
    healthyHits += 1;
    jsonResponse(res, 200, { ok: true });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", [failed.url, healthy.url])]),
  );

  const response = await request(`${gateway.url}/api/users/`);

  assert.equal(response.status, 500);
  assert.equal(failedHits, 1);
  assert.equal(healthyHits, 0);
});

test("retries an idempotent request on a different target", async (t) => {
  const unavailable = await unavailableUrl();
  let healthyHits = 0;
  const healthy = await startUpstream(t, (req, res) => {
    healthyHits += 1;
    jsonResponse(res, 200, { ok: true });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", [unavailable, healthy.url])]),
  );

  const response = await request(`${gateway.url}/api/users/`);

  assert.equal(response.status, 200);
  assert.equal(parseJson(response).ok, true);
  assert.equal(healthyHits, 1);
});

test("does not retry a non-idempotent request", async (t) => {
  const unavailable = await unavailableUrl();
  let healthyHits = 0;
  const healthy = await startUpstream(t, (req, res) => {
    healthyHits += 1;
    jsonResponse(res, 200, { ok: true });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", [unavailable, healthy.url])]),
  );

  const response = await request(`${gateway.url}/api/users/`, {
    method: "POST",
  });

  assert.equal(response.status, 503);
  assert.equal(healthyHits, 0);
});

test("sheds overload immediately and releases the in-flight slot", async (t) => {
  let hits = 0;
  const upstream = await startUpstream(t, (req, res) => {
    hits += 1;
    setTimeout(() => {
      jsonResponse(res, 200, { ok: true });
    }, 80);
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([
      route("/api/users", upstream.url, {
        timeoutMs: 500,
        maxInFlight: 1,
        retry: { maxAttempts: 1 },
      }),
    ]),
  );

  const first = request(`${gateway.url}/api/users/`);
  while (hits === 0) {
    await delay(1);
  }
  const overloaded = await request(`${gateway.url}/api/users/`);

  assert.equal(overloaded.status, 503);
  assert.equal(parseJson(overloaded).error.code, "overloaded");
  assert.equal((await first).status, 200);

  const admitted = await request(`${gateway.url}/api/users/`);
  assert.equal(admitted.status, 200);
  assert.equal(hits, 2);
});

test("emits one correlated structured log record", async (t) => {
  const entries = [];
  const upstream = await startUpstream(t, (req, res) => {
    jsonResponse(res, 200, { ok: true });
  });
  const gateway = await startGateway(
    t,
    gatewayConfig([route("/api/users", upstream.url)]),
    {
      logger: (entry) => entries.push(entry),
    },
  );

  const response = await request(`${gateway.url}/api/users/`, {
    headers: { "x-request-id": "log-request" },
  });
  await delay(0);

  assert.equal(response.status, 200);
  assert.equal(entries.length, 1);
  assert.equal(entries[0].request_id, "log-request");
  assert.equal(entries[0].route, "/api/users");
  assert.equal(entries[0].target, upstream.url);
  assert.equal(entries[0].status, 200);
  assert.equal(entries[0].attempt, 1);
  assert.match(entries[0].trace_id, /^[0-9a-f]{32}$/);
});
