const GATEWAY = Bun.env.GATEWAY_URL ?? "http://localhost:8080";

const cases = [
  { path: "/health", expectService: "gateway" },
  { path: "/api/users", expectService: "user-service" },
  { path: "/api/orders", expectService: "order-service" },
  { path: "/api/products", expectService: "product-service" },
];

let failed = 0;

for (const c of cases) {
  const url = GATEWAY + c.path;
  try {
    const res = await fetch(url);
    const body = await res.json().catch(() => ({}));
    const ok = res.ok && body?.service === c.expectService;
    console.log(
      `${ok ? "PASS" : "FAIL"}  ${res.status}  ${url}  service=${body?.service ?? "-"}`,
    );
    if (!ok) failed++;
  } catch (err) {
    console.log(`FAIL  ERR   ${url}  ${(err as Error).message}`);
    failed++;
  }
}

process.exit(failed === 0 ? 0 : 1);
