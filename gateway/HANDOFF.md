# Gateway 交接文档（多对话接力）

> 用途：本项目需要在多个对话间接力开发。新对话请先读本文件，再按「恢复点」继续。
> 最后更新：Tier1 网关完成并验证；业务策略层（身份+限流）方案已定、尚未实现。

---

## 0. 一句话现状

- **已完成**：Tier1 API 网关（路由、LB、健康检查、重试、超时、中间件链、可观测、优雅退出），单测 + 本地集成全部通过。
- **未开始**：业务策略层。本轮已确认方案：**先身份识别（API Key）→ 再限流（单机内存 + `Store` 抽象，预留分布式）**。
- **进行中改动**：`gateway/internal/httpx/httpx.go` 已加入 `Principal` 类型与上下文读写函数，**但尚未接线、无人使用**（编译与测试仍通过）。这是唯一未完成的代码改动。
- **未提交**：`gateway/` 目前全部为 untracked，无任何 git commit。

---

## 1. 项目目标与硬约束

- 原题：在 `gateway/` 实现轻量 API Gateway，单端口（默认 `:8080`）按规则转发：
  - `/api/users/**` → `http://localhost:8081`
  - `/api/orders/**` → `http://localhost:8082`
  - `/api/products/**` → `http://localhost:8083`
- **禁止修改 `services/`**；所有代码写在 `gateway/`。
- 本机环境：Go 1.27（已 `brew install go`）、**无 Docker**、有 Node 24 / Python 3.14 / Homebrew。
- 设计取向：**零第三方依赖**、单机可运行、可插拔扩展、贴近生产网关语义。

---

## 2. 当前实现（已完成并验证）

### 目录

```
gateway/
├── go.mod                     # module gateway, go 1.27, 零依赖
├── main.go                    # 装配 + 生命周期 + 全局中间件链 + 管理端点
├── main_test.go
├── config/routes.json         # 路由表（含 defaults）
├── internal/
│   ├── config/                # 配置加载/默认值/校验/env 覆盖
│   ├── httpx/                 # requestID / routeInfo / principal / JSON / ClientIP / StatusRecorder
│   ├── router/                # 最长前缀 + method 匹配 + path 重写
│   ├── lb/                    # Instance + Balancer(round_robin/weighted)
│   ├── health/                # Active 探活 + Passive 离群剔除
│   ├── proxy/                 # RouteHandler: 选实例/重试/超时/错误分类
│   ├── middleware/            # Middleware 类型 + Chain + Registry + RequestID/Recovery/Logging/Observe
│   └── metrics/               # 手写 Prometheus 文本
└── scripts/
    ├── run_env.sh             # 公共启动函数（被 dev/e2e source）
    ├── dev.sh                 # 一键本地起 mock service + 网关
    └── e2e.sh                 # 集成断言
```

### 功能清单

| 能力 | 说明 | 位置 |
|---|---|---|
| 路由 | 最长前缀、method 过滤、`strip_prefix` 重写 | `internal/router` |
| 负载均衡 | `Balancer` 接口；`round_robin` / `weighted`（加权随机） | `internal/lb` |
| 主动健康检查 | 每实例探活协程，连续成功/失败阈值翻转 | `internal/health` `Active` |
| 被动离群剔除 | 连续失败 eject + 冷却半开 | `internal/health` `Passive` |
| 重试 | 可重试错误分类 + 换实例 + 退避抖动 + 请求体缓冲重放 | `internal/proxy` |
| 超时 | connect / response header / idle / 每尝试 context | `config.Timeout` |
| 中间件链 | 全局链 + 按路由命名链 + `Registry` | `internal/middleware` |
| 可观测 | `X-Request-ID`、slog JSON 访问日志、`/metrics`、`/healthz`、`/readyz` | `main.go` + `internal/metrics` |
| 稳定性 | 统一错误语义(502/503/504)、panic 恢复、优雅退出 | `internal/proxy` `writeError`, `middleware.Recovery` |
| 配置 | defaults + 路由级覆盖合并、`GATEWAY_LISTEN` / `GATEWAY_UPSTREAM_<ID>` env 覆盖 | `internal/config` |

### 验证结果（全部 PASS）

