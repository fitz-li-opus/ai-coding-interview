// Configuration from environment variables
const PORT = parseInt(process.env.PORT || "8080");
const REQUEST_TIMEOUT = parseInt(process.env.REQUEST_TIMEOUT || "30000"); // 30 seconds
const RATE_LIMIT_WINDOW = parseInt(process.env.RATE_LIMIT_WINDOW || "60000"); // 1 minute
const RATE_LIMIT_MAX_REQUESTS = parseInt(process.env.RATE_LIMIT_MAX_REQUESTS || "100");
const CORS_ENABLED = process.env.CORS_ENABLED !== "false"; // Default true

// Route configuration mapping prefixes to backend services
const routes = [
  {
    prefix: "/api/users",
    targets: (process.env.USER_SERVICE_URLS || "http://localhost:8081").split(","),
    currentIndex: 0,
  },
  {
    prefix: "/api/orders",
    targets: (process.env.ORDER_SERVICE_URLS || "http://localhost:8082").split(","),
    currentIndex: 0,
  },
  {
    prefix: "/api/products",
    targets: (process.env.PRODUCT_SERVICE_URLS || "http://localhost:8083").split(","),
    currentIndex: 0,
  },
];

// Circuit breaker state for each service
const circuitBreakers = new Map<string, {
  failures: number;
  lastFailureTime: number;
  state: "closed" | "open" | "half-open";
}>();

const CIRCUIT_BREAKER_THRESHOLD = 5;
const CIRCUIT_BREAKER_TIMEOUT = 30000; // 30 seconds

// Rate limiting store
const rateLimitStore = new Map<string, { count: number; resetTime: number }>();

// Metrics
const metrics = {
  totalRequests: 0,
  successfulRequests: 0,
  failedRequests: 0,
  totalLatency: 0,
};

// Hop-by-hop headers that should not be forwarded
const HOP_BY_HOP_HEADERS = [
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailers",
  "transfer-encoding",
  "upgrade",
];

