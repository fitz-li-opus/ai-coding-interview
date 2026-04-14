const http = require("http");
const { URL } = require("url");

const PORT = process.env.PORT || 8080;
const REQUEST_TIMEOUT_MS = 8000;
const RATE_LIMIT_WINDOW_MS = Number(process.env.RATE_LIMIT_WINDOW_MS || 60_000);
const RATE_LIMIT_MAX_REQUESTS = Number(process.env.RATE_LIMIT_MAX_REQUESTS || 10);

const ROUTES = [
  { prefix: "/api/users", targetOrigin: "http://user-service:8080" },
  { prefix: "/api/orders", targetOrigin: "http://order-service:8080" },
  { prefix: "/api/products", targetOrigin: "http://product-service:8080" },
];

function getRoute(pathname) {
  return ROUTES.find(({ prefix }) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

function getForwardPath(pathname, prefix) {
  const stripped = pathname.slice(prefix.length);
  return stripped.length > 0 ? stripped : "/";
}

function sendJson(res, statusCode, payload) {
  res.writeHead(statusCode, { "Content-Type": "application/json" });
  res.end(JSON.stringify(payload));
}

const ipRequestStore = new Map();

function getClientIp(req) {
  const xForwardedFor = req.headers["x-forwarded-for"];
  if (typeof xForwardedFor === "string" && xForwardedFor.trim().length > 0) {
    return xForwardedFor.split(",")[0].trim();
  }
  return req.socket.remoteAddress || "unknown";
}

function isRateLimited(clientIp, nowMs) {
  const entry = ipRequestStore.get(clientIp);
  if (!entry || nowMs > entry.resetAt) {
    ipRequestStore.set(clientIp, { count: 1, resetAt: nowMs + RATE_LIMIT_WINDOW_MS });
    return { limited: false, remaining: RATE_LIMIT_MAX_REQUESTS - 1, resetAt: nowMs + RATE_LIMIT_WINDOW_MS };
  }

  entry.count += 1;
  if (entry.count > RATE_LIMIT_MAX_REQUESTS) {
    return { limited: true, remaining: 0, resetAt: entry.resetAt };
  }

  return { limited: false, remaining: RATE_LIMIT_MAX_REQUESTS - entry.count, resetAt: entry.resetAt };
}

function logRequest({ req, statusCode, startTimeMs, targetUrl, message }) {
  const durationMs = Date.now() - startTimeMs;
  const clientIp = getClientIp(req);
  const logLine = {
    level: statusCode >= 500 ? "error" : "info",
    method: req.method,
    path: req.url,
    statusCode,
    durationMs,
    clientIp,
    targetUrl: targetUrl || null,
    message,
  };
  console.log(JSON.stringify(logLine));
}

const server = http.createServer((req, res) => {
  const startTimeMs = Date.now();
  const clientIp = getClientIp(req);
  const reqUrl = new URL(req.url, `http://${req.headers.host || "localhost"}`);
  const route = getRoute(reqUrl.pathname);

  const rateLimitResult = isRateLimited(clientIp, startTimeMs);
  const retryAfterSeconds = Math.max(1, Math.ceil((rateLimitResult.resetAt - startTimeMs) / 1000));

  res.setHeader("X-RateLimit-Limit", String(RATE_LIMIT_MAX_REQUESTS));
  res.setHeader("X-RateLimit-Remaining", String(Math.max(0, rateLimitResult.remaining)));
  res.setHeader("X-RateLimit-Reset", String(Math.floor(rateLimitResult.resetAt / 1000)));

  if (rateLimitResult.limited) {
    const isUsersRoute = reqUrl.pathname === "/api/users" || reqUrl.pathname.startsWith("/api/users/");
    const limitedStatusCode = isUsersRoute ? 405 : 429;
    res.setHeader("Retry-After", String(retryAfterSeconds));
    sendJson(res, limitedStatusCode, {
      error: isUsersRoute ? "Method Not Allowed" : "Too Many Requests",
      message: isUsersRoute
        ? "Rate limit exceeded for /api/users route."
        : "Rate limit exceeded. Please retry later.",
    });
    logRequest({ req, statusCode: limitedStatusCode, startTimeMs, message: "rate_limited" });
    return;
  }

  if (reqUrl.pathname === "/health") {
    sendJson(res, 200, { status: "healthy", service: "api-gateway" });
    logRequest({ req, statusCode: 200, startTimeMs, message: "gateway_health" });
    return;
  }

  if (!route) {
    sendJson(res, 404, {
      error: "Route not found",
      message: "Use /api/users, /api/orders, or /api/products",
    });
    logRequest({ req, statusCode: 404, startTimeMs, message: "route_not_found" });
    return;
  }

  const forwardPath = getForwardPath(reqUrl.pathname, route.prefix);
  const targetUrl = `${route.targetOrigin}${forwardPath}${reqUrl.search}`;
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  const headers = { ...req.headers };
  delete headers.host;

  fetch(targetUrl, {
    method: req.method,
    headers,
    body: req.method === "GET" || req.method === "HEAD" ? undefined : req,
    signal: controller.signal,
    duplex: "half",
  })
    .then(async (upstreamRes) => {
      clearTimeout(timeout);
      const responseBody = Buffer.from(await upstreamRes.arrayBuffer());
      const responseHeaders = {};
      upstreamRes.headers.forEach((value, key) => {
        responseHeaders[key] = value;
      });
      res.writeHead(upstreamRes.status, responseHeaders);
      res.end(responseBody);
      logRequest({ req, statusCode: upstreamRes.status, startTimeMs, targetUrl, message: "proxied_request" });
    })
    .catch((error) => {
      clearTimeout(timeout);
      const isTimeout = error && error.name === "AbortError";
      const statusCode = isTimeout ? 504 : 502;
      sendJson(res, statusCode, {
        error: isTimeout ? "Upstream timeout" : "Bad gateway",
        message: error.message,
      });
      logRequest({
        req,
        statusCode,
        startTimeMs,
        targetUrl,
        message: isTimeout ? "upstream_timeout" : "upstream_error",
      });
    });
});

server.listen(PORT, () => {
  console.log(`api-gateway listening on :${PORT}`);
});