| 用例 | 内容 |
|---|---|
| L0 | 直连 8081/82/83 `/health` 与 `/` |
| L1 | 三条路由 + `path` 重写（`/api/users`→`/users`） |
| L2 | `requests_total` 递增、`X-Request-ID` |
| L3 | 404 / 405(+`Allow`) / healthz / readyz |
| L4 | 双 user 实例：杀一实例仍全 200、`retries_total=3`、order 隔离；全挂 502→503 |
| L5 | 优雅退出、端口释放 |
| L6 | `GATEWAY_PORT` / `PORT_*` env 覆盖 |
| L7 | `scripts/e2e.sh` 14 项断言 |
| Extra | 504：sleep 上游 + `response_header:1s`，1.03s 返回 |

### 如何运行

```bash
cd gateway
bash scripts/dev.sh          # 前台起环境，Ctrl-C 停
bash scripts/e2e.sh          # 一键集成回归
go test ./... && go vet ./...
# 端口可调：GATEWAY_PORT=18080 PORT_USER=9081 PORT_ORDER=9082 PORT_PRODUCT=9083
```

---

## 3. 架构：请求生命周期与扩展点

```
net/http(timeouts) → RequestID → Recovery → Logging → Observe(metrics)
  → dispatcher(match → 404/405 → strip_prefix → 写 routeInfo)
  → [per-route middleware] → RouteHandler
      → pick(健康+未剔除) → ReverseProxy(Rewrite/SetXForwarded)
      → 失败 = passive.Report + 重试 → 成功/耗尽 → 统一错误
```

三个扩展点（新功能优先复用，不要改主流程）：

1. `lb.Balancer`：`Pick(candidates []*Instance) *Instance`，加算法只需实现接口 + 在 `lb.New` 注册。
2. `middleware.Middleware` + `middleware.Registry`：`func(http.Handler) http.Handler` 组合；路由配置 `middlewares:[...]` 按名字挂载。
3. **未来** `ratelimit.Store`：限流后端抽象（memory / redis），见 §5.2。

配置分层：`defaults`（全局）→ 路由字段级覆盖（`TimeoutFor/RetryFor/HealthFor`）→ 环境变量（listen/upstream）。

---

## 4. 下一阶段方案总览（业务策略层）

**核心认知**：限流/配额依赖"谁在调用"，所以**身份识别是前置件**。推荐顺序：

`① 身份识别(API Key) → ② 单机限流(Store 抽象) → ③ 多租户配额/计量 → ④ 灰度/降级 → ⑤ 缓存/幂等/审计`

| 阶段 | 功能 | 关键设计 |
|---|---|---|
| 1 | 身份识别 | Consumer 注册表、API Key 校验、Principal 注入 context、身份透传下游、401 |
| 2 | 限流 | 三维度(key/算法/scope)、令牌桶、`Store` 接口、429 + RateLimit 头、fail-open |
| 3 | 多租户配额/计量 | 按 consumer/plan 的配额、用量计数、供计费/SLA |
| 4 | 灰度/镜像 | header/权重分流上游组、shadow 镜像流量 |
| 5 | 降级/兜底 | 熔断打开或上游失败时返回缓存/静态默认/fallback 链 |
| 6 | 缓存/幂等/审计/变换 | 路由级缓存、`Idempotency-Key`、审计脱敏、header/正则重写 |

---

## 5. 详细设计

### 5.1 身份识别（Phase 1）

**配置**（`config.Config` 增加）：
```jsonc
{
  "consumers": [
    { "id": "demo",    "api_key": "demo-key",    "plan": "free" },
    { "id": "partner", "api_key": "partner-key", "plan": "gold" }
  ],
  "routes": [
    { "id": "users", "auth": { "required": false, "key_header": "X-API-Key" } }
  ]
}
```

**行为**：
- 从 `key_header`（默认 `X-API-Key`）取 key。
- key 缺失：`required=true` → 401；`required=false` → 匿名放行。
- key 存在但无效 → 401（即使 optional 也拒绝坏凭证）。
- 有效 → 写 `httpx.Principal{ID,Plan}` 到 context；请求头设 `X-Consumer-ID` 透传下游；响应头也回写。
- **安全点**：先删除入站 `X-Consumer-ID` 防伪造，验证通过后再设置。

