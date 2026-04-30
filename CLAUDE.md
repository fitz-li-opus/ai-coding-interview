# API Gateway Interview - TODO

40 分鐘面試題:在 `gateway/` 目錄實作一個 API Gateway,轉發請求到三個後端服務。

## 後端服務(已提供,不可改)

| Service | Port | Path Prefix |
|---------|------|-------------|
| user-service | 8081 | `/api/users/**` |
| order-service | 8082 | `/api/orders/**` |
| product-service | 8083 | `/api/products/**` |

啟動:`docker compose up --build`,每個服務都有 `/` 跟 `/health`。

---

## 必做 (Minimum Requirement)

- [x] Gateway 監聽 `:8080`
- [x] Path-based routing 到對應後端
  - `/api/users/**` → `localhost:8081`
  - `/api/orders/**` → `localhost:8082`
  - `/api/products/**` → `localhost:8083`
- [x] 保留原始 path、method、header、body
- [x] Gateway 本地跑(`go run ./cmd`,連 docker compose 起的 host port)
- [x] 用 curl 驗證三條路線都通

## Bonus(依 CP 值排序,40 分鐘內挑著做)

### 高 CP 值
- [x] env-driven 路由(`.env` + `godotenv`,k8s ConfigMap 友善)
- [x] `/health` endpoint
- [x] 結構化 log + Request ID(X-Request-Id,方便 trace)
- [x] Timeout 與錯誤處理(後端掛掉回 502,不是 panic)
- [x] Graceful shutdown(收到 SIGTERM 10s drain)

### 中等

- [ ] **Rate limiting**(token bucket,per-IP 或 global)
  - **目的**:保護 backend,防止被單一 client 打爆;在 gateway 集中限流,backend 不用各自實作。
  - **放哪**:`internal/middleware/rate_limit.go`,在 logger 之後、proxy 之前。
  - **怎麼做**:`golang.org/x/time/rate.Limiter`;per-IP 用 `map[string]*Limiter` + `sync.RWMutex`,production 要包 LRU 或 TTL 防 memory leak。
  - **觸發限制**:回 429,header 帶 `Retry-After`。

- [ ] **Retry**(只對冪等方法 GET / HEAD)
  - **目的**:後端偶發網路抖動 / 5xx 自動重試,不讓 client 看到 transient error。
  - **放哪**:`internal/repository/http/backend.go`,把 `*ReverseProxy.Transport` 包一層 retry transport(實作 `http.RoundTripper`)。
  - **限制**:只重 GET / HEAD(冪等);POST / PATCH 重試會雙寫資料。
  - **注意**:context deadline 要傳遞;backoff 要加 jitter,避免 thundering herd;最多 2-3 次。

- [ ] **CORS**
  - **目的**:前端跨 origin 打 gateway 時必備;在 gateway 統一處理,backend 不用煩。
  - **放哪**:`internal/middleware/cors.go`,**logger 之前**(preflight 不需要記 access log)。
  - **重點**:`OPTIONS` preflight 直接回 204,**不要**轉發到 backend;allowed origins / methods / headers 走 env 設定。

### 進階(時間夠才做,否則口頭聊)

- [ ] **Circuit breaker**
  - **目的**:後端持續失敗時「短路」,直接拒絕請求給後端時間恢復,避免雪崩(cascade failure)。
  - **放哪**:`internal/service/routing_service.go` 或包進 `repository/http/backend.go`,per-backend 一個 breaker。
  - **怎麼做**:三態 closed → open → half-open;用 `sony/gobreaker` 或自己寫;觸發條件用「失敗率 + 視窗」。
  - **跟 retry 的關係**:retry 在 breaker 內側,breaker 開了 retry 也跳過。

- [ ] **Auth middleware**(API key / JWT)
  - **目的**:在 gateway 集中認證,backend 信任 gateway 內網流量,不重複實作 auth。
  - **放哪**:`internal/middleware/auth.go`,proxy 之前;`/health` 走白名單。
  - **API key**:header `X-API-Key` lookup;簡單但難 rotate。
  - **JWT**:驗 signature → 解 claims → 塞 context,backend 從 header(`X-User-Id`)讀已驗證身份。
  - **失敗**:回 401,不洩漏為什麼失敗。

