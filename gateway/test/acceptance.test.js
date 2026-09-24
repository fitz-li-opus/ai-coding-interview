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

function request(url) {
  return new Promise((resolve, reject) => {
    const req = http.request(url, (res) => {
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
    });
    req.on("error", reject);
    req.setTimeout(3000, () => req.destroy(new Error("request timed out")));
    req.end();
  });
}

function route(prefix, target) {
  return {
    prefix,
    targets: [target],
    timeoutMs: 2000,
    maxInFlight: 100,
    maxSockets: 20,
    retry: { maxAttempts: 1, backoffMs: 0 },
    breaker: {
      windowMs: 10_000,
      minRequests: 20,
      failureRatio: 0.5,
      openMs: 5000,
      halfOpenProbes: 2,
    },
  };
}

test("routes all real API prefixes to the Compose backends", async (t) => {
  const gateway = createGateway(
    {
      globalMaxInFlight: 100,
      routes: [
        route(
          "/api/users",
          process.env.USER_SERVICE_URL || "http://127.0.0.1:8081",
        ),
        route(
          "/api/orders",
          process.env.ORDER_SERVICE_URL || "http://127.0.0.1:8082",
        ),
        route(
          "/api/products",
          process.env.PRODUCT_SERVICE_URL || "http://127.0.0.1:8083",
        ),
      ],
    },
    { logger: () => {} },
  );
  const baseUrl = await listen(gateway);
  t.after(() => closeServer(gateway));

  const cases = [
    ["/api/users/", "user-service", 3],
    ["/api/orders/", "order-service", 3],
    ["/api/products/", "product-service", 4],
  ];

  for (const [path, service, count] of cases) {
    const response = await request(`${baseUrl}${path}`);
    assert.equal(response.status, 200, `${path} should return 200`);
    assert.ok(response.headers["x-request-id"]);

    const payload = JSON.parse(response.body);
    assert.equal(payload.service, service);
    assert.equal(payload.path, path);
    assert.equal(payload.data.length, count);
  }

  const missing = await request(`${baseUrl}/api/unknown`);
  assert.equal(missing.status, 404);
  assert.equal(JSON.parse(missing.body).error.code, "route_not_found");
});
