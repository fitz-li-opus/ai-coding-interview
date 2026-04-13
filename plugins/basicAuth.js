'use strict';

const crypto = require('node:crypto');

function basicAuth(options = {}) {
  const { user, pass, realm = 'gateway' } = options;
  if (typeof user !== 'string' || user.length === 0) {
    throw new Error('basicAuth: user is required');
  }
  if (typeof pass !== 'string' || pass.length === 0) {
    throw new Error('basicAuth: pass is required');
  }
  const expected = Buffer.from(`${user}:${pass}`, 'utf8');

  return async function basicAuthPlugin(req, res) {
    const header = req.headers['authorization'];
    if (typeof header !== 'string' || !header.startsWith('Basic ')) {
      return deny(res, realm);
    }
    const b64 = header.slice(6).trim();
    const decoded = Buffer.from(b64, 'base64');
    if (!safeEqual(decoded, expected)) {
      return deny(res, realm);
    }
    return true;
  };
}

function safeEqual(a, b) {
  if (a.length !== b.length) {
    crypto.timingSafeEqual(Buffer.alloc(b.length), b);
    return false;
  }
  return crypto.timingSafeEqual(a, b);
}

function deny(res, realm) {
  res.statusCode = 401;
  res.setHeader('WWW-Authenticate', `Basic realm="${realm}", charset="UTF-8"`);
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify({ error: 'unauthorized' }));
  return false;
}

module.exports = basicAuth;
