#!/bin/bash
# Automated test script for API Gateway
# Prerequisites: backend services running (docker compose up), gateway running (npm start)

GATEWAY="http://localhost:8080"
PASS=0
FAIL=0

check() {
  local description="$1"
  local url="$2"
  local expected_status="$3"
  local method="${4:-GET}"

  actual_status=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" "$url")

  if [ "$actual_status" = "$expected_status" ]; then
    echo "  ✅ $description (HTTP $actual_status)"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $description — expected $expected_status, got $actual_status"
    FAIL=$((FAIL + 1))
  fi
}

check_body() {
  local description="$1"
  local url="$2"
  local expected_substr="$3"

  body=$(curl -s "$url")

  if echo "$body" | grep -q "$expected_substr"; then
    echo "  ✅ $description (body contains \"$expected_substr\")"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $description — expected body to contain \"$expected_substr\", got: $body"
    FAIL=$((FAIL + 1))
  fi
}

echo ""
echo "=== API Gateway Automated Tests ==="
echo ""

echo "[1] Route matching — correct forwarding"
check "GET /api/users/"    "$GATEWAY/api/users/"    200
check "GET /api/orders/"   "$GATEWAY/api/orders/"   200
check "GET /api/products/" "$GATEWAY/api/products/" 200

echo ""
echo "[2] Route matching — no trailing slash"
check "GET /api/users"    "$GATEWAY/api/users"    200
check "GET /api/orders"   "$GATEWAY/api/orders"   200
check "GET /api/products" "$GATEWAY/api/products" 200

echo ""
echo "[3] Health endpoints via gateway"
check "GET /api/users/health"    "$GATEWAY/api/users/health"    200
check "GET /api/orders/health"   "$GATEWAY/api/orders/health"   200
check "GET /api/products/health" "$GATEWAY/api/products/health" 200

echo ""
echo "[4] 404 for unmatched routes"
check "GET /"             "$GATEWAY/"             404
check "GET /api/unknown"  "$GATEWAY/api/unknown"  404
check "GET /random"       "$GATEWAY/random"       404

echo ""
echo "[5] Other HTTP methods"
check "POST /api/users/"      "$GATEWAY/api/users/"    200 POST
check "PUT /api/orders/"      "$GATEWAY/api/orders/"   200 PUT
check "DELETE /api/products/" "$GATEWAY/api/products/" 200 DELETE
check "PATCH /api/users/"     "$GATEWAY/api/users/"    200 PATCH

echo ""
echo "[6] Response body validation"
check_body "Users response contains user data"     "$GATEWAY/api/users/"    "users"
check_body "Orders response contains order data"   "$GATEWAY/api/orders/"   "orders"
check_body "Products response contains product data" "$GATEWAY/api/products/" "products"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
