import { Elysia } from "elysia";
import type { GatewayConfig } from "./config.ts";
import type { Fetcher } from "./proxy.ts";
import { proxyRequest } from "./proxy.ts";

export const createApp = (config: GatewayConfig, fetcher?: Fetcher) =>
  new Elysia()
    .get("/health", () => ({
      service: "api-gateway",
      status: "healthy",
      upstreams: config.services.map(({ name, prefix, upstreamUrl }) => ({
        name,
        prefix,
        upstreamUrl,
      })),
    }))
    .all("/*", ({ request }) =>
      proxyRequest(request, {
        fetcher,
        requestTimeoutMs: config.requestTimeoutMs,
        services: config.services,
      }),
    );

export type App = ReturnType<typeof createApp>;
