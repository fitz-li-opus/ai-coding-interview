'use strict';

module.exports = {
  basicAuth: require('./basicAuth'),
  // TODO: rateLimit   — per-IP request throttle (429 on excess).
  // TODO: ipBlock     — static allow/deny list (403 on denied IP).
  // TODO: commonExploit — reject malicious headers / path-traversal / oversized input (400).
};
