'use strict';

const DEFAULT_ROUTES = Object.freeze([
  { prefix: '/api/users', target: 'http://localhost:8081' },
  { prefix: '/api/orders', target: 'http://localhost:8082' },
  { prefix: '/api/products', target: 'http://localhost:8083' },
]);

let routes = DEFAULT_ROUTES.map((r) => ({ ...r }));

function getRoutes() {
  return routes.map((r) => {
    const copy = { prefix: r.prefix, target: r.target };
    if (r.auth) copy.auth = { ...r.auth };
    return copy;
  });
}

function validateRoutes(input) {
  if (!Array.isArray(input)) {
    return { ok: false, error: 'routes must be an array' };
  }
  const seen = new Set();
  for (const [i, entry] of input.entries()) {
    if (!entry || typeof entry !== 'object') {
      return { ok: false, error: `routes[${i}] must be an object` };
    }
    const { prefix, target } = entry;
    if (typeof prefix !== 'string' || !prefix.startsWith('/')) {
      return { ok: false, error: `routes[${i}].prefix must be a string starting with /` };
    }
    if (typeof target !== 'string') {
      return { ok: false, error: `routes[${i}].target must be a string` };
    }
    let url;
    try {
      url = new URL(target);
    } catch {
      return { ok: false, error: `routes[${i}].target is not a valid URL` };
    }
    if (url.protocol !== 'http:' && url.protocol !== 'https:') {
      return { ok: false, error: `routes[${i}].target must be http: or https:` };
    }
    if (seen.has(prefix)) {
      return { ok: false, error: `duplicate prefix ${prefix}` };
    }
    seen.add(prefix);
    if (entry.auth !== undefined && entry.auth !== null) {
      if (typeof entry.auth !== 'object') {
        return { ok: false, error: `routes[${i}].auth must be an object` };
      }
      const { user, pass } = entry.auth;
      if (typeof user !== 'string' || user.length === 0) {
        return { ok: false, error: `routes[${i}].auth.user must be a non-empty string` };
      }
      if (typeof pass !== 'string' || pass.length === 0) {
        return { ok: false, error: `routes[${i}].auth.pass must be a non-empty string` };
      }
    }
  }
  return { ok: true };
}

function setRoutes(input) {
  const result = validateRoutes(input);
  if (!result.ok) return result;
  routes = input.map((r) => {
    const entry = { prefix: r.prefix, target: r.target };
    if (r.auth) entry.auth = { user: r.auth.user, pass: r.auth.pass };
    return entry;
  });
  return { ok: true };
}

function resetToDefaults() {
  routes = DEFAULT_ROUTES.map((r) => ({ ...r }));
}

module.exports = {
  getRoutes,
  setRoutes,
  validateRoutes,
  resetToDefaults,
  DEFAULT_ROUTES,
};
