import { Elysia } from "elysia";
import type { Logger } from "./access-log.ts";
import { createAccessLogEntry } from "./access-log.ts";
import type { GatewayConfig } from "./config.ts";
import type { Fetcher } from "./proxy.ts";
import { proxyRequest } from "./proxy.ts";
import { createRateLimiter } from "./rate-limit.ts";

export const createApp = (config: GatewayConfig, fetcher?: Fetcher, logger?: Logger) => {
  const rateLimit = createRateLimiter(config);

  return new Elysia()
    .get("/health", () => ({
      service: "api-gateway",
      status: "healthy",
      upstreams: config.services.map(({ name, prefix, upstreamUrl }) => ({
        name,
        prefix,
        upstreamUrl,
      })),
    }))
    .all("/*", async ({ request }) => {
      const startedAt = performance.now();
      const rateLimitResponse = rateLimit(request);
      const response =
        rateLimitResponse ??
        (await proxyRequest(request, {
          fetcher,
          requestTimeoutMs: config.requestTimeoutMs,
          services: config.services,
        }));

      logger?.(createAccessLogEntry(request, response, startedAt));

      return response;
    });
};

export type App = ReturnType<typeof createApp>;
