import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import http from 'node:http';
import net from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';

const gatewayEntrypoint = fileURLToPath(new URL('../src/server.js', import.meta.url));

const tests = [];

function test(name, fn) {
  tests.push({ name, fn });
}

async function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.on('error', reject);
    server.listen(0, () => {
      const { port } = server.address();
      server.close(() => resolve(port));
    });
  });
}

async function waitForGateway(baseUrl, child) {
  const deadline = Date.now() + 5000;

  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`gateway exited early with code ${child.exitCode}`);
    }

    try {
      const response = await fetch(`${baseUrl}/health`);
      if (response.ok) {
        return;
      }
    } catch {
      // Retry until the process is listening.
    }

    await delay(100);
  }

  throw new Error('gateway did not become healthy in time');
}

async function startGateway(env = {}) {
  const port = await getFreePort();
  const child = spawn(process.execPath, [gatewayEntrypoint], {
    env: {
      ...process.env,
      PORT: String(port),
      ...env,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  const logs = [];
  child.stdout.on('data', (chunk) => logs.push(chunk.toString()));
  child.stderr.on('data', (chunk) => logs.push(chunk.toString()));

  const baseUrl = `http://localhost:${port}`;
  await waitForGateway(baseUrl, child);

  return {
    baseUrl,
    logs,
    async stop() {
      if (child.exitCode !== null) {
        return;
      }

      child.kill('SIGTERM');
      await new Promise((resolve) => child.once('exit', resolve));
    },
  };
}

async function startMockBackend(service, data) {
  const port = await getFreePort();
  const server = http.createServer((req, res) => {
    res.setHeader('Content-Type', 'application/json');

    if (req.url === '/health') {
      res.end(JSON.stringify({ status: 'healthy', service }));
      return;
    }

    res.end(
      JSON.stringify({
        service,
        path: new URL(req.url, `http://localhost:${port}`).pathname,
        method: req.method,
        timestamp: new Date().toISOString(),
        data,
      })
    );
  });

  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, resolve);
  });

  return {
    url: `http://localhost:${port}`,
    close() {
      return new Promise((resolve) => server.close(resolve));
    },
  };
}

async function startBackends() {
  const userService = await startMockBackend('user-service', [
    { id: 1, name: 'Alice' },
  ]);
  const orderService = await startMockBackend('order-service', [
    { id: 1001, product: 'Laptop' },
  ]);
  const productService = await startMockBackend('product-service', [
    { id: 1, name: 'Laptop' },
  ]);

  return {
    env: {
      USER_SERVICE_URL: userService.url,
      ORDER_SERVICE_URL: orderService.url,
      PRODUCT_SERVICE_URL: productService.url,
    },
    async close() {
      await Promise.all([
        userService.close(),
        orderService.close(),
        productService.close(),
      ]);
    },
  };
}

async function withGateway(env, fn) {
  const backends = await startBackends();
  const gateway = await startGateway({ ...backends.env, ...env });

  try {
    await fn(gateway);
  } finally {
    await gateway.stop();
    await backends.close();
  }
}

async function requestJson(url, options) {
  const response = await fetch(url, options);
  const text = await response.text();
  let body = null;

  if (text) {
    body = JSON.parse(text);
  }

  return { response, body };
}

test('health endpoint returns gateway status', async () => {
  await withGateway({}, async (gateway) => {
    const { response, body } = await requestJson(`${gateway.baseUrl}/health`);

    assert.equal(response.status, 200);
    assert.deepEqual(body, { status: 'healthy', service: 'api-gateway' });
  });
});

test('routes users, orders, and products to the correct services', async () => {
  await withGateway({}, async (gateway) => {
    const cases = [
      ['/api/users', 'user-service'],
      ['/api/orders', 'order-service'],
      ['/api/products', 'product-service'],
    ];

    for (const [path, service] of cases) {
      const { response, body } = await requestJson(`${gateway.baseUrl}${path}`);

      assert.equal(response.status, 200);
      assert.equal(body.service, service);
      assert.equal(body.path, path);
      assert.equal(body.method, 'GET');
    }
  });
});

test('forwards nested paths without stripping the api prefix', async () => {
  await withGateway({}, async (gateway) => {
    const { response, body } = await requestJson(
      `${gateway.baseUrl}/api/users/123?demo=1`
    );

    assert.equal(response.status, 200);
    assert.equal(body.service, 'user-service');
    assert.equal(body.path, '/api/users/123');
  });
});

test('returns json 404 for unsupported routes and similar prefixes', async () => {
  await withGateway({}, async (gateway) => {
    for (const path of ['/api/unknown', '/api/usersx']) {
      const { response, body } = await requestJson(`${gateway.baseUrl}${path}`);

      assert.equal(response.status, 404);
      assert.equal(body.error.code, 'not_found');
    }
  });
});

test('returns json 502 when a matched backend is unavailable', async () => {
  await withGateway({ USER_SERVICE_URL: 'http://localhost:9' }, async (gateway) => {
    const { response, body } = await requestJson(`${gateway.baseUrl}/api/users`);

    assert.equal(response.status, 502);
    assert.equal(body.error.code, 'bad_gateway');
  });
});

test('rejects missing api key when API_KEY is configured', async () => {
  await withGateway({ API_KEY: 'secret' }, async (gateway) => {
    const missing = await requestJson(`${gateway.baseUrl}/api/users`);
    const valid = await requestJson(`${gateway.baseUrl}/api/users`, {
      headers: { 'x-api-key': 'secret' },
    });

    assert.equal(missing.response.status, 401);
    assert.equal(missing.body.error.code, 'unauthorized');
    assert.equal(valid.response.status, 200);
    assert.equal(valid.body.service, 'user-service');
  });
});

test('applies fixed-window rate limiting', async () => {
  await withGateway({
    RATE_LIMIT_MAX_REQUESTS: '2',
    RATE_LIMIT_WINDOW_MS: '60000',
  }, async (gateway) => {
    const first = await requestJson(`${gateway.baseUrl}/api/users`);
    const second = await requestJson(`${gateway.baseUrl}/api/users`);
    const third = await requestJson(`${gateway.baseUrl}/api/users`);

    assert.equal(first.response.status, 200);
    assert.equal(second.response.status, 200);
    assert.equal(third.response.status, 429);
    assert.equal(third.body.error.code, 'rate_limited');
  });
});

test('writes routing logs with target service and status code', async () => {
  await withGateway({}, async (gateway) => {
    const { response } = await requestJson(`${gateway.baseUrl}/api/users`);
    assert.equal(response.status, 200);

    await delay(100);
    const output = gateway.logs.join('');
    assert.match(output, /GET \/api\/users -> user-service 200 \d+ms/);
  });
});

let failed = 0;

for (const { name, fn } of tests) {
  try {
    await fn();
    console.log(`ok - ${name}`);
  } catch (error) {
    failed += 1;
    console.error(`not ok - ${name}`);
    console.error(error);
  }
}

if (failed > 0) {
  console.error(`${failed}/${tests.length} tests failed`);
  process.exit(1);
}

console.log(`${tests.length}/${tests.length} tests passed`);
