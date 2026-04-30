import type { GatewayConfig } from "./config.ts";
import { findRoute } from "./proxy.ts";

type RateLimitState = {
  count: number;
  resetAt: number;
};

const getClientIp = (request: Request) =>
  request.headers.get("x-forwarded-for")?.split(",")[0]?.trim() ||
  request.headers.get("x-real-ip") ||
  "unknown";

const createRateLimitResponse = (retryAfterSeconds: number) =>
  Response.json(
    { error: "rate_limited", message: "Rate limit exceeded" },
    {
      headers: {
        "retry-after": String(retryAfterSeconds),
      },
      status: 429,
    },
  );

export const createRateLimiter = (config: GatewayConfig) => {
  const buckets = new Map<string, RateLimitState>();

  return (request: Request) => {
    if (!config.rateLimit.enabled) return undefined;

    const route = findRoute(new URL(request.url).pathname, config.services);
    if (!route) return undefined;

    const now = Date.now();
    const key = `${getClientIp(request)}:${route.prefix}`;
    const current = buckets.get(key);

    if (!current || current.resetAt <= now) {
      buckets.set(key, {
        count: 1,
        resetAt: now + config.rateLimit.windowMs,
      });
      return undefined;
    }

    if (current.count >= config.rateLimit.maxRequests) {
      return createRateLimitResponse(Math.ceil((current.resetAt - now) / 1_000));
    }

    current.count += 1;
    return undefined;
  };
};
