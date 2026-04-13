'use strict';

const http = require('http');
const config = require('./config');
const router = require('./router');
const proxy = require('./proxy');
const admin = require('./admin');
const chain = require('./chain');
const plugins = require('./plugins');

function defaultPlugins() {
  // Global plugin chain (for future rateLimit / ipBlock / commonExploit).
  // Per-route basic auth is configured on each route entry, not here.
  return [];
}

function createHandler(pluginList) {
  return async function handle(req, res) {
    try {
      const url = new URL(req.url, 'http://internal');
      const pathname = url.pathname;

      if (pathname === '/health' && req.method === 'GET') {
        res.statusCode = 200;
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({ status: 'ok' }));
        return;
      }

      if (pathname === '/admin' && req.method === 'GET') {
        admin.serveAdminPage(req, res);
        return;
      }

      if (pathname === '/admin/api/routes') {
        if (req.method === 'GET') return admin.getRoutes(req, res);
        if (req.method === 'PUT') return admin.putRoutes(req, res);
        res.statusCode = 405;
        res.setHeader('Allow', 'GET, PUT');
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({ error: 'method not allowed' }));
        return;
      }

      const route = router.match(pathname, config.getRoutes());
      if (!route) {
        res.statusCode = 404;
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({ error: 'no route matched' }));
        return;
      }

      if (route.auth) {
        const check = plugins.basicAuth(route.auth);
        const ok = await check(req, res);
        if (ok === false) return;
      }

      const status = await chain.run(req, res, { route }, pluginList);
      if (status === 'halted') return;

      proxy.forward(req, res, route.target);
    } catch (err) {
      if (!res.headersSent) {
        res.statusCode = 500;
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({ error: 'internal error', detail: err.message }));
      }
    }
  };
}

function createServer(pluginList = []) {
  return http.createServer(createHandler(pluginList));
}

function start(port = Number(process.env.PORT) || 8080) {
  const server = http.createServer(createHandler(defaultPlugins()));
  server.listen(port, () => {
    console.log(`gateway listening on :${port}`);
  });
  return server;
}

if (require.main === module) {
  start();
}

module.exports = { createServer, createHandler, start, defaultPlugins };
