# API Gateway 实现纲要

## 1. 目标

使用 Go 标准库实现一个轻量、可靠、易维护的 API Gateway，监听 `8080` 端口，并将请求转发到对应的后端服务。

本次实现优先保证：

- 路由正确
- 安全边界明确
- 故障行为可控
- 代码结构清晰
- 关键行为可验证

## 2. 基础路由

| Gateway 路径 | 上游服务 |
|---|---|
| `/api/users`、`/api/users/**` | `http://localhost:8081` |
| `/api/orders`、`/api/orders/**` | `http://localhost:8082` |
| `/api/products`、`/api/products/**` | `http://localhost:8083` |

路由必须按完整路径段匹配。例如 `/api/users-admin` 不得匹配 `/api/users`，未知路径统一返回 `404`。

默认保留原始请求路径、查询参数、HTTP 方法、请求体和必要请求头。上游响应的状态码、响应头和响应体应正确返回客户端。

## 3. 必须实现的能力

### 3.1 配置与启动校验

- 监听地址和三个上游地址支持通过环境变量配置。
- 启动时解析并校验全部上游 URL。
- 上游只能来自可信配置，客户端不能动态指定目标地址，避免 SSRF。
- 配置无效时拒绝启动，而不是等请求到达后才失败。

### 3.2 可靠反向代理

- 使用 `net/http/httputil.ReverseProxy`，不手写请求和响应复制。
- 正确处理 path、query、method、body、headers 和 status code。
- 清理客户端伪造的转发头，并由 Gateway 重新生成可信的 `X-Forwarded-*` 信息。
- 上游连接失败返回 `502 Bad Gateway`。
- 上游超时返回 `504 Gateway Timeout`。
- 对客户端使用统一 JSON 错误格式，不暴露内部地址或底层网络错误。

### 3.3 资源保护

- 设置 `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout` 和 `IdleTimeout`。
- 设置 `MaxHeaderBytes`。
- 限制请求体大小，超限返回 `413 Payload Too Large`。
- 为上游 HTTP Transport 设置连接和响应头超时，避免请求无限等待。

### 3.4 可观测性

- 为每个请求生成或校验 `X-Request-ID`。
- 将 Request ID 返回客户端并传递给上游。
- 输出结构化访问日志，至少包含：
  - request ID
  - method
  - path
  - route
  - status
  - duration
- 不记录 Authorization、Cookie、完整请求体等敏感数据。

### 3.5 健康检查与生命周期

- `/healthz`：检查 Gateway 进程是否可服务。
- `/readyz`：使用短超时检查三个上游的 `/health`。
- 收到退出信号时优雅关闭，给正在处理的请求留出完成时间。

## 4. 推荐代码结构

```text
gateway/
├── main.go                      # 依赖组装、HTTP Server 和优雅关闭
├── config.go                    # 环境配置读取与启动校验
├── gateway.go                   # 路由匹配、反向代理和健康检查
├── middleware.go                # Request ID、日志、恢复、请求限制
├── gateway_test.go              # Gateway 行为测试
├── go.mod
├── Dockerfile
└── README.md
```

当前规模使用一个 `package main`，通过文件和类型保留逻辑边界，不为每个职责创建独立 package。配置 Loader 不进入 Gateway；它只负责将环境变量转换成经过校验的 `Config`。Gateway 只依赖结构化配置。

构造依赖保持简单：

```text
Config Loader -> Config -> Gateway -> HTTP Server
```

Gateway 内部包含只读路由表，每个 Route 持有一个 `http.Handler`。当前 Handler 是 `ReverseProxy`；未来可以替换或包装为鉴权、灰度路由、负载均衡等实现，而不需要修改路由匹配流程。

不引入依赖注入框架、插件系统或没有实际替换需求的自定义接口。优先使用 Go 标准库、`http.Handler` 和显式构造函数。只有动态服务发现、多协议代理或多种 Proxy 实现成为真实需求时，才提取独立 package。

## 5. Middleware 顺序

请求处理链与构造依赖是两个不同概念。建议请求处理顺序：

```text
panic recovery
  -> request ID
  -> access log
  -> request body limit
  -> Gateway route match
  -> reverse proxy
  -> upstream
```

路由匹配和 URL 改写属于 Gateway 核心逻辑，不包装成通用 Middleware，也不与环境配置读取耦合。

## 6. 最低测试范围

- 三组基础路由分别到达正确上游。
- `/api/users` 和 `/api/users/**` 均可匹配。
- `/api/users-admin` 和未知路径返回 `404`。
- method、query、body 和 headers 能正确转发。
- 上游 status、headers 和 body 能正确返回。
- 上游不可用时返回 `502`。
- 上游超时时返回 `504`。
- 请求体过大时返回 `413`。
- Request ID 能返回客户端并传递给上游。
- 健康检查能反映 Gateway 和上游状态。

代理行为优先使用 `httptest.Server` 验证，避免测试依赖真实端口和 Docker。

## 7. 本次暂不实现

以下能力有生产价值，但不属于第一版基础范围：

- JWT 或 API Key 鉴权
- 每客户端限流
- 熔断和自动重试
- 服务发现和动态配置
- 权重路由和灰度发布
- Prometheus metrics 和分布式 tracing
- 响应缓存

这些能力需要明确业务语义和信任边界。基础代理稳定后，再以独立策略模块逐项增加。

## 8. 完成标准

Gateway 应能够独立构建和运行，并通过 curl 清楚演示：

```bash
curl -i http://localhost:8080/api/users
curl -i http://localhost:8080/api/orders/1001?verbose=true
curl -i http://localhost:8080/api/products
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

完成时应同时具备运行说明、配置说明、自动化测试，以及对超时、安全边界和未实现能力的简短设计说明。
