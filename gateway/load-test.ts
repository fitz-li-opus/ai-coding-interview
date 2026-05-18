/**
 * Simple load testing script for the API Gateway
 *
 * Usage:
 *   bun run load-test.ts [requests] [concurrency]
 *
 * Example:
 *   bun run load-test.ts 1000 50
 */

const GATEWAY_URL = process.env.GATEWAY_URL || "http://localhost:8080";
const TOTAL_REQUESTS = parseInt(process.argv[2] || "1000");
const CONCURRENCY = parseInt(process.argv[3] || "10");

interface TestResult {
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  rateLimitedRequests: number;
  totalDuration: number;
  averageLatency: number;
  minLatency: number;
  maxLatency: number;
  requestsPerSecond: number;
  statusCodes: Record<number, number>;
}

async function makeRequest(url: string): Promise<{
  status: number;
  latency: number;
  success: boolean;
}> {
  const start = Date.now();
  try {
    const response = await fetch(url);
    const latency = Date.now() - start;
    return {
      status: response.status,
      latency,
      success: response.status === 200,
    };
  } catch (error) {
    const latency = Date.now() - start;
    return {
      status: 0,
      latency,
      success: false,
    };
  }
}

async function runLoadTest(): Promise<TestResult> {
  console.log(`\n🚀 Starting load test...`);
  console.log(`   Target: ${GATEWAY_URL}`);
  console.log(`   Total Requests: ${TOTAL_REQUESTS}`);
  console.log(`   Concurrency: ${CONCURRENCY}\n`);

  const endpoints = [
    `${GATEWAY_URL}/api/users/`,
    `${GATEWAY_URL}/api/orders/`,
    `${GATEWAY_URL}/api/products/`,
  ];

  const results: Array<{ status: number; latency: number; success: boolean }> = [];
  const statusCodes: Record<number, number> = {};

  const startTime = Date.now();
  let completed = 0;

  // Create batches of concurrent requests
  for (let i = 0; i < TOTAL_REQUESTS; i += CONCURRENCY) {
    const batchSize = Math.min(CONCURRENCY, TOTAL_REQUESTS - i);
    const batch = [];

    for (let j = 0; j < batchSize; j++) {
      const endpoint = endpoints[(i + j) % endpoints.length];
      batch.push(makeRequest(endpoint));
    }

    const batchResults = await Promise.all(batch);
    results.push(...batchResults);

    completed += batchSize;
    process.stdout.write(`\r   Progress: ${completed}/${TOTAL_REQUESTS} requests`);

    // Count status codes
    batchResults.forEach(result => {
      statusCodes[result.status] = (statusCodes[result.status] || 0) + 1;
    });
  }

  const totalDuration = Date.now() - startTime;

  console.log(`\n\n✅ Load test completed!\n`);

  // Calculate statistics
  const successfulRequests = results.filter(r => r.success).length;
  const failedRequests = results.filter(r => !r.success && r.status !== 429).length;
  const rateLimitedRequests = results.filter(r => r.status === 429).length;
  const latencies = results.map(r => r.latency);
  const averageLatency = latencies.reduce((a, b) => a + b, 0) / latencies.length;
  const minLatency = Math.min(...latencies);
  const maxLatency = Math.max(...latencies);
  const requestsPerSecond = (TOTAL_REQUESTS / totalDuration) * 1000;

  return {
    totalRequests: TOTAL_REQUESTS,
    successfulRequests,
    failedRequests,
    rateLimitedRequests,
    totalDuration,
    averageLatency,
    minLatency,
    maxLatency,
    requestsPerSecond,
    statusCodes,
  };
}

function printResults(result: TestResult) {
  console.log(`📊 Results:`);
  console.log(`   Total Requests:        ${result.totalRequests}`);
  console.log(`   Successful:            ${result.successfulRequests} (${((result.successfulRequests / result.totalRequests) * 100).toFixed(2)}%)`);
  console.log(`   Failed:                ${result.failedRequests} (${((result.failedRequests / result.totalRequests) * 100).toFixed(2)}%)`);
  console.log(`   Rate Limited:          ${result.rateLimitedRequests} (${((result.rateLimitedRequests / result.totalRequests) * 100).toFixed(2)}%)`);
  console.log(`\n⏱️  Performance:`);
  console.log(`   Total Duration:        ${result.totalDuration}ms`);
  console.log(`   Average Latency:       ${result.averageLatency.toFixed(2)}ms`);
  console.log(`   Min Latency:           ${result.minLatency}ms`);
  console.log(`   Max Latency:           ${result.maxLatency}ms`);
  console.log(`   Requests/Second:       ${result.requestsPerSecond.toFixed(2)}`);
  console.log(`\n📈 Status Codes:`);
  Object.entries(result.statusCodes)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .forEach(([code, count]) => {
      const percentage = ((count / result.totalRequests) * 100).toFixed(2);
      console.log(`   ${code}: ${count} (${percentage}%)`);
    });
  console.log();
}

async function checkGatewayHealth(): Promise<boolean> {
  try {
    const response = await fetch(`${GATEWAY_URL}/health`);
    if (response.status === 200) {
      const data = await response.json();
      console.log(`✅ Gateway is healthy`);
      console.log(`   Current metrics:`);
      console.log(`   - Total Requests: ${data.metrics.totalRequests}`);
      console.log(`   - Success Rate: ${((data.metrics.successfulRequests / data.metrics.totalRequests) * 100).toFixed(2)}%`);
      console.log(`   - Average Latency: ${data.metrics.averageLatency}ms`);
      return true;
    }
    return false;
  } catch (error) {
    console.error(`❌ Gateway is not reachable at ${GATEWAY_URL}`);
    console.error(`   Make sure the gateway is running: bun start`);
    return false;
  }
}

// Main execution
(async () => {
  console.log(`\n${"=".repeat(60)}`);
  console.log(`  API Gateway Load Test`);
  console.log(`${"=".repeat(60)}`);

  // Check if gateway is running
  const isHealthy = await checkGatewayHealth();
  if (!isHealthy) {
    process.exit(1);
  }

  // Run the load test
  const result = await runLoadTest();

  // Print results
  printResults(result);

  // Check gateway health after test
  console.log(`\n${"=".repeat(60)}`);
  console.log(`  Gateway Health After Test`);
  console.log(`${"=".repeat(60)}\n`);
  await checkGatewayHealth();

  console.log(`\n${"=".repeat(60)}\n`);
})();
