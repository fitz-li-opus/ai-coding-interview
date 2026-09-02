# API Gateway Requirements

## 1. 项目目标

使用 Python 在 `gateway/` 目录实现一个轻量级 API Gateway。网关统一监听一个端口，根据请求路径将流量转发到对应的后端微服务，并正确返回后端响应。

项目以 40 分钟编码面试为背景，应优先保证核心链路正确、代码可运行、行为可验证。在核心功能完成后，再实现少量能够体现生产化思考的增强能力。

## 2. 技术约束

- 使用 Python 3.11+。
- 推荐使用 FastAPI、HTTPX 和 Uvicorn。
- 所有网关代码必须放在 `gateway/` 目录。
- 不得修改 `services/` 下的任何代码。
- 后端服务通过现有 `docker-compose.yml` 启动。
- 网关默认监听 `0.0.0.0:8080`。
- 上游服务地址必须支持通过环境变量配置。

## 3. 后端服务

| 服务 | 默认宿主机地址 | 健康检查 |
|---|---|---|
| user-service | `http://localhost:8081` | `/health` |
| order-service | `http://localhost:8082` | `/health` |
| product-service | `http://localhost:8083` | `/health` |

当网关运行在 Docker 网络中时，应允许通过配置将上游地址替换为 `user-service:8080`、`order-service:8080` 和 `product-service:8080`。

## 4. 需求优先级

### P0：必须完成

P0 是项目的最低交付要求。任一 P0 功能缺失，都视为核心需求未完成。

#### P0.1 路由转发

网关必须支持以下路由：

| 网关路径 | 目标服务 |
|---|---|
| `/api/users`、`/api/users/**` | user-service |
| `/api/orders`、`/api/orders/**` | order-service |
| `/api/products`、`/api/products/**` | product-service |

验收标准：

- 三类路径分别由正确的后端服务处理。
- `/api/users-invalid` 等相似但不合法的路径不得匹配 users 路由。
- 未配置的路由返回 HTTP `404` 和 JSON 错误响应。

#### P0.2 路径重写

网关应移除 API 路由前缀后再请求上游：

| 客户端路径 | 上游收到的路径 |
|---|---|
| `/api/users` | `/` |
| `/api/users/` | `/` |
| `/api/users/123` | `/123` |
| `/api/orders/1001/items` | `/1001/items` |

验收标准：后端响应中的 `path` 字段与预期重写结果一致。

#### P0.3 请求透传

网关必须正确透传：

- HTTP 方法；
- 请求体；
- 查询参数；
- 必要的请求 Header，例如 `Authorization`、`Content-Type` 和 `Accept`。

至少应支持 `GET`、`POST`、`PUT`、`PATCH`、`DELETE`、`OPTIONS` 和 `HEAD`。

验收标准：通过测试或 curl 能确认方法、查询参数和请求内容未被错误修改。

#### P0.4 响应透传

网关必须将上游的以下内容返回给客户端：

- HTTP 状态码；
- 响应体；
- 适合端到端传递的响应 Header。

网关应过滤 `Connection`、`Keep-Alive`、`Transfer-Encoding` 等逐跳 Header，请求侧与响应侧均不得错误透传这些 Header。

#### P0.5 上游错误处理

| 场景 | 网关状态码 |
|---|---:|
| 无匹配路由 | `404` |
| 无法连接上游 | `502` |
| 上游请求超时 | `504` |
| 网关未处理异常 | `500` |

验收标准：错误响应使用 JSON 格式，不向客户端暴露 Python 异常栈等内部实现信息。

#### P0.6 配置与连接管理

至少支持以下环境变量：

```text
GATEWAY_HOST=0.0.0.0
GATEWAY_PORT=8080
USER_SERVICE_URL=http://localhost:8081
ORDER_SERVICE_URL=http://localhost:8082
PRODUCT_SERVICE_URL=http://localhost:8083
UPSTREAM_TIMEOUT_SECONDS=5
```

HTTPX 客户端应在应用生命周期内复用连接池，不应为每个请求创建一个新的 `AsyncClient`。

#### P0.7 可运行与可演示

交付物必须包含：

