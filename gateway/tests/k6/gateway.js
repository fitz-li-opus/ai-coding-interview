import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const VUS = Number(__ENV.K6_VUS || 20);
const DURATION = __ENV.K6_DURATION || "1m";

export const options = {
  scenarios: {
    steady: {
      executor: "constant-vus",
      vus: VUS,
      duration: DURATION,
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<300"],
  },
};

export default function () {
  const routes = ["/healthz", "/readyz", "/api/users/", "/api/orders/", "/api/products/"];

  for (const route of routes) {
    const res = http.get(`${BASE_URL}${route}`, {
      headers: {
        "X-Forwarded-For": "9.9.9.9",
      },
    });

    check(res, {
      [`${route} status`]: (r) => r.status === 200,
      [`${route} request-id`]: (r) => !!r.headers["X-Request-Id"],
    });
  }

  sleep(1);
}
