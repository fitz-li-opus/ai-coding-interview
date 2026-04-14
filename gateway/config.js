const config = {
  port: process.env.GATEWAY_PORT || 8080,

  routes: [
    { prefix: '/api/users', target: 'http://localhost:8081' },
    { prefix: '/api/orders', target: 'http://localhost:8082' },
    { prefix: '/api/products', target: 'http://localhost:8083' },
  ],
};

module.exports = config;
