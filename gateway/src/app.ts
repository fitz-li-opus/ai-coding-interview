import { Elysia } from "elysia";
import type { GatewayConfig } from "./config.ts";
import type { Fetcher } from "./proxy.ts";
import { proxyRequest } from "./proxy.ts";
import { createRateLimiter } from "./rate-limit.ts";

export const createApp = (config: GatewayConfig, fetcher?: Fetcher) => {
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
      const rateLimitResponse = rateLimit(request);
      return (
        rateLimitResponse ??
        proxyRequest(request, {
          fetcher,
          requestTimeoutMs: config.requestTimeoutMs,
          services: config.services,
        })
      );
    });
};

export type App = ReturnType<typeof createApp>;
