const http = require("http");

const users = [
  { id: 1, name: "Alice", email: "alice@example.com", role: "admin" },
  { id: 2, name: "Bob", email: "bob@example.com", role: "user" },
  { id: 3, name: "Charlie", email: "charlie@example.com", role: "user" },
];

const server = http.createServer((req, res) => {
  res.setHeader("Content-Type", "application/json");

  if (req.url === "/health") {
    res.writeHead(200);
    res.end(JSON.stringify({ status: "healthy", service: "user-service" }));
    return;
  }

  res.writeHead(200);
  res.end(
    JSON.stringify({
      service: "user-service",
      path: req.url,
      method: req.method,
      timestamp: new Date().toISOString(),
      data: users,
    }),
  );
});

server.listen(8080, () => {
  console.log("user-service listening on :8080");
});