- Python 网关源代码；
- 依赖声明；
- 启动说明；
- curl 演示命令。

验收标准：启动三个后端和网关后，可以通过网关分别获得 users、orders 和 products 数据。

### P1：核心完成后优先实现

P1 功能用于提高可观测性、可靠性和可测试性，应在 P0 稳定后实现。

#### P1.1 网关健康检查

- `GET /health` 检查网关进程是否存活。
- 正常时返回 HTTP `200` 和 JSON 状态。
- `/health` 不依赖上游服务状态。

#### P1.2 请求 ID

- 如果请求包含 `X-Request-ID`，继续使用该值。
- 如果没有，则生成新的请求 ID。
- 将请求 ID 传递给上游，并写入客户端响应 Header。
- 错误响应中也应包含请求 ID。

#### P1.3 访问日志

每次请求至少记录：

- 请求 ID；
- HTTP 方法；
- 请求路径；
- 响应状态码；
- 请求耗时；
- 目标服务或路由名称。

日志不得输出 Authorization、Cookie 等敏感信息。

#### P1.4 自动化测试

测试不应强依赖真实 Docker 后端，推荐通过应用工厂和 HTTPX `MockTransport` 注入模拟上游。

至少覆盖：

- 三条路由正确选择上游；
- 路径重写；
- 查询参数保留；
- 方法、请求体和 Header 透传；
- 上游状态码和响应体透传；
- 未知路由返回 `404`；
- 上游连接失败返回 `502`；
- 上游超时返回 `504`；
- `/health` 返回 `200`。

#### P1.5 优雅退出

应用退出时应停止接受新请求，并正确关闭共享的 HTTPX 客户端连接池。

### P2：可选增强

只有在 P0 和 P1 已完成且验证通过后，才考虑 P2。

#### P2.1 上游就绪检查

提供 `GET /ready`，并发请求三个后端的 `/health`：

- 全部必要服务正常时返回 `200`；
- 任一必要服务不可用时返回 `503`；
- 响应中展示各服务状态。

#### P2.2 流式响应

使用 HTTPX 流式读取和 FastAPI `StreamingResponse` 返回上游响应，避免将大响应完整加载到内存。

#### P2.3 容器化

提供 `gateway/Dockerfile`，使网关可以作为独立容器构建运行。是否修改根目录 `docker-compose.yml` 应根据剩余时间决定，且不得修改已有后端服务代码。

#### P2.4 基础安全限制

可根据时间增加：

- 最大请求体限制；
- 严格的上游服务白名单；
- 基础 CORS 配置；
- 常见安全响应 Header。

## 5. 非功能要求

- 代码结构清晰，路由配置与代理逻辑分离。
- 避免不必要的第三方依赖和过度设计。
- 默认配置下可以在宿主机直接运行。
- 超时、连接错误和异常行为必须可预测。
- 代码应易于扩展新的路径和上游服务。
- README 中应解释关键设计决策及其取舍。

## 6. 当前不在实现范围

以下能力可以在设计讨论中说明，但不要求在本次限时实现中完成：

- 用户认证与权限系统；
- 分布式限流；
- 熔断器和自动重试策略；
- 动态服务发现；
- 负载均衡；
- WebSocket 代理；
- Redis、数据库或消息队列集成；
- 完整指标、链路追踪和监控平台接入。

不默认实现自动重试，因为对非幂等请求进行重试可能造成重复写入。

## 7. 最终验收流程

```bash
# 1. 启动后端服务
docker compose up --build

# 2. 安装依赖并启动网关
cd gateway
python -m pip install -r requirements.txt
uvicorn main:app --host 0.0.0.0 --port 8080

# 3. 验证核心路由
curl -i http://localhost:8080/api/users
curl -i http://localhost:8080/api/orders/1001
curl -i 'http://localhost:8080/api/products?category=electronics'

# 4. 验证网关行为
curl -i http://localhost:8080/health
curl -i http://localhost:8080/unknown

# 5. 运行自动化测试
pytest
```

成功标准：三类业务请求均由正确服务响应，路径和查询参数符合重写规则，未知路由及上游故障返回明确且稳定的错误状态码。
