const PORT = Number(Bun.env.PORT ?? 8080);
const upstreamTimeoutMs = () => Number(Bun.env.UPSTREAM_TIMEOUT_MS ?? 30000);

type Route = {
  prefix: string;
  target: string;
};

const routes: Route[] = [
  { prefix: "/api/users", target: Bun.env.USER_SERVICE_URL ?? "http://localhost:8081" },
  { prefix: "/api/orders", target: Bun.env.ORDER_SERVICE_URL ?? "http://localhost:8082" },
  { prefix: "/api/products", target: Bun.env.PRODUCT_SERVICE_URL ?? "http://localhost:8083" },
];

// RFC 7230 §6.1 hop-by-hop headers. These must not be forwarded by a proxy.
const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);

function stripHopByHop(src: Headers): Headers {
  const out = new Headers(src);
  // Per RFC 7230, any header name listed in the Connection header is also hop-by-hop.
  const connection = out.get("connection");
  if (connection) {
    for (const name of connection.split(",")) {
      const n = name.trim();
      if (n) out.delete(n);
    }
  }
  for (const h of HOP_BY_HOP) out.delete(h);
  return out;
}

function appendForwardedFor(headers: Headers, clientIp: string | undefined) {
  if (!clientIp) return;
  const prev = headers.get("x-forwarded-for");
  headers.set("x-forwarded-for", prev ? `${prev}, ${clientIp}` : clientIp);
}

function setIfAbsent(headers: Headers, name: string, value: string) {
  if (!headers.has(name)) headers.set(name, value);
}

function matchRoute(pathname: string): Route | undefined {
  return routes.find(
    (r) => pathname === r.prefix || pathname.startsWith(r.prefix + "/"),
  );
}

function buildUpstreamUrl(req: Request, route: Route): string {
  const url = new URL(req.url);
  const stripped = url.pathname.slice(route.prefix.length) || "/";
  return route.target + stripped + url.search;
}

async function proxy(
  req: Request,
  route: Route,
  clientIp: string | undefined,
): Promise<Response> {
  const upstreamUrl = buildUpstreamUrl(req, route);
  const reqUrl = new URL(req.url);

  const headers = stripHopByHop(req.headers);
  headers.delete("host");
  appendForwardedFor(headers, clientIp);
  setIfAbsent(headers, "x-forwarded-host", reqUrl.host);
  setIfAbsent(headers, "x-forwarded-proto", reqUrl.protocol.replace(":", ""));

  const init: RequestInit & { duplex?: "half" } = {
    method: req.method,
    headers,
    redirect: "manual",
    signal: AbortSignal.timeout(upstreamTimeoutMs()),
  };
  if (req.method !== "GET" && req.method !== "HEAD") {
    init.body = req.body;
    init.duplex = "half";
  }

  let upstreamRes: Response;
  try {
    upstreamRes = await fetch(upstreamUrl, init);
  } catch (err) {
    const e = err as { name?: string; message?: string };
    const isTimeout = e?.name === "TimeoutError" || e?.name === "AbortError";
    console.error(
      `[gateway] upstream ${isTimeout ? "timeout" : "error"} ${upstreamUrl}:`,
      err,
    );
    return Response.json(
      {
        error: isTimeout ? "Gateway Timeout" : "Bad Gateway",
        upstream: route.target,
      },
      { status: isTimeout ? 504 : 502 },
    );
  }

  const outHeaders = stripHopByHop(upstreamRes.headers);
  return new Response(upstreamRes.body, {
    status: upstreamRes.status,
    statusText: upstreamRes.statusText,
    headers: outHeaders,
  });
}

export async function handle(
  req: Request,
  clientIp?: string,
): Promise<Response> {
  const { pathname } = new URL(req.url);
  if (pathname === "/health") {
    return Response.json({ status: "healthy", service: "gateway" });
  }
  const route = matchRoute(pathname);
  if (!route) {
    return Response.json({ error: "Not Found", path: pathname }, { status: 404 });
  }
  return proxy(req, route, clientIp);
}

if (import.meta.main) {
  const server = Bun.serve({
    port: PORT,
    async fetch(req, server) {
      const start = performance.now();
      const ip = server.requestIP(req)?.address;
      const res = await handle(req, ip);
      const ms = (performance.now() - start).toFixed(1);
      const { pathname } = new URL(req.url);
      console.log(`${req.method} ${pathname} -> ${res.status} (${ms}ms)`);
      return res;
    },
  });

  console.log(`[gateway] listening on http://localhost:${server.port}`);
  for (const r of routes) console.log(`  ${r.prefix}/** -> ${r.target}`);
}
