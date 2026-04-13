'use strict';

const httpProxy = require('http-proxy');

const proxy = httpProxy.createProxyServer({
  changeOrigin: true,
  xfwd: true,
  proxyTimeout: 10_000,
  timeout: 10_000,
});

proxy.on('error', (err, req, res) => {
  if (!res || res.headersSent || res.writableEnded) return;
  res.statusCode = 502;
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify({ error: 'bad gateway', detail: err.message }));
});

function forward(req, res, target) {
  proxy.web(req, res, { target });
}

module.exports = { forward, _proxy: proxy };