- [ ] **Metrics endpoint**(Prometheus `/metrics`)
  - **目的**:RED metrics(**R**ate / **E**rrors / **D**uration)per-route 觀測性,搭 Grafana。
  - **放哪**:`internal/middleware/metrics.go` 量 latency 跟 status;`/metrics` 直接 mount 在 router。
  - **怎麼做**:`prometheus/client_golang`;`Histogram`(latency)+ `Counter`(req_total);label:`method`、`path_prefix`、`status`。
  - **注意**:`/metrics` 要在 logger **之前** mount(否則 Prometheus 每秒 scrape 會洗版 access log);label cardinality 不要爆(別用完整 path 當 label)。

- [ ] **Load balancing**(目前每服務只有 1 instance,意義不大,口頭聊)
  - **目的**:同一個 backend 有多個 instance 時分流。
  - **放哪**:`internal/repository/http/backend.go`,`HTTPBackend` 從持有單一 `*url.URL` 改成 `[]*url.URL` + selector;`ReverseProxy.Director` 每次請求動態挑 target。
  - **策略**:round-robin(最簡單)/ random / least-connections / weighted;production 通常是 least-connections + health-aware。
  - **跟 k8s 的關係**:k8s service 已經做 LB(iptables / IPVS),gateway 端的 LB 在大多數情境是冗餘的;**值得做的時機**是要做 weighted routing(canary)、sticky session、或跨 cluster。

---

## 限制 / 注意

- **不可修改** `services/` 底下任何檔案
- 所有程式碼寫在 `gateway/` 目錄
- 最後以 PR 形式繳交
- 面試官會看螢幕,重點在「跟 AI 怎麼協作」,不只結果

## 已決定

- **技術棧**:Go,只用 stdlib(`net/http` + `net/http/httputil.ReverseProxy`)+ `godotenv`(本地 dev 載 `.env`)
- **跑法**:本地 `go run ./cmd`,連 host 上的 `localhost:8081/8082/8083`(三個 docker service 的 host port)
- **架構**:layered pattern,`/cmd` + `/internal`,參考 `go-layered-server`
- **設定**:env-driven(k8s ConfigMap/Secret 友善),本地 dev 用 `.env`(gitignored),schema 看 `.env.example`

---

## 設計架構

### 目錄結構

```
gateway/
├── cmd/
│   └── main.go                    # 入口:wire 依賴 + 啟 server
├── go.mod
└── internal/
    ├── config/
    │   └── config.go              # PORT + 路由表(MVP hardcode,bonus 改讀檔)
    ├── repository/                # ← microservice 業務隔離(infrastructure)
    │   ├── interfaces.go          # Backend interface
    │   └── http/
    │       ├── backend.go         # 共用 *httputil.ReverseProxy 實作
    │       ├── user_service.go    # NewUserServiceClient
    │       ├── order_service.go   # NewOrderServiceClient
    │       └── product_service.go # NewProductServiceClient
    ├── service/                   # ← gateway 業務層
    │   ├── type.go
    │   └── routing_service.go     # path-prefix → Backend lookup, Forward
    ├── handler/
    │   ├── type.go
    │   ├── health.go              # /health
    │   └── proxy.go               # catchall → service.Routing.Forward
    ├── middleware/
    │   ├── type.go
    │   └── logger.go              # access log + X-Request-Id
    ├── router/
    │   └── router.go              # 組 mux,掛 middleware + handler
    └── factory/
        ├── repository.go          # build 三個 backend
        ├── service.go
        ├── handler.go
        └── middleware.go
```

### 呼叫鏈

```
HTTP request
  → router (mux)
  → middleware.Logger
  → handler.Proxy (catchall)
  → service.Routing.Forward(w, r)                       ← 業務:path → backend
  → repository/http.UserServiceClient.ServeHTTP(w, r)   ← infra:reverse proxy
  → backend service
```

### 設計重點(可講的 trade-off)

1. **每個 microservice 一個檔案,但共用 `HTTPBackend` impl**
   - 業務隔離:per-service 客製(timeout / retry / auth header)只動自己的檔
   - 不重複代碼:底層 reverse proxy wrapper 共用
2. **service 層在 pass-through 看起來薄,但是 bonus feature 的歸宿**(rate limit / auth / transform)
3. **`Backend` interface 而非 concrete type**:可換 `MockBackend`(test)、`GRPCBackend`(未來)
4. **不用 framework**:gateway 是 pure HTTP plumbing,stdlib 就夠;少依賴、啟動快
