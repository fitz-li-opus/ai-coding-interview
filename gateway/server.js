const http = require('http');
const httpProxy = require('http-proxy');
const config = require('./config');
const { matchRoute } = require('./router');

const proxy = httpProxy.createProxyServer({});

proxy.on('error', (err, req, res) => {
  console.error(`[PROXY ERROR] ${req.method} ${req.url} → ${err.message}`);
  if (!res.headersSent) {
    res.writeHead(502, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'Bad Gateway', message: err.message }));
  }
});

const server = http.createServer((req, res) => {
  const route = matchRoute(req.url);

  if (!route) {
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ error: 'Not Found', message: `No route matched for ${req.url}` }));
    return;
  }

  console.log(`[GATEWAY] ${req.method} ${req.url} → ${route.target}`);
  proxy.web(req, res, { target: route.target });
});

server.listen(config.port, () => {
  console.log(`API Gateway listening on port ${config.port}`);
  console.log('Routes:');
  config.routes.forEach((r) => {
    console.log(`  ${r.prefix} → ${r.target}`);
  });
});
