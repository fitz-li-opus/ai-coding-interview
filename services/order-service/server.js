const http = require("http");

const orders = [
  { id: 1001, userId: 1, product: "Laptop", amount: 999.99, status: "shipped" },
  { id: 1002, userId: 2, product: "Phone", amount: 699.99, status: "pending" },
  { id: 1003, userId: 1, product: "Headphones", amount: 149.99, status: "delivered" },
];

const server = http.createServer((req, res) => {
  res.setHeader("Content-Type", "application/json");

  if (req.url === "/health") {
    res.writeHead(200);
    res.end(JSON.stringify({ status: "healthy", service: "order-service" }));
    return;
  }

  res.writeHead(200);
  res.end(
    JSON.stringify({
      service: "order-service",
      path: req.url,
      method: req.method,
      timestamp: new Date().toISOString(),
      data: orders,
    }),
  );
});

server.listen(8080, () => {
  console.log("order-service listening on :8080");
});
