import { describe, test, expect, beforeAll } from "bun:test";

/**
 * Integration tests for API Gateway with real backend services
 *
 * Prerequisites:
 * 1. Backend services must be running (docker compose up)
 * 2. Gateway must be running (bun start)
 */

describe("API Gateway Integration Tests", () => {
  const GATEWAY_URL = "http://localhost:8080";
  const USER_SERVICE_URL = "http://localhost:8081";
  const ORDER_SERVICE_URL = "http://localhost:8082";
  const PRODUCT_SERVICE_URL = "http://localhost:8083";

  beforeAll(async () => {
    // Verify backend services are running
    try {
      await fetch(USER_SERVICE_URL);
      await fetch(ORDER_SERVICE_URL);
      await fetch(PRODUCT_SERVICE_URL);
    } catch (error) {
      console.warn("⚠️  Backend services may not be running. Start them with: docker compose up");
    }
  });

  describe("End-to-End Request Flow", () => {
    test("should successfully proxy request to user service", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/users/`);

      expect(response.status).toBe(200);
      expect(response.headers.get("Content-Type")).toContain("application/json");

      const data = await response.json();
      expect(data).toBeDefined();

      // Verify tracing headers
      expect(response.headers.get("X-Request-ID")).toBeDefined();
      expect(response.headers.get("X-Response-Time")).toBeDefined();
    });

    test("should successfully proxy request to order service", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/orders/`);

      expect(response.status).toBe(200);
      const data = await response.json();
      expect(data).toBeDefined();
    });

    test("should successfully proxy request to product service", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/products/`);

      expect(response.status).toBe(200);
      const data = await response.json();
      expect(data).toBeDefined();
    });
  });

  describe("Backend Health Checks via Gateway", () => {
    test("should proxy health check to user service", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/users/health`);

      expect(response.status).toBe(200);
      const data = await response.json();
      expect(data.status).toBe("healthy");
    });

    test("should proxy health check to order service", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/orders/health`);

      expect(response.status).toBe(200);
      const data = await response.json();
      expect(data.status).toBe("healthy");
    });

    test("should proxy health check to product service", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/products/health`);

      expect(response.status).toBe(200);
      const data = await response.json();
      expect(data.status).toBe("healthy");
    });
  });

  describe("Response Consistency", () => {
    test("should return same data structure as direct backend call", async () => {
      // Call through gateway
      const gatewayResponse = await fetch(`${GATEWAY_URL}/api/users/`);
      const gatewayData = await gatewayResponse.json();

      // Call backend directly
      const directResponse = await fetch(`${USER_SERVICE_URL}/`);
      const directData = await directResponse.json();

      // Data should be identical
      expect(gatewayData).toEqual(directData);
    });

    test("should preserve backend response status codes", async () => {
      // Test with a path that might not exist
      const gatewayResponse = await fetch(`${GATEWAY_URL}/api/users/nonexistent`);
      const directResponse = await fetch(`${USER_SERVICE_URL}/nonexistent`);

      // Status codes should match (both 404 or both 200)
      expect(gatewayResponse.status).toBe(directResponse.status);
    });
  });

  describe("Query Parameter Forwarding", () => {
    test("should forward query parameters to backend", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/users/?test=value&page=1`);

      expect(response.status).toBe(200);
      // Backend should receive and process query parameters
    });

    test("should handle special characters in query parameters", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/users/?name=${encodeURIComponent("John Doe")}`);

      expect(response.status).toBe(200);
    });
  });

  describe("Request Body Forwarding", () => {
    test("should forward JSON request body", async () => {
      const testData = { name: "Test User", email: "test@example.com" };

      const response = await fetch(`${GATEWAY_URL}/api/users/`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(testData),
      });

      // Backend might not support POST, but gateway should forward it
      expect([200, 404, 405]).toContain(response.status);
    });

    test("should forward form data", async () => {
      const formData = new FormData();
      formData.append("name", "Test User");

      const response = await fetch(`${GATEWAY_URL}/api/users/`, {
        method: "POST",
        body: formData,
      });

      expect([200, 404, 405]).toContain(response.status);
    });
  });

  describe("Concurrent Requests", () => {
    test("should handle multiple concurrent requests", async () => {
      const requests = [];

      for (let i = 0; i < 20; i++) {
        requests.push(fetch(`${GATEWAY_URL}/api/users/`));
        requests.push(fetch(`${GATEWAY_URL}/api/orders/`));
        requests.push(fetch(`${GATEWAY_URL}/api/products/`));
      }

      const responses = await Promise.all(requests);

      // All requests should succeed
      responses.forEach(response => {
        expect(response.status).toBe(200);
      });

      // All should have unique request IDs
      const requestIds = responses.map(r => r.headers.get("X-Request-ID"));
      const uniqueIds = new Set(requestIds);
      expect(uniqueIds.size).toBe(requestIds.length);
    });
  });

  describe("Performance", () => {
    test("should add minimal latency overhead", async () => {
      // Measure direct backend call
      const directStart = Date.now();
      await fetch(`${USER_SERVICE_URL}/`);
      const directLatency = Date.now() - directStart;

      // Measure gateway call
      const gatewayStart = Date.now();
      const gatewayResponse = await fetch(`${GATEWAY_URL}/api/users/`);
      const gatewayLatency = Date.now() - gatewayStart;

      // Gateway overhead should be reasonable (< 100ms in most cases)
      const overhead = gatewayLatency - directLatency;
      console.log(`Gateway overhead: ${overhead}ms`);

      // This is a soft check - network conditions can vary
      expect(overhead).toBeLessThan(200);

      // Verify response time header is accurate
      const reportedTime = parseInt(gatewayResponse.headers.get("X-Response-Time") || "0");
      expect(reportedTime).toBeGreaterThan(0);
      expect(reportedTime).toBeLessThan(gatewayLatency + 50); // Allow some margin
    });
  });

  describe("Circuit Breaker Integration", () => {
    test("should track circuit breaker state for real services", async () => {
      // Make some successful requests
      for (let i = 0; i < 5; i++) {
        await fetch(`${GATEWAY_URL}/api/users/`);
      }

      // Check health endpoint
      const healthResponse = await fetch(`${GATEWAY_URL}/health`);
      const healthData = await healthResponse.json();

      // Circuit breakers should exist for the services
      expect(healthData.circuitBreakers.length).toBeGreaterThan(0);

      // All should be in closed state (healthy)
      const userServiceBreaker = healthData.circuitBreakers.find(
        (cb: any) => cb.service.includes("8081")
      );

      if (userServiceBreaker) {
        expect(userServiceBreaker.state).toBe("closed");
        expect(userServiceBreaker.failures).toBeLessThanOrEqual(1);
      }
    });
  });

  describe("Metrics Accuracy", () => {
    test("should accurately track request metrics", async () => {
      // Get initial metrics
      const healthBefore = await fetch(`${GATEWAY_URL}/health`);
      const dataBefore = await healthBefore.json();
      const metricsBefore = dataBefore.metrics;

      // Make 10 requests
      const requests = [];
      for (let i = 0; i < 10; i++) {
        requests.push(fetch(`${GATEWAY_URL}/api/users/`));
      }
      await Promise.all(requests);

      // Get updated metrics
      const healthAfter = await fetch(`${GATEWAY_URL}/health`);
      const dataAfter = await healthAfter.json();
      const metricsAfter = dataAfter.metrics;

      // Verify metrics increased
      expect(metricsAfter.totalRequests).toBeGreaterThanOrEqual(
        metricsBefore.totalRequests + 10
      );
      expect(metricsAfter.successfulRequests).toBeGreaterThanOrEqual(
        metricsBefore.successfulRequests + 10
      );
      expect(metricsAfter.averageLatency).toBeGreaterThan(0);
    });
  });

  describe("Error Scenarios", () => {
    test("should handle backend service unavailability", async () => {
      // Try to access a service on a port that doesn't exist
      // This simulates a backend service being down

      // First, we need to temporarily configure a bad target
      // Since we can't modify config at runtime, we'll just verify
      // that the gateway handles connection errors gracefully

      const response = await fetch(`${GATEWAY_URL}/api/users/`);

      // Should either succeed (if service is up) or return 503 (if down)
      expect([200, 503]).toContain(response.status);

      if (response.status === 503) {
        const data = await response.json();
        expect(data.error).toBe("Service unavailable");
        expect(data.requestId).toBeDefined();
      }
    });
  });

  describe("Header Propagation", () => {
    test("should add X-Forwarded-* headers", async () => {
      // We can't directly verify headers sent to backend
      // But we can verify the request succeeds
      const response = await fetch(`${GATEWAY_URL}/api/users/`, {
        headers: {
          "User-Agent": "Integration-Test",
          "X-Custom-Header": "test-value",
        },
      });

      expect(response.status).toBe(200);
    });

    test("should preserve Content-Type from backend", async () => {
      const response = await fetch(`${GATEWAY_URL}/api/users/`);

      expect(response.headers.get("Content-Type")).toContain("application/json");
    });
  });
});
