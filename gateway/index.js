const crypto = require('node:crypto');
const express = require('express');
const rateLimit = require('express-rate-limit');
const { createProxyMiddleware } = require('http-proxy-middleware');

const PORT = process.env.PORT || 8080;
const SERVICE_NAME = 'gateway';

const envInt = (name, def) => {
  const n = Number.parseInt(process.env[name] ?? '', 10);
  return Number.isFinite(n) && n > 0 ? n : def;
};

const UPSTREAM_TIMEOUT_MS = envInt('UPSTREAM_TIMEOUT_MS', 5000);
const RATE_LIMIT_WINDOW_MS = envInt('RATE_LIMIT_WINDOW_MS', 60000);
const RATE_LIMIT_MAX = envInt('RATE_LIMIT_RPM', 100);
const HEALTH_PROBE_TIMEOUT_MS = envInt('HEALTH_PROBE_TIMEOUT_MS', 500);
const HEALTH_CACHE_MS = envInt('HEALTH_CACHE_MS', 1000);

const routes = [
  { prefix: '/api/users', target: process.env.USER_SERVICE_URL || 'http://user-service:8080' },
  { prefix: '/api/orders', target: process.env.ORDER_SERVICE_URL || 'http://order-service:8080' },
  { prefix: '/api/products', target: process.env.PRODUCT_SERVICE_URL || 'http://product-service:8080' },
].map((r) => ({ ...r, name: new URL(r.target).hostname }));

const TIMEOUT_CODES = new Set(['ETIMEDOUT', 'ECONNABORTED']);

function classifyUpstreamError(err, req) {
  const isTimeout = (req && req.upstreamTimedOut) || TIMEOUT_CODES.has(err && err.code);
  return isTimeout
    ? { status: 504, code: 'GATEWAY_TIMEOUT', message: 'Upstream service timed out.' }
    : { status: 502, code: 'BAD_GATEWAY', message: 'Upstream service is unavailable.' };
}

function handleUpstreamError(target) {
  return (err, req, res) => {
    if (!res || res.writableEnded) return;
    const mapping = classifyUpstreamError(err, req);
    log('error', {
      msg: 'upstream-error',
      request_id: req && req.requestId,
      upstream: target,
      err_code: (err && err.code) || null,
      err_message: (err && err.message) || String(err),
      mapped_status: mapping.status,
    });
    sendErrorEnvelope(res, mapping.status, mapping.code, mapping.message, req && req.requestId);
  };
}

function sendErrorEnvelope(res, status, code, message, requestId) {
  if (!res || res.writableEnded || res.destroyed) return;
  if (res.headersSent) {
    res.destroy();
    return;
  }
  const body = JSON.stringify({
    error: {
      code,
      message,
      request_id: requestId || crypto.randomUUID(),
    },
  });
  try {
    res.writeHead(status, {
      'Content-Type': 'application/json; charset=utf-8',
      'Content-Length': Buffer.byteLength(body),
    });
    res.end(body);
  } catch (writeErr) {
    log('error', { msg: 'error-envelope-write-failed', err: String(writeErr) });
    if (!res.destroyed) res.destroy();
  }
}

function log(level, fields) {
  process.stdout.write(
    JSON.stringify({
      ts: new Date().toISOString(),
      level,
      service: SERVICE_NAME,
      ...fields,
    }) + '\n'
  );
}

const proxyLogger = {
  log: (msg) => log('info', { msg: String(msg) }),
  debug: (msg) => log('debug', { msg: String(msg) }),
  info: (msg) => log('info', { msg: String(msg) }),
  warn: (msg) => log('warn', { msg: String(msg) }),
  error: (msg) => log('error', { msg: String(msg) }),
};

async function probeUpstream(target) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), HEALTH_PROBE_TIMEOUT_MS);
  const start = process.hrtime.bigint();
  try {
    const res = await fetch(`${target}/health`, { signal: controller.signal });
    const latency_ms = Number(process.hrtime.bigint() - start) / 1e6;
    return {
      status: res.ok ? 'healthy' : 'unhealthy',
      http_status: res.status,
      latency_ms: Math.round(latency_ms * 100) / 100,
    };
  } catch (err) {
    return {
      status: 'unhealthy',
      error: err.name === 'AbortError' ? 'timeout' : err.cause?.code || err.code || err.message,
    };
  } finally {
    clearTimeout(timer);
  }
}

let healthCache = { at: 0, body: null };
let inflightHealth = null;

