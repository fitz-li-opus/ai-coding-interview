"use strict";

const crypto = require("node:crypto");
const http = require("node:http");
const https = require("node:https");
const { CircuitBreaker } = require("./breaker");

const HOP_BY_HOP_HEADERS = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);

const RETRYABLE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);
const RETRYABLE_STATUS_CODES = new Set([502, 503, 504]);

class GatewayError extends Error {
  constructor(code, statusCode, message) {
    super(message);
    this.code = code;
    this.statusCode = statusCode;
    this.retryable = [
      "upstream_timeout",
      "upstream_unavailable",
      "bad_gateway",
    ].includes(code);
  }
}

function createGateway(config, dependencies = {}) {
  const now = dependencies.now || Date.now;
  const logger = dependencies.logger || ((entry) => console.log(JSON.stringify(entry)));
  const globalMaxInFlight = config.globalMaxInFlight || 1000;
  const routes = config.routes.map((route) => prepareRoute(route, now));
  const agents = new Set();
  let globalInFlight = 0;

  for (const route of routes) {
    for (const target of route.targets) {
      target.agent = createAgent(target.url, route.maxSockets);
      target.inFlight = 0;
      target.breaker = new CircuitBreaker(route.breaker, now);
      agents.add(target.agent);
    }
  }

  const server = http.createServer((req, res) => {
    handleRequest(req, res, {
      routes,
      globalMaxInFlight,
      logger,
      getGlobalInFlight: () => globalInFlight,
      incrementGlobalInFlight: () => {
        globalInFlight += 1;
      },
      decrementGlobalInFlight: () => {
        globalInFlight -= 1;
      },
    });
  });

  server.on("close", () => {
    for (const agent of agents) {
      agent.destroy();
    }
  });

  return server;
}

function prepareRoute(route, now) {
  const breaker = {
    windowMs: 10_000,
    minRequests: 20,
    failureRatio: 0.5,
    openMs: 5_000,
    halfOpenProbes: 2,
    ...(route.breaker || {}),
  };

  return {
    ...route,
    prefix: normalizePrefix(route.prefix),
    inFlight: 0,
    retry: {
      maxAttempts: 2,
      backoffMs: 25,
      ...(route.retry || {}),
    },
    breaker,
    now,
    targets: route.targets.map((target) => ({
      url: new URL(target),
      displayUrl: target,
    })),
  };
}

function normalizePrefix(prefix) {
  if (!prefix.startsWith("/")) {
    return `/${prefix.replace(/\/+$/, "")}`;
  }
  return prefix.length > 1 ? prefix.replace(/\/+$/, "") : prefix;
}

function createAgent(url, maxSockets) {
  const Agent = url.protocol === "https:" ? https.Agent : http.Agent;
  return new Agent({
    keepAlive: true,
    maxSockets,
    maxFreeSockets: maxSockets,
  });
}

async function handleRequest(req, res, state) {
  const startedAt = Date.now();
  const pathname = new URL(req.url, "http://gateway.local").pathname;
  const route = matchRoute(state.routes, pathname);
  const requestId = getRequestId(req.headers["x-request-id"]);
  const traceparent = getTraceparent(req.headers.traceparent);
  const traceId = traceparent.split("-")[1];

  let target = null;
  let attempts = 0;
  let errorType = null;

  try {
    if (!route) {
      throw new GatewayError("route_not_found", 404, "route not found");
    }

    if (
      state.getGlobalInFlight() >= state.globalMaxInFlight ||
      route.inFlight >= route.maxInFlight
    ) {
      throw new GatewayError("overloaded", 503, "gateway is overloaded");
    }

    state.incrementGlobalInFlight();
    route.inFlight += 1;

    const result = await proxyWithPolicy(req, res, route, {
      requestId,
      traceparent,
    });
    target = result.target;
    attempts = result.attempts;
    errorType = result.errorType;
  } catch (error) {
    if (error instanceof GatewayError) {
      errorType = error.code;
    } else {
      errorType = "internal_error";
    }

    if (!res.headersSent) {
      const statusCode = error.statusCode || 500;
      const code = error.code || "internal_error";
      sendError(res, statusCode, code, error.message, requestId);
    } else {
      res.destroy(error);
    }
  } finally {
    if (route && route.inFlight > 0) {
      route.inFlight -= 1;
      state.decrementGlobalInFlight();
    }

    state.logger({
      time: new Date().toISOString(),
      level: res.statusCode >= 500 ? "error" : "info",
      request_id: requestId,
      trace_id: traceId,
      route: route ? route.prefix : null,
      target: target ? target.displayUrl : null,
      method: req.method,
      path: pathname,
      status: res.statusCode,
      duration_ms: Date.now() - startedAt,
      attempt: attempts,
      error_type: errorType,
    });
  }
}

