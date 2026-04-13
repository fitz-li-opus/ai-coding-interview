'use strict';

const fs = require('fs');
const path = require('path');
const config = require('./config');

const ADMIN_HTML_PATH = path.join(__dirname, '..', 'public', 'admin.html');

function serveAdminPage(req, res) {
  fs.readFile(ADMIN_HTML_PATH, (err, data) => {
    if (err) {
      res.statusCode = 500;
      res.setHeader('Content-Type', 'application/json');
      res.end(JSON.stringify({ error: 'failed to load admin page' }));
      return;
    }
    res.statusCode = 200;
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.end(data);
  });
}

function sendJson(res, status, body) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify(body));
}

function getRoutes(req, res) {
  sendJson(res, 200, { routes: config.getRoutes() });
}

function readBody(req, limit = 64 * 1024) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > limit) {
        reject(new Error('body too large'));
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    req.on('error', reject);
  });
}

async function putRoutes(req, res) {
  let raw;
  try {
    raw = await readBody(req);
  } catch (err) {
    sendJson(res, 400, { error: err.message });
    return;
  }
  let parsed;
  try {
    parsed = JSON.parse(raw || '{}');
  } catch {
    sendJson(res, 400, { error: 'invalid JSON' });
    return;
  }
  if (!parsed || !Array.isArray(parsed.routes)) {
    sendJson(res, 400, { error: 'body must be { routes: [...] }' });
    return;
  }
  const result = config.setRoutes(parsed.routes);
  if (!result.ok) {
    sendJson(res, 400, { error: result.error });
    return;
  }
  sendJson(res, 200, { routes: config.getRoutes() });
}

module.exports = {
  serveAdminPage,
  getRoutes,
  putRoutes,
};
