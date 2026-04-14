const config = require('./config');

function matchRoute(url) {
  // Longest prefix match — sort by prefix length descending
  const sorted = [...config.routes].sort(
    (a, b) => b.prefix.length - a.prefix.length
  );

  for (const route of sorted) {
    if (url === route.prefix || url.startsWith(route.prefix + '/')) {
      return route;
    }
  }
  return null;
}

module.exports = { matchRoute };
