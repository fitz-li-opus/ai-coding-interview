# API Gateway - Development Plan

## 開發優先順序

```
Phase 1: 骨架 + Proxy（核心）
   ↓
Phase 2: Auth Middleware
   ↓
Phase 3: Health Check + Circuit Breaker
   ↓
Phase 4: Rate Limiting
   ↓
Phase 5: 收尾
```

---

## Phase 1: 骨架 + Proxy

> 目標：Gateway 能啟動、讀 config、正確轉發請求到三個 upstream

**新增檔案：**
- `gateway/go.mod`
- `gateway/config/config.go`
- `gateway/config/routes.yml`
- `gateway/router/router.go`
- `gateway/proxy/proxy.go`
- `gateway/middleware/logging.go`
- `gateway/middleware/recovery.go`
- `gateway/main.go`

**Checklist：**
- [ ] `go mod init` 建立模組
- [ ] 定義 Config struct，實作 YAML 解析
- [ ] 撰寫 `routes.yml`（三條路由規則）
- [ ] 實作 prefix-based Router（longest match）
- [ ] 實作 ReverseProxy wrapper（含 strip prefix）
- [ ] 實作 Recovery middleware
- [ ] 實作 Logging middleware（slog）
- [ ] `main.go` 組裝：讀 config → 建 router → 掛 middleware → 啟動 server
- [ ] 驗證：`curl localhost:8080/api/users/` 能拿到 user-service 回應
- [ ] 驗證：`curl localhost:8080/not-found` 拿到 404

---

## Phase 2: Auth Middleware

> 目標：未帶合法 API Key 的請求被擋在 gateway 外

**新增檔案：**
- `gateway/middleware/auth.go`

**Checklist：**
- [ ] 定義 `Authenticator` interface
- [ ] 實作 `APIKeyAuthenticator`（從 config 讀取合法 key 清單）
- [ ] 實作 Auth middleware，檢查 `X-API-Key` header
- [ ] `/health` 路徑免驗證
- [ ] 掛進 middleware chain（Logging → Auth → Router）
- [ ] 驗證：不帶 key → 401
- [ ] 驗證：帶合法 key → 正常轉發
- [ ] 驗證：`/health` 不需要 key

---

## Phase 3: Health Check + Circuit Breaker

> 目標：Gateway 暴露聚合健康狀態，upstream 掛掉時自動斷路

**新增檔案：**
- `gateway/health/health.go`
- `gateway/proxy/circuitbreaker.go`

**Checklist：**

Health Check：
- [ ] 實作背景 goroutine，定期 poll upstream `/health`
- [ ] 實作 `GET /health` handler，回傳三態聚合結果（healthy / degraded / unhealthy）
- [ ] `/health` 免 auth、免 rate limit
- [ ] 驗證：所有 upstream 活著 → healthy
- [ ] 驗證：手動停一個 service → degraded

Circuit Breaker：
- [ ] 定義 `CircuitBreaker` interface
- [ ] 實作 in-memory circuit breaker（Closed / Open / HalfOpen 狀態機）
- [ ] 整合進 proxy 層：Router → CircuitBreaker → ReverseProxy
- [ ] Open 狀態直接回 503，不打 upstream
- [ ] Config 支援 failure_threshold / success_threshold / timeout_seconds
- [ ] 驗證：upstream 掛掉後連續失敗 → circuit open → 503
- [ ] 驗證：upstream 恢復後 → half-open → 試探成功 → closed

---

## Phase 4: Rate Limiting

> 目標：Per-IP 限流，防止單一來源打爆 gateway

**新增檔案：**
- `gateway/middleware/ratelimit.go`

**Checklist：**
- [ ] 定義 `RateLimiter` interface
- [ ] 實作 in-memory token bucket（使用 `golang.org/x/time/rate`）
- [ ] 實作 RateLimit middleware，以 client IP 為 key
- [ ] 超限回 429 + `Retry-After` header
- [ ] `/health` 免限流
- [ ] 掛進 middleware chain（Logging → Auth → RateLimit → Router）
- [ ] 驗證：短時間大量請求 → 429

---

## Phase 5: 收尾

> 目標：容器化、優雅關閉、最終驗證

**新增檔案：**
- `gateway/Dockerfile`

**Checklist：**
- [ ] 撰寫 multi-stage Dockerfile（與現有 service 風格一致）
- [ ] 實作 graceful shutdown（SIGTERM / SIGINT）
- [ ] 實作 Request ID propagation（無 ID 則產生，轉發至 upstream header）
- [ ] 確認所有 error response 格式一致（JSON）
- [ ] 確認 log 輸出包含足夠 context
- [ ] 跑一次完整 E2E 驗證（proxy / auth / health / circuit breaker / rate limit）

---

## 模組相依性

```
main.go
  ├── config/config.go       ← 無依賴，最先開發
  ├── proxy/proxy.go         ← 依賴 config
  ├── proxy/circuitbreaker.go ← 依賴 config
  ├── router/router.go       ← 依賴 config, proxy
  ├── middleware/recovery.go  ← 無依賴
  ├── middleware/logging.go   ← 無依賴
  ├── middleware/auth.go      ← 依賴 config
  ├── middleware/ratelimit.go ← 依賴 config
  └── health/health.go       ← 依賴 config
```

開發順序遵循依賴關係：config → proxy → router → middleware → health → main 組裝。
