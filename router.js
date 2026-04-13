'use strict';

function match(path, routes) {
  if (typeof path !== 'string' || !Array.isArray(routes)) return null;
  let best = null;
  for (const route of routes) {
    const { prefix } = route;
    if (path === prefix || path.startsWith(prefix + '/')) {
      if (!best || prefix.length > best.prefix.length) {
        best = route;
      }
    }
  }
  return best ? { ...best } : null;
}

module.exports = { match };