// Generate unique request ID
function generateRequestId(): string {
  return `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
}

// Get client IP address
function getClientIp(req: Request): string {
  const forwarded = req.headers.get("x-forwarded-for");
  if (forwarded) {
    return forwarded.split(",")[0].trim();
  }
  return "unknown";
}

// Rate limiting check
function checkRateLimit(clientIp: string): boolean {
  const now = Date.now();
  const record = rateLimitStore.get(clientIp);

  if (!record || now > record.resetTime) {
    rateLimitStore.set(clientIp, {
      count: 1,
      resetTime: now + RATE_LIMIT_WINDOW,
    });
    return true;
  }

  if (record.count >= RATE_LIMIT_MAX_REQUESTS) {
    return false;
  }

  record.count++;
  return true;
}

// Circuit breaker check
function checkCircuitBreaker(serviceUrl: string): boolean {
  const breaker = circuitBreakers.get(serviceUrl);
  if (!breaker) {
    circuitBreakers.set(serviceUrl, {
      failures: 0,
      lastFailureTime: 0,
      state: "closed",
    });
    return true;
  }

  const now = Date.now();

  if (breaker.state === "open") {
    if (now - breaker.lastFailureTime > CIRCUIT_BREAKER_TIMEOUT) {
      breaker.state = "half-open";
      return true;
    }
    return false;
  }

  return true;
}

// Record circuit breaker failure
function recordFailure(serviceUrl: string) {
  const breaker = circuitBreakers.get(serviceUrl);
  if (!breaker) return;

  breaker.failures++;
  breaker.lastFailureTime = Date.now();

  if (breaker.failures >= CIRCUIT_BREAKER_THRESHOLD) {
    breaker.state = "open";
    console.warn(`[Circuit Breaker] Service ${serviceUrl} is now OPEN`);
  }
}

// Record circuit breaker success
function recordSuccess(serviceUrl: string) {
  const breaker = circuitBreakers.get(serviceUrl);
  if (!breaker) return;

  if (breaker.state === "half-open") {
    breaker.state = "closed";
    breaker.failures = 0;
    console.log(`[Circuit Breaker] Service ${serviceUrl} is now CLOSED`);
  } else if (breaker.state === "closed") {
    breaker.failures = Math.max(0, breaker.failures - 1);
  }
}

// Load balancing: round-robin selection
function selectTarget(route: typeof routes[0]): string {
  const target = route.targets[route.currentIndex];
  route.currentIndex = (route.currentIndex + 1) % route.targets.length;
  return target;
}

// Add CORS headers
function addCorsHeaders(headers: Headers) {
  if (CORS_ENABLED) {
    headers.set("Access-Control-Allow-Origin", "*");
    headers.set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS");
    headers.set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID");
    headers.set("Access-Control-Max-Age", "86400");
  }
}

// Health check endpoint for the gateway itself
const healthCheck = () => {
  const headers = new Headers({ "Content-Type": "application/json" });
  addCorsHeaders(headers);

  return new Response(
    JSON.stringify({
      status: "healthy",
      service: "api-gateway",
      timestamp: new Date().toISOString(),
      metrics: {
        totalRequests: metrics.totalRequests,
        successfulRequests: metrics.successfulRequests,
        failedRequests: metrics.failedRequests,
        averageLatency: metrics.totalRequests > 0
          ? Math.round(metrics.totalLatency / metrics.totalRequests)
          : 0,
      },
      circuitBreakers: Array.from(circuitBreakers.entries()).map(([url, breaker]) => ({
        service: url,
        state: breaker.state,
        failures: breaker.failures,
      })),
    }),
    { headers }
  );
};

// Forward request to the appropriate backend service
async function proxyRequest(req: Request, targetUrl: string, path: string, requestId: string) {
  const startTime = Date.now();

  try {
    // Check circuit breaker
    if (!checkCircuitBreaker(targetUrl)) {
      throw new Error(`Circuit breaker is OPEN for ${targetUrl}`);
    }

    const url = new URL(req.url);
    const backendUrl = `${targetUrl}${path}${url.search}`;

    console.log(`[${new Date().toISOString()}] [${requestId}] ${req.method} ${url.pathname} -> ${backendUrl}`);

    // Filter headers - remove hop-by-hop headers and Host
    const forwardHeaders = new Headers();
    req.headers.forEach((value, key) => {
      const lowerKey = key.toLowerCase();
      if (!HOP_BY_HOP_HEADERS.includes(lowerKey) && lowerKey !== "host") {
        forwardHeaders.set(key, value);
      }
    });

    // Add request ID for tracing
    forwardHeaders.set("X-Request-ID", requestId);
    forwardHeaders.set("X-Forwarded-For", getClientIp(req));
    forwardHeaders.set("X-Forwarded-Proto", url.protocol.replace(":", ""));
    forwardHeaders.set("X-Forwarded-Host", url.host);

    // Create abort controller for timeout
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT);

    try {
      // Forward the request to the backend service
      const response = await fetch(backendUrl, {
        method: req.method,
        headers: forwardHeaders,
        body: req.method !== "GET" && req.method !== "HEAD" ? req.body : undefined,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      // Record success
      recordSuccess(targetUrl);
      metrics.successfulRequests++;

      // Create response headers and add CORS
      const responseHeaders = new Headers(response.headers);
      addCorsHeaders(responseHeaders);
      responseHeaders.set("X-Request-ID", requestId);

      // Record latency
      const latency = Date.now() - startTime;
      metrics.totalLatency += latency;
      responseHeaders.set("X-Response-Time", `${latency}ms`);

      // Return the backend response
      return new Response(response.body, {
        status: response.status,
        statusText: response.statusText,
        headers: responseHeaders,
      });
    } catch (fetchError) {
      clearTimeout(timeoutId);

      if (fetchError instanceof Error && fetchError.name === "AbortError") {
        throw new Error(`Request timeout after ${REQUEST_TIMEOUT}ms`);
      }
      throw fetchError;
    }
  } catch (error) {
    // Record failure
    recordFailure(targetUrl);
    metrics.failedRequests++;

    const latency = Date.now() - startTime;
    metrics.totalLatency += latency;

    console.error(`[${requestId}] Error proxying request: ${error}`);

    const headers = new Headers({ "Content-Type": "application/json" });
    addCorsHeaders(headers);
    headers.set("X-Request-ID", requestId);

    return new Response(
      JSON.stringify({
        error: "Service unavailable",
        message: error instanceof Error ? error.message : "Unknown error",
        requestId,
      }),
      {
        status: 503,
        headers,
      }
    );
  }
}

// Main request handler
const server = Bun.serve({
  port: PORT,
  async fetch(req) {
    const requestId = generateRequestId();
    const clientIp = getClientIp(req);
    const url = new URL(req.url);
    const path = url.pathname;

    metrics.totalRequests++;

    // Handle CORS preflight requests
    if (req.method === "OPTIONS") {
      const headers = new Headers();
      addCorsHeaders(headers);
      return new Response(null, { status: 204, headers });
    }

    // Gateway health check
    if (path === "/health") {
      return healthCheck();
    }

    // Rate limiting check
    if (!checkRateLimit(clientIp)) {
      const headers = new Headers({ "Content-Type": "application/json" });
      addCorsHeaders(headers);
      headers.set("X-Request-ID", requestId);
      headers.set("X-RateLimit-Limit", RATE_LIMIT_MAX_REQUESTS.toString());
      headers.set("X-RateLimit-Remaining", "0");

      return new Response(
        JSON.stringify({
          error: "Too Many Requests",
          message: `Rate limit exceeded. Maximum ${RATE_LIMIT_MAX_REQUESTS} requests per ${RATE_LIMIT_WINDOW / 1000} seconds.`,
          requestId,
        }),
        {
          status: 429,
          headers,
        }
      );
    }

    // Route matching and proxying
    for (const route of routes) {
      if (path.startsWith(route.prefix)) {
        // Remove the prefix and forward the rest of the path
        const targetPath = path.slice(route.prefix.length) || "/";
        const targetUrl = selectTarget(route);
        return proxyRequest(req, targetUrl, targetPath, requestId);
      }
    }

    // No matching route found
    const headers = new Headers({ "Content-Type": "application/json" });
    addCorsHeaders(headers);
    headers.set("X-Request-ID", requestId);

    return new Response(
      JSON.stringify({
        error: "Not Found",
        message: `No route configured for path: ${path}`,
        availableRoutes: routes.map((r) => r.prefix),
        requestId,
      }),
      {
        status: 404,
        headers,
      }
    );
  },
});

console.log(`🚀 API Gateway running on http://localhost:${server.port}`);
console.log(`\nConfiguration:`);
console.log(`  Request Timeout: ${REQUEST_TIMEOUT}ms`);
console.log(`  Rate Limit: ${RATE_LIMIT_MAX_REQUESTS} requests per ${RATE_LIMIT_WINDOW / 1000}s`);
console.log(`  CORS Enabled: ${CORS_ENABLED}`);
console.log(`  Circuit Breaker Threshold: ${CIRCUIT_BREAKER_THRESHOLD} failures`);
console.log(`\nConfigured routes:`);
routes.forEach((route) => {
  console.log(`  ${route.prefix} -> ${route.targets.join(", ")}`);
});
console.log(`\nHealth check: http://localhost:${server.port}/health\n`);
