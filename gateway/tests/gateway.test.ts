import { describe, test, expect, beforeAll, afterAll } from "bun:test";
import { handle } from "../index";

type Stub = ReturnType<typeof Bun.serve>;

const stubs: Stub[] = [];

function stubService(port: number, name: string): Stub {
  return Bun.serve({
    port,
    async fetch(req) {
      const url = new URL(req.url);

      const delay = Number(url.searchParams.get("delay") ?? 0);
      if (delay > 0) await Bun.sleep(delay);

      if (url.pathname === "/health") {
        return Response.json({ status: "healthy", service: name });
      }

      const reqHeaders: Record<string, string> = {};
      req.headers.forEach((v, k) => {
        reqHeaders[k] = v;
      });

      const extraResHeaders = new Headers();
      if (url.searchParams.get("hophdrs") === "1") {
        extraResHeaders.set("Keep-Alive", "timeout=5");
        extraResHeaders.set("X-Real-Data", "ok");
      }

      return Response.json(
        {
          service: name,
          path: url.pathname,
          method: req.method,
          query: url.search,
          reqHeaders,
        },
        { headers: extraResHeaders },
      );
    },
  });
}

beforeAll(() => {
  stubs.push(
    stubService(8081, "user-service"),
    stubService(8082, "order-service"),
    stubService(8083, "product-service"),
  );
});

afterAll(() => {
  for (const s of stubs) s.stop(true);
});

async function call(path: string, init?: RequestInit, clientIp?: string) {
  return handle(new Request(`http://gateway.test${path}`, init), clientIp);
}

describe("gateway routing", () => {
  test("gateway /health responds locally without hitting upstream", async () => {
    const res = await call("/health");
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ status: "healthy", service: "gateway" });
  });

  test("/api/users -> user-service (port 8081), prefix stripped to /", async () => {
    const res = await call("/api/users");
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.service).toBe("user-service");
    expect(body.path).toBe("/");
  });

  test("/api/users/42 -> user-service /42", async () => {
    const res = await call("/api/users/42");
    const body = await res.json();
    expect(body.service).toBe("user-service");
    expect(body.path).toBe("/42");
  });

  test("/api/orders -> order-service (port 8082)", async () => {
    const res = await call("/api/orders");
    const body = await res.json();
    expect(body.service).toBe("order-service");
  });

  test("/api/products -> product-service (port 8083)", async () => {
    const res = await call("/api/products/abc?limit=5");
    const body = await res.json();
    expect(body.service).toBe("product-service");
    expect(body.path).toBe("/abc");
    expect(body.query).toBe("?limit=5");
  });

  test("unknown prefix returns 404", async () => {
    const res = await call("/api/unknown");
    expect(res.status).toBe(404);
  });

  test("POST body is forwarded", async () => {
    const res = await call("/api/orders", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ item: "x" }),
    });
    const body = await res.json();
    expect(body.service).toBe("order-service");
    expect(body.method).toBe("POST");
  });
});

describe("forwarded headers", () => {
  test("sets X-Forwarded-For to client IP when none exists", async () => {
    const res = await call("/api/users", undefined, "203.0.113.7");
    const body = await res.json();
    expect(body.reqHeaders["x-forwarded-for"]).toBe("203.0.113.7");
  });

  test("appends client IP to existing X-Forwarded-For chain", async () => {
    const res = await call(
      "/api/users",
      { headers: { "x-forwarded-for": "198.51.100.1, 198.51.100.2" } },
      "203.0.113.7",
    );
    const body = await res.json();
    expect(body.reqHeaders["x-forwarded-for"]).toBe(
      "198.51.100.1, 198.51.100.2, 203.0.113.7",
    );
  });

  test("sets X-Forwarded-Host/Proto when not provided", async () => {
    const res = await call("/api/users");
    const body = await res.json();
    expect(body.reqHeaders["x-forwarded-host"]).toBe("gateway.test");
    expect(body.reqHeaders["x-forwarded-proto"]).toBe("http");
  });

  test("preserves incoming X-Forwarded-Host/Proto from upstream LB", async () => {
    const res = await call("/api/users", {
      headers: {
        "x-forwarded-host": "public.example.com",
        "x-forwarded-proto": "https",
      },
    });
    const body = await res.json();
    expect(body.reqHeaders["x-forwarded-host"]).toBe("public.example.com");
    expect(body.reqHeaders["x-forwarded-proto"]).toBe("https");
  });

  test("omits X-Forwarded-For when client IP is unknown", async () => {
    const res = await call("/api/users");
    const body = await res.json();
    expect(body.reqHeaders["x-forwarded-for"]).toBeUndefined();
  });
});

describe("hop-by-hop header stripping", () => {
  test("strips standard hop-by-hop headers from request", async () => {
    const res = await call("/api/users", {
      headers: {
        "keep-alive": "timeout=5",
        "proxy-authorization": "Basic xxx",
        te: "trailers",
      },
    });
    const body = await res.json();
    expect(body.reqHeaders["keep-alive"]).toBeUndefined();
    expect(body.reqHeaders["proxy-authorization"]).toBeUndefined();
    expect(body.reqHeaders["te"]).toBeUndefined();
  });

  test("strips headers named in Connection header (RFC 7230)", async () => {
    const res = await call("/api/users", {
      headers: {
        connection: "x-custom-hop, x-also-hop",
        "x-custom-hop": "secret",
        "x-also-hop": "secret2",
        "x-end-to-end": "keep-me",
      },
    });
    const body = await res.json();
    expect(body.reqHeaders["x-custom-hop"]).toBeUndefined();
    expect(body.reqHeaders["x-also-hop"]).toBeUndefined();
    // Note: fetch client may re-add its own Connection directive (e.g. "keep-alive")
    // for its connection pool; the contract we care about is that the incoming
    // "x-custom-hop, x-also-hop" directive is not propagated.
    expect(body.reqHeaders["connection"]).not.toContain("x-custom-hop");
    expect(body.reqHeaders["x-end-to-end"]).toBe("keep-me");
  });

  test("strips hop-by-hop headers from upstream response", async () => {
    const res = await call("/api/users?hophdrs=1");
    expect(res.headers.get("keep-alive")).toBeNull();
    expect(res.headers.get("x-real-data")).toBe("ok");
  });
});

describe("upstream timeout", () => {
  test("returns 504 when upstream exceeds UPSTREAM_TIMEOUT_MS", async () => {
    const prev = process.env.UPSTREAM_TIMEOUT_MS;
    process.env.UPSTREAM_TIMEOUT_MS = "50";
    try {
      const res = await call("/api/products?delay=300");
      expect(res.status).toBe(504);
      const body = await res.json();
      expect(body.error).toBe("Gateway Timeout");
    } finally {
      if (prev === undefined) delete process.env.UPSTREAM_TIMEOUT_MS;
      else process.env.UPSTREAM_TIMEOUT_MS = prev;
    }
  });
});
