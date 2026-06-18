import express from 'express';
import { createProxyMiddleware } from 'http-proxy-middleware';

const app = express();

const config = {
  port: Number(process.env.PORT || 8080),
  apiKey: process.env.API_KEY || '',
  requestTimeoutMs: Number(process.env.REQUEST_TIMEOUT_MS || 5000),
  rateLimitWindowMs: Number(process.env.RATE_LIMIT_WINDOW_MS || 60_000),
  rateLimitMaxRequests: Number(process.env.RATE_LIMIT_MAX_REQUESTS || 100),
  services: [
    {
      name: 'user-service',
      prefix: '/api/users',
      target: process.env.USER_SERVICE_URL || 'http://localhost:8081',
    },
    {
      name: 'order-service',
      prefix: '/api/orders',
      target: process.env.ORDER_SERVICE_URL || 'http://localhost:8082',
    },
    {
      name: 'product-service',
      prefix: '/api/products',
      target: process.env.PRODUCT_SERVICE_URL || 'http://localhost:8083',
    },
  ],
};

const rateLimitBuckets = new Map();

function jsonError(res, status, code, message) {
  return res.status(status).json({ error: { code, message } });
}

function isPrefixMatch(path, prefix) {
  return path === prefix || path.startsWith(`${prefix}/`);
}

function getClientIp(req) {
  return req.socket.remoteAddress || 'unknown';
}

function requestLogger(req, res, next) {
  const start = Date.now();

  res.on('finish', () => {
    const durationMs = Date.now() - start;
    const targetService = req.targetService || '-';
    console.log(
      `${req.method} ${req.originalUrl} -> ${targetService} ${res.statusCode} ${durationMs}ms`
    );
  });

  next();
}

function healthHandler(_req, res) {
  res.json({ status: 'healthy', service: 'api-gateway' });
}

function authMiddleware(req, res, next) {
  if (!config.apiKey || req.path === '/health') {
    return next();
  }

  if (req.get('x-api-key') !== config.apiKey) {
    return jsonError(res, 401, 'unauthorized', 'Missing or invalid API key');
  }

  return next();
}

function rateLimitMiddleware(req, res, next) {
  if (req.path === '/health') {
    return next();
  }

  const now = Date.now();
  const key = getClientIp(req);
  const bucket = rateLimitBuckets.get(key);

  if (!bucket || now >= bucket.resetAt) {
    rateLimitBuckets.set(key, {
      count: 1,
      resetAt: now + config.rateLimitWindowMs,
    });
    return next();
  }

  bucket.count += 1;

  if (bucket.count > config.rateLimitMaxRequests) {
    res.set('Retry-After', Math.ceil((bucket.resetAt - now) / 1000).toString());
    return jsonError(res, 429, 'rate_limited', 'Too many requests');
  }

  return next();
}

function tagTargetService(service) {
  return (req, _res, next) => {
    req.targetService = service.name;
    next();
  };
}

function createServiceProxy(service) {
  return createProxyMiddleware({
    target: service.target,
    changeOrigin: true,
    proxyTimeout: config.requestTimeoutMs,
    timeout: config.requestTimeoutMs,
    on: {
      error: (err, req, res) => {
        console.error(
          `proxy error ${req.method} ${req.originalUrl} -> ${service.name}: ${err.message}`
        );

        if (!res.headersSent) {
          jsonError(res, 502, 'bad_gateway', `${service.name} is unavailable`);
        }
      },
    },
  });
}

const serviceProxies = config.services.map((service) => ({
  ...service,
  proxy: createServiceProxy(service),
}));

app.disable('x-powered-by');
app.use(requestLogger);
app.get('/health', healthHandler);
app.use(authMiddleware);
app.use(rateLimitMiddleware);

app.use((req, res, next) => {
  const matchedService = serviceProxies.find((service) =>
    isPrefixMatch(req.path, service.prefix)
  );

  if (!matchedService) {
    return next();
  }

  return tagTargetService(matchedService)(req, res, () =>
    matchedService.proxy(req, res, next)
  );
});

app.use((req, res) => {
  jsonError(res, 404, 'not_found', `No route for ${req.method} ${req.originalUrl}`);
});

app.use((err, req, res, _next) => {
  console.error(`gateway error ${req.method} ${req.originalUrl}: ${err.message}`);

  if (res.headersSent) {
    return;
  }

  jsonError(res, 500, 'internal_error', 'Unexpected gateway error');
});

app.listen(config.port, () => {
  console.log(`api-gateway listening on :${config.port}`);
  for (const service of config.services) {
    console.log(`${service.prefix} -> ${service.target}`);
  }
});