async function proxyWithPolicy(req, res, route, context) {
  const attempts = [];
  const maxAttempts = canRetryRequest(req)
    ? Math.min(route.retry.maxAttempts, route.targets.length)
    : 1;
  const deadline = Date.now() + route.timeoutMs;
  let lastError = null;
  let lastTarget = null;

  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    const target = selectTarget(route, attempts, deadline);
    lastTarget = target;

    if (!target) {
      if (lastError) {
        throw lastError;
      }
      throw new GatewayError(
        "upstream_unavailable",
        503,
        "no upstream target is available",
      );
    }

    attempts.push(target);
    target.inFlight += 1;

    let breakerOutcome = "failure";
    let upstreamResponse = null;
    let attemptError = null;

    try {
      upstreamResponse = await startProxyAttempt(
        req,
        res,
        target,
        deadline,
        route.timeoutMs,
        context,
      );
      breakerOutcome = RETRYABLE_STATUS_CODES.has(upstreamResponse.statusCode)
        ? "failure"
        : "success";

      if (
        breakerOutcome === "failure" &&
        attempt < maxAttempts &&
        Date.now() < deadline
      ) {
        drainResponse(upstreamResponse);
        await delay(route.retry.backoffMs);
        continue;
      }

      await sendUpstreamResponse(res, upstreamResponse, context.requestId);
      return {
        target,
        attempts: attempt,
        errorType: null,
      };
    } catch (error) {
      attemptError = normalizeProxyError(error);
      lastError = attemptError;

      if (
        attempt < maxAttempts &&
        attemptError.retryable &&
        Date.now() < deadline
      ) {
        await delay(route.retry.backoffMs);
        continue;
      }

      throw attemptError;
    } finally {
      target.inFlight = Math.max(0, target.inFlight - 1);
      if (breakerOutcome === "success") {
        target.breaker.record(true);
      } else if (attemptError || upstreamResponse) {
        target.breaker.record(false);
      }
      if (upstreamResponse && upstreamResponse.destroyed !== true) {
        upstreamResponse.resume();
      }
    }
  }

  throw lastError || new GatewayError(
    "upstream_unavailable",
    503,
    "no upstream target is available",
  );
}

function startProxyAttempt(req, res, target, deadline, timeoutMs, context) {
  return new Promise((resolve, reject) => {
    const remainingMs = Math.min(timeoutMs, deadline - Date.now());
    if (remainingMs <= 0) {
      reject(new GatewayError("upstream_timeout", 504, "upstream timed out"));
      return;
    }

    const headers = filterHeaders(req.headers);
    headers["x-request-id"] = context.requestId;
    headers.traceparent = context.traceparent;
    const options = {
      protocol: target.url.protocol,
      hostname: target.url.hostname,
      port: target.url.port || undefined,
      method: req.method,
      path: req.url,
      headers,
      agent: target.agent,
    };

    const transport = target.url.protocol === "https:" ? https : http;
    let settled = false;
    let timedOut = false;
    const upstreamReq = transport.request(options);

    const finish = (callback, value) => {
      if (settled) {
        return;
      }
      settled = true;
      clearTimeout(timer);
      callback(value);
    };

    const timer = setTimeout(() => {
      timedOut = true;
      upstreamReq.destroy();
      finish(
        reject,
        new GatewayError("upstream_timeout", 504, "upstream timed out"),
      );
    }, remainingMs);

    upstreamReq.on("response", (upstreamResponse) => {
      finish(resolve, upstreamResponse);
    });

    upstreamReq.on("error", (error) => {
      if (timedOut) {
        return;
      }
      finish(reject, error);
    });

    req.on("aborted", () => {
      upstreamReq.destroy(
        new GatewayError("client_aborted", 499, "client aborted request"),
      );
    });
    res.on("close", () => {
      if (!res.writableEnded) {
        upstreamReq.destroy(
          new GatewayError("client_aborted", 499, "client aborted request"),
        );
      }
    });

    if (hasRequestBody(req)) {
      req.pipe(upstreamReq);
    } else {
      upstreamReq.end();
    }
  });
}

