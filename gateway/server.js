"use strict";

const { createGateway } = require("./gateway");

const config = {
  globalMaxInFlight: 1000,
  routes: [
    {
      prefix: "/api/users",
      targets: [
        process.env.USER_SERVICE_URL || "http://127.0.0.1:8081",
      ],
      timeoutMs: 300,
      maxInFlight: 200,
      maxSockets: 50,
      retry: { maxAttempts: 2, backoffMs: 25 },
      breaker: {
        windowMs: 10_000,
        minRequests: 20,
        failureRatio: 0.5,
        openMs: 5_000,
        halfOpenProbes: 2,
      },
    },
    {
      prefix: "/api/orders",
      targets: [
        process.env.ORDER_SERVICE_URL || "http://127.0.0.1:8082",
      ],
      timeoutMs: 500,
      maxInFlight: 200,
      maxSockets: 50,
      retry: { maxAttempts: 2, backoffMs: 25 },
      breaker: {
        windowMs: 10_000,
        minRequests: 20,
        failureRatio: 0.5,
        openMs: 5_000,
        halfOpenProbes: 2,
      },
    },
    {
      prefix: "/api/products",
      targets: [
        process.env.PRODUCT_SERVICE_URL || "http://127.0.0.1:8083",
      ],
      timeoutMs: 300,
      maxInFlight: 200,
      maxSockets: 50,
      retry: { maxAttempts: 2, backoffMs: 25 },
      breaker: {
        windowMs: 10_000,
        minRequests: 20,
        failureRatio: 0.5,
        openMs: 5_000,
        halfOpenProbes: 2,
      },
    },
  ],
};

const port = Number(process.env.PORT || 8080);
const server = createGateway(config);

server.listen(port, "0.0.0.0", () => {
  console.log(
    JSON.stringify({
      level: "info",
      event: "gateway_started",
      port,
    }),
  );
});

let shuttingDown = false;

function shutdown(signal) {
  if (shuttingDown) {
    return;
  }
  shuttingDown = true;

  console.log(
    JSON.stringify({
      level: "info",
      event: "gateway_shutdown",
      signal,
    }),
  );

  const forceExit = setTimeout(() => process.exit(1), 10_000);
  forceExit.unref();

  server.close((error) => {
    clearTimeout(forceExit);
    process.exit(error ? 1 : 0);
  });
  server.closeIdleConnections();
}

process.on("SIGTERM", () => shutdown("SIGTERM"));
process.on("SIGINT", () => shutdown("SIGINT"));
