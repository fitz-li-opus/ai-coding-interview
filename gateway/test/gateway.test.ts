import { beforeAll, describe, expect, test } from "bun:test";
import { createApp } from "../src/app.ts";
import type { GatewayConfig } from "../src/config.ts";
import type { Fetcher } from "../src/proxy.ts";
import { findRoute } from "../src/proxy.ts";

let config: GatewayConfig;

beforeAll(() => {
  config = {
    port: 8080,
    rateLimit: {
      enabled: false,
      maxRequests: 60,
      windowMs: 60_000,
    },
    requestTimeoutMs: 1_000,
    services: [
      {
        name: "user-service",
        prefix: "/api/users",
        upstreamUrl: "http://user-service.test",
      },
      {
        name: "order-service",
        prefix: "/api/orders",
        upstreamUrl: "http://order-service.test",
      },
      {
        name: "product-service",
        prefix: "/api/products",
        upstreamUrl: "http://product-service.test",
      },
    ],
  };
});

const echoFetch: Fetcher = async (input, init) => {
  const request = new Request(input, init);
  const url = new URL(request.url);

  return Response.json({
    body: await request.text(),
    method: request.method,
    path: url.pathname,
    query: url.search,
    requestId: request.headers.get("x-request-id"),
  });
};

describe("route matching", () => {
  test("matches exact service prefixes and nested paths", () => {
    expect(findRoute("/api/users", config.services)?.name).toBe("user-service");
    expect(findRoute("/api/users/123", config.services)?.name).toBe("user-service");
    expect(findRoute("/api/users2", config.services)).toBeUndefined();
  });
});

describe("gateway proxy", () => {
  test("returns gateway health without proxying", async () => {
    const app = createApp(config, async () => {
      throw new Error("health should not proxy");
    });
    const response = await app.handle(new Request("http://gateway.test/health"));

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      service: "api-gateway",
      status: "healthy",
    });
  });

  test("forwards requests and preserves path, query, method, body, and request id", async () => {
    const app = createApp(config, echoFetch);
    const response = await app.handle(
      new Request("http://gateway.test/api/orders/1001?expand=user", {
        body: "payload",
        headers: {
          "x-request-id": "req-123",
        },
        method: "POST",
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      body: "payload",
      method: "POST",
      path: "/api/orders/1001",
      query: "?expand=user",
      requestId: "req-123",
    });
  });

  test("returns 404 for unknown routes", async () => {
    const app = createApp(config);
    const response = await app.handle(new Request("http://gateway.test/api/inventory"));

    expect(response.status).toBe(404);
    await expect(response.json()).resolves.toMatchObject({
      error: "not_found",
    });
  });

  test("returns 502 when upstream connection fails", async () => {
    const app = createApp(config, async () => {
      throw new TypeError("connect ECONNREFUSED");
    });
    const response = await app.handle(new Request("http://gateway.test/api/users"));

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toMatchObject({
      error: "bad_gateway",
    });
  });

  test("returns 504 when upstream request times out", async () => {
    const app = createApp(
      {
        ...config,
        requestTimeoutMs: 1,
      },
      async (_input, init) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () =>
            reject(new DOMException("aborted", "AbortError")),
          );
        }),
    );
    const response = await app.handle(new Request("http://gateway.test/api/users"));

    expect(response.status).toBe(504);
    await expect(response.json()).resolves.toMatchObject({
      error: "gateway_timeout",
    });
  });

  test("rate limits requests by client and route", async () => {
    const app = createApp(
      {
        ...config,
        rateLimit: {
          enabled: true,
          maxRequests: 2,
          windowMs: 60_000,
        },
      },
      echoFetch,
    );

    expect((await app.handle(new Request("http://gateway.test/api/users"))).status).toBe(200);
    expect((await app.handle(new Request("http://gateway.test/api/users"))).status).toBe(200);

    const response = await app.handle(new Request("http://gateway.test/api/users"));

    expect(response.status).toBe(429);
    expect(response.headers.get("retry-after")).toBe("60");
    await expect(response.json()).resolves.toMatchObject({
      error: "rate_limited",
    });
  });

  test("logs proxied requests with status, duration, and request id", async () => {
    const entries: unknown[] = [];
    const app = createApp(config, echoFetch, (entry) => entries.push(entry));

    const response = await app.handle(
      new Request("http://gateway.test/api/orders", {
        headers: {
          "x-request-id": "req-log",
        },
      }),
    );

    expect(response.status).toBe(200);
    expect(entries).toHaveLength(1);
    expect(entries[0]).toMatchObject({
      method: "GET",
      path: "/api/orders",
      requestId: "req-log",
      status: 200,
    });
    expect(entries[0]).toHaveProperty("durationMs");
  });
});
