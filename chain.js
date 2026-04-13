'use strict';

async function run(req, res, ctx, plugins) {
  for (const plugin of plugins) {
    if (res.headersSent || res.writableEnded) return 'halted';
    let result;
    try {
      result = await plugin(req, res, ctx);
    } catch (err) {
      if (!res.headersSent) {
        res.statusCode = 500;
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify({ error: 'internal error', detail: err.message }));
      }
      return 'halted';
    }
    if (result === false || res.headersSent || res.writableEnded) {
      return 'halted';
    }
  }
  return 'continue';
}

module.exports = { run };