async function sendUpstreamResponse(res, upstreamResponse, requestId) {
  if (res.destroyed) {
    drainResponse(upstreamResponse);
    return;
  }

  const headers = filterHeaders(upstreamResponse.headers);
  headers["x-request-id"] = requestId;
  res.writeHead(upstreamResponse.statusCode, headers);

  if (upstreamResponse.statusCode === 204 || upstreamResponse.statusCode === 304) {
    upstreamResponse.resume();
    res.end();
    return;
  }

  await new Promise((resolve, reject) => {
    upstreamResponse.on("error", reject);
    upstreamResponse.on("end", resolve);
    upstreamResponse.pipe(res);
  });
}

function selectTarget(route, attemptedTargets, deadline) {
  if (Date.now() >= deadline) {
    throw new GatewayError("upstream_timeout", 504, "upstream timed out");
  }

  const candidates = route.targets.filter(
    (target) =>
      !attemptedTargets.includes(target) &&
      target.breaker.canAttempt(),
  );

  let selected = null;
  for (const candidate of candidates) {
    if (!selected || candidate.inFlight < selected.inFlight) {
      selected = candidate;
    }
  }

  if (selected && selected.breaker.acquire()) {
    return selected;
  }
  return null;
}

function matchRoute(routes, pathname) {
  let match = null;
  for (const route of routes) {
    if (!matchesPrefix(pathname, route.prefix)) {
      continue;
    }
    if (!match || route.prefix.length > match.prefix.length) {
      match = route;
    }
  }
  return match;
}

function matchesPrefix(pathname, prefix) {
  return pathname === prefix || pathname.startsWith(`${prefix}/`);
}

function canRetryRequest(req) {
  return RETRYABLE_METHODS.has(req.method) && !hasRequestBody(req);
}

function hasRequestBody(req) {
  const contentLength = req.headers["content-length"];
  return (
    req.headers["transfer-encoding"] !== undefined ||
    (contentLength !== undefined && Number(contentLength) > 0)
  );
}

function getRequestId(value) {
  if (typeof value === "string" && value.trim()) {
    return value.trim();
  }
  return crypto.randomUUID();
}

function getTraceparent(value) {
  const match =
    typeof value === "string"
      ? value.match(
        /^00-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$/i,
      )
      : null;

  if (
    match &&
    !/^0+$/.test(match[1]) &&
    !/^0+$/.test(match[2])
  ) {
    return value;
  }

  return `00-${randomHex(16)}-${randomHex(8)}-01`;
}

function randomHex(bytes) {
  return crypto.randomBytes(bytes).toString("hex");
}

function filterHeaders(source) {
  const headers = {};
  const hopByHop = new Set(HOP_BY_HOP_HEADERS);
  const connection = source.connection;

  if (typeof connection === "string") {
    for (const header of connection.split(",")) {
      const name = header.trim().toLowerCase();
      if (name) {
        hopByHop.add(name);
      }
    }
  }

  for (const [name, value] of Object.entries(source)) {
    if (!hopByHop.has(name.toLowerCase())) {
      headers[name] = value;
    }
  }
  return headers;
}

function drainResponse(response) {
  response.resume();
}

function delay(ms) {
  if (!ms) {
    return Promise.resolve();
  }
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function normalizeProxyError(error) {
  if (error instanceof GatewayError) {
    return error;
  }

  if (
    error &&
    ["ECONNREFUSED", "ECONNRESET", "ENOTFOUND", "EAI_AGAIN"].includes(
      error.code,
    )
  ) {
    const gatewayError = new GatewayError(
      "upstream_unavailable",
      503,
      "upstream is unavailable",
    );
    gatewayError.cause = error;
    return gatewayError;
  }

  const gatewayError = new GatewayError(
    "bad_gateway",
    502,
    process.env.GATEWAY_DEBUG ? error.message : "upstream proxy failed",
  );
  gatewayError.cause = error;
  return gatewayError;
}

function sendError(res, statusCode, code, message, requestId) {
  const body = JSON.stringify({
    error: {
      code,
      message,
      requestId,
    },
  });

  res.writeHead(statusCode, {
    "content-type": "application/json",
    "content-length": Buffer.byteLength(body),
    "x-request-id": requestId,
  });
  res.end(body);
}

module.exports = {
  createGateway,
  GatewayError,
};
