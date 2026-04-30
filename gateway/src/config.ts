export type GatewayConfig = {
  port: number;
  requestTimeoutMs: number;
  services: ServiceRoute[];
};

export type ServiceRoute = {
  name: string;
  prefix: string;
  upstreamUrl: string;
};

const DEFAULT_PORT = 8080;
const DEFAULT_TIMEOUT_MS = 5_000;

const parsePositiveInteger = (value: string | undefined, fallback: number) => {
  if (!value) return fallback;

  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
};

export const loadConfig = (env: NodeJS.ProcessEnv = process.env): GatewayConfig => ({
  port: parsePositiveInteger(env.PORT, DEFAULT_PORT),
  requestTimeoutMs: parsePositiveInteger(env.REQUEST_TIMEOUT_MS, DEFAULT_TIMEOUT_MS),
  services: [
    {
      name: "user-service",
      prefix: "/api/users",
      upstreamUrl: env.USER_SERVICE_URL ?? "http://localhost:8081",
    },
    {
      name: "order-service",
      prefix: "/api/orders",
      upstreamUrl: env.ORDER_SERVICE_URL ?? "http://localhost:8082",
    },
    {
      name: "product-service",
      prefix: "/api/products",
      upstreamUrl: env.PRODUCT_SERVICE_URL ?? "http://localhost:8083",
    },
  ],
});