**新增/改动**：
- `internal/httpx/httpx.go`：`Principal` + `WithPrincipal`/`PrincipalFrom`（**已预置**）。
- `internal/auth/auth.go`（新）：`Consumer`/`Registry`/`Middleware`。
- `internal/config`：`Consumer`、`Auth` 结构 + 校验 + 默认 header；可选 env 覆盖 key：`GATEWAY_CONSUMER_<ID>_KEY`。
- `main.go`：建 registry，按路由装配 auth 中间件（在限流之前）。

**扩展点**：未来 JWT/OAuth2 只需再加一个 `auth` 校验器，Principal 结构不变。

### 5.2 限流（Phase 2，单机 + Store 抽象）

**三个正交维度**：

| 维度 | 取值 | 说明 |
|---|---|---|
| key | `route` / `ip` / `api_key` / `consumer` / `header:<Name>` | "指定服务限流"= 策略按路由独立；key 决定服务内对谁细分 |
| 算法 | 令牌桶（默认）/ 漏桶 / 固定/滑动窗口 / GCRA | 令牌桶允许 `burst` 突发、长期收敛 `rps` |
| scope | 单机内存 / 分布式共享 | 是否分布式取决于**网关副本数**，不是限流本身 |

**单机 vs 分布式决策**：

| 场景 | 单机内存 | 分布式共享 |
|---|---|---|
| 单副本 / 本地 demo | ✅ 精确零依赖 | 过度设计 |
| N 副本 + 全局精确配额 | ❌ 实际 N×rps | ✅ 必须 |
| 多副本 + 可近似 | ⚠️ 有 N 倍误差 | 折中：分层限流 / 本地配额分片 |

**结论（已确认）**：先做**单机内存**，接口按分布式预留。

**接口设计（关键：把选择留给配置）**：
```go
type Decision struct {
    Allowed    bool
    Limit      int
    Remaining  int
    Reset      time.Duration
    RetryAfter time.Duration
}

type Store interface {
    Take(key string, now time.Time) (Decision, error) // memory 现实现；redis 未来实现
}
```

**配置**（路由级）：
```jsonc
"rate_limit": {
  "rps": 100, "burst": 200,
  "key": "consumer",     // route | ip | api_key | consumer | header:X-Tenant
  "store": "memory",     // memory（本轮唯一实现）
  "fail_open": true      // Store 出错时放过（true）还是拒绝(503)（false）
}
```

**响应契约**：
```
429 Too Many Requests
Retry-After: 1
RateLimit-Limit: 100
RateLimit-Remaining: 0
RateLimit-Reset: 1
```

**可选本地 429 演示**：把 `products` 路由设成 `{"rps":5,"burst":5,"key":"route"}`，连续 curl 即可看到 429。

**分布式（Phase 3，可选）**：实现 `RedisStore` + Lua 原子脚本；要点——用 Redis `TIME` 防时钟漂移、处理热点 key、明确 fail-open/fail-closed、关键路径加超时。需要 `brew install redis`（本机可行，无需 Docker）。

### 5.3 后续功能设计要点（Phase 4+）

- **多租户配额/计量**：Consumer 增加 `quota`，按 plan 限流；用量计数导出（`gateway_usage_total{consumer}`）。
- **灰度/金丝雀**：路由匹配扩展 header/cookie；上游分组权重分流；**镜像**：复制请求到影子上游、丢弃响应（暗发布）。
- **降级/兜底**：`ModifyResponse`/ErrorHandler 里对可降级路由返回缓存或静态默认；fallback 上游链。
- **缓存**：路由级 cache key/TTL/`Vary`、`stale-while-revalidate`；Store 同样抽象（memory→redis）。
- **幂等/去重**：`Idempotency-Key` + Store，重复请求返回首次结果。
- **审计/PII**：审计日志 + 字段脱敏。
- **变换**：header 增删改、正则重写 path/query、版本路由 + `Deprecation/Sunset` 头。

（更远：JWT/JWKS、mTLS、协议转码、服务发现。均属"业务策略层"，共用中间件/注册表扩展点。）

---

## 6. 文件级实施计划

