const http = require("http");

const products = [
  { id: 1, name: "Laptop", price: 999.99, stock: 50, category: "electronics" },
  { id: 2, name: "Phone", price: 699.99, stock: 120, category: "electronics" },
  { id: 3, name: "Headphones", price: 149.99, stock: 200, category: "accessories" },
  { id: 4, name: "Keyboard", price: 79.99, stock: 300, category: "accessories" },
];

const server = http.createServer((req, res) => {
  res.setHeader("Content-Type", "application/json");

  if (req.url === "/health") {
    res.writeHead(200);
    res.end(JSON.stringify({ status: "healthy", service: "product-service" }));
    return;
  }

  res.writeHead(200);
  res.end(
    JSON.stringify({
      service: "product-service",
      path: req.url,
      method: req.method,
      timestamp: new Date().toISOString(),
      data: products,
    }),
  );
});

server.listen(8080, () => {
  console.log("product-service listening on :8080");
});