async function aggregateHealth() {
  const now = Date.now();
  if (now - healthCache.at < HEALTH_CACHE_MS && healthCache.body) return healthCache.body;
  if (inflightHealth) return inflightHealth;
  inflightHealth = (async () => {
    try {
      const results = await Promise.all(
        routes.map(async ({ prefix, target, name }) => [
          name,
          { prefix, target, ...(await probeUpstream(target)) },
        ])
      );
      const upstreams = Object.fromEntries(results);
      const healthyCount = results.filter(([, v]) => v.status === 'healthy').length;
      const status =
        healthyCount === results.length ? 'healthy' : healthyCount === 0 ? 'unhealthy' : 'degraded';
      healthCache = { at: Date.now(), body: { status, service: SERVICE_NAME, upstreams } };
      return healthCache.body;
    } finally {
      inflightHealth = null;
    }
  })();
  return inflightHealth;
}

const app = express();
app.disable('x-powered-by');

app.use((req, res, next) => {
  const inbound = req.headers['x-request-id'];
  const requestId =
    typeof inbound === 'string' && /^[A-Za-z0-9_.:-]{1,128}$/.test(inbound)
      ? inbound
      : crypto.randomUUID();
  req.requestId = requestId;
  res.setHeader('X-Request-Id', requestId);

  const start = process.hrtime.bigint();
  res.on('finish', () => {
    const durationMs = Number(process.hrtime.bigint() - start) / 1e6;
    log('info', {
      request_id: requestId,
      method: req.method,
      path: req.originalUrl,
      upstream: req.upstreamTarget || null,
      status: res.statusCode,
      duration_ms: Math.round(durationMs * 100) / 100,
      remote_addr: req.ip,
    });
  });
  next();
});

app.get('/health', async (_req, res) => {
  const body = await aggregateHealth();
  res.status(body.status === 'unhealthy' ? 503 : 200).json(body);
});

const limiter = rateLimit({
  windowMs: RATE_LIMIT_WINDOW_MS,
  limit: RATE_LIMIT_MAX,
  standardHeaders: 'draft-7',
  legacyHeaders: false,
  handler: (req, res) => {
    const resetMs = req.rateLimit?.resetTime
      ? Math.max(0, new Date(req.rateLimit.resetTime).getTime() - Date.now())
      : RATE_LIMIT_WINDOW_MS;
    res.setHeader('Retry-After', Math.ceil(resetMs / 1000));
    res.status(429).json({
      error: {
        code: 'RATE_LIMITED',
        message: 'Too many requests.',
        request_id: req.requestId,
      },
      limit: RATE_LIMIT_MAX,
      window_ms: RATE_LIMIT_WINDOW_MS,
      retry_after_ms: resetMs,
    });
  },
});
app.use(limiter);

for (const { prefix, target } of routes) {
  app.use(
    prefix,
    (req, _res, next) => {
      req.upstreamTarget = target;
      next();
    },
    createProxyMiddleware({
      target,
      changeOrigin: true,
      pathRewrite: (_p, req) => req.originalUrl,
      logger: proxyLogger,
      proxyTimeout: UPSTREAM_TIMEOUT_MS,
      on: {
        proxyReq: (proxyReq, req, res) => {
          if (req.requestId) {
            proxyReq.setHeader('X-Request-Id', req.requestId);
          }
          proxyReq.on('timeout', () => {
            req.upstreamTimedOut = true;
            log('error', {
              msg: 'upstream-error',
              request_id: req.requestId,
              upstream: target,
              err_code: 'ETIMEDOUT',
              err_message: 'proxyTimeout fired',
              mapped_status: 504,
            });
            sendErrorEnvelope(
              res,
              504,
              'GATEWAY_TIMEOUT',
              'Upstream service timed out.',
              req.requestId
            );
            proxyReq.destroy();
          });
        },
        error: handleUpstreamError(target),
        econnreset: handleUpstreamError(target),
      },
    })
  );
}

app.use((req, res) => {
  res.status(404).json({ error: 'no route matched', path: req.originalUrl, request_id: req.requestId });
});

app.listen(PORT, () => {
  log('info', {
    msg: `gateway listening on :${PORT}`,
    routes,
    upstream_timeout_ms: UPSTREAM_TIMEOUT_MS,
    rate_limit: { window_ms: RATE_LIMIT_WINDOW_MS, limit: RATE_LIMIT_MAX },
  });
});