### Phase 1：身份识别
1. `internal/config/config.go`：加 `Consumer`、`Auth`（`Required`,`KeyHeader`）、`Config.Consumers`、`Route.Auth`；`applyDefaults` 默认 header；`Validate` 校验 id/key 唯一非空；可选 env 覆盖。
2. `internal/auth/auth.go`（新）：`Registry`（`NewRegistry`/`Lookup`/`Len`）、`Middleware(reg, header, required) middleware.Middleware`。
3. `main.go`：`auth.NewRegistry(cfg.Consumers)`，路由若有 `Auth` 则加中间件（限制流之前）。
4. `config/routes.json`：加 `consumers` 与各路由 `auth`（默认 `required:false`，不破坏现有 demo）。
5. 测试：`internal/auth/auth_test.go`（有效/无效/缺失/required/透传/防伪造）、`config` 新字段校验、`httpx` Principal。

### Phase 2：单机限流
1. `internal/ratelimit/ratelimit.go`（新）：`Decision`、`Store`、`MemoryStore`（令牌桶 + 惰性清理）、`KeyFuncFor`、`Middleware`。
2. `internal/metrics/metrics.go`：加 `IncThrottled(route)` 与 `gateway_ratelimit_throttled_total{route}`。
3. `internal/config/config.go`：加 `RateLimit` 与默认/校验（rps>0、key/store 合法、fail_open 默认 true）。
4. `main.go`：每路由建 `MemoryStore` + 限流中间件（在 auth 之后）；日志打印 consumer 数。
5. `config/routes.json`：各路由加 `rate_limit`（products 可设低限便于演示 429）。
6. 测试：`internal/ratelimit/ratelimit_test.go`（令牌桶补桶/拒绝/retryAfter、key 提取、429 头、fail-open/closed、清理）。
7. `scripts/e2e.sh`：加 auth（带 key 200/坏 key 401）与限流（连续请求得到 429）断言。

### Phase 3+：见 §5.2 分布式 与 §5.3。

---

## 7. 恢复点（下个对话从这里开始）

> **下一步**：执行 Phase 1 第 1 步——在 `internal/config/config.go` 增加 `Consumer`/`Auth` 字段与校验；`httpx.Principal` 已就绪，直接实现 `internal/auth`。

验收命令：
```bash
cd gateway && go build ./... && go vet ./... && go test ./... && bash scripts/e2e.sh
```

实现完成后手工验证：
```bash
bash scripts/dev.sh
curl -s -H 'X-API-Key: demo-key' localhost:8080/api/users -D - -o /dev/null | grep -i x-consumer-id   # demo
curl -s -o /dev/null -w '%{http_code}\n' -H 'X-API-Key: bad' localhost:8080/api/users                # 401
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/users                                     # 200 匿名
```

---

## 8. 已知坑与注意事项（血泪）

- **zsh 变量 `path`**：它是 PATH 的数组别名，脚本里 `path=...` 会清空 PATH 导致 `command not found`。用 `wantpath` 之类变量名。
- **端口环境变量是 `GATEWAY_PORT`**，不是 `GATEWAY_LISTEN`（`run_env.sh` 会用 `GATEWAY_PORT` 覆盖生成 `GATEWAY_LISTEN`）。
- **后台进程收不到 SIGINT**：非交互 shell 启动的后台作业忽略 SIGINT；测试用 `SIGTERM`，真实终端 Ctrl-C 正常。
- **`go run` 会泄漏子进程**：脚本一律 `go build` 出二进制再运行，便于精确 kill。
- **优雅退出与 `ReadHeaderTimeout`**：keep-alive 连接若正处于读请求头状态，`Shutdown` 会等到读超时；已把 `ReadHeaderTimeout` 调到 2s 缓解。
- **metrics 的 `code` 标签无引号**：是 `code=200` 而非 `code="200"`，grep 时注意。
- **services 端口硬编码 `:8080`**：本地用 `sed` 改副本端口，**不要改原文件**。
- **不要修改 `services/`**；`.gitignore` 忽略 `go.sum`/`vendor/`，倾向零依赖。
- 单测不要追满覆盖率（用户已明确收敛），聚焦关键分支。

---

## 9. 交接清单

- [x] Tier1 网关实现并可运行
- [x] 单测 + e2e 通过
- [x] 业务层方案确定（身份→限流→配额→灰度→降级）
- [x] `httpx.Principal` 预置
- [ ] Phase 1 身份识别代码（下一步）
- [ ] Phase 2 单机限流代码
- [ ] e2e 增加 auth/限流断言
- [ ] （可选）git 提交：`gateway/` 当前全部未提交
