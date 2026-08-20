# API Gateway 基础框架

基于 **JDK 17 + Spring Boot 3.2 + Spring Cloud 2023 + Spring Cloud Alibaba** 的网关基础框架，
内置三大能力：

| 能力 | 组件 | 说明 |
|------|------|------|
| 注册中心 | Nacos Discovery | 网关自身注册/发现，支持 `lb://` 服务路由 |
| 动态配置管理 | Nacos Config | 配置中心统一管理，变更自动刷新（`@RefreshScope`） |
| 熔断降级 | Sentinel | 路由级 QPS 流控 + 异常比例/慢调用熔断降级 |

## 架构

```
Client ──> Spring Cloud Gateway (:8080)
               │  ├─ 注册中心 Nacos (:8848) —— 服务注册与发现
               │  ├─ 配置中心 Nacos (:8848) —— 动态路由/参数（dataId: api-gateway.yaml）
               │  └─ Sentinel 熔断降级 —— 流控/熔断触发 → 统一降级 JSON
               │
               ├─ /api/users/**   ──> user-service   (:8081)
               ├─ /api/orders/**  ──> order-service  (:8082)
               └─ /api/products/** ──> product-service (:8083)
```

## 启动步骤

**1. 启动后端微服务（根目录）**

```bash
docker compose up --build
```

**2. 启动 Nacos（可选但推荐，注册中心 + 配置中心）**

仅在本目录内提供脚本，不修改根目录 docker-compose：

```bash
# Windows PowerShell
powershell -ExecutionPolicy Bypass -File scripts/start-nacos.ps1
# 或 Git Bash / Linux
bash scripts/start-nacos.sh
```

验证：`curl http://127.0.0.1:8848/nacos/v1/console/health/readiness`

> 不启动 Nacos 也能运行：`spring.config.import: optional:nacos:...` 保证本地兜底启动。

**3. 启动网关**

```bash
mvn spring-boot:run        # 或 mvn package && java -jar target/api-gateway-1.0.0.jar
```

## 验证

```bash
# 基础路由
curl http://localhost:8080/api/users/
curl http://localhost:8080/api/orders/
curl http://localhost:8080/api/products/

# 健康检查
curl http://localhost:8080/actuator/health

# Nacos 注册状态（Nacos 控制台 http://127.0.0.1:8848/nacos 服务列表应看到 api-gateway）
```

## 三大能力演示

### 1. 注册中心（Nacos Discovery）

- 网关启动后自动注册到 Nacos，控制台服务列表可见 `api-gateway`
- 路由 `uri` 支持 `lb://service-name` 负载均衡模式（当前 3 个 Go 后端未注册，
  生产路由用直连 URL；后续 Spring 服务接入后可直接切换）

### 2. 动态配置管理（Nacos Config）

- 网关读取配置 dataId `api-gateway.yaml`（namespace 默认 public / group DEFAULT_GROUP）
- 在 Nacos 控制台修改该配置（如网关参数、路由），网关自动刷新生效，无需重启

### 3. 熔断降级（Sentinel）

规则在 `config/SentinelGatewayConfig.java` 中加载，默认：

| 路由 | 流控 | 熔断降级 |
|------|------|----------|
| user-service | QPS > 10 限流 | - |
| order-service | QPS > 20 限流 | 10s 内异常比例 ≥ 50% → 熔断 10s |
| product-service | - | RT ≥ 500ms 占比 ≥ 60% → 熔断 10s |

**演示熔断**：停掉 order-service → `curl http://localhost:8080/api/orders/` 多次
→ 触发熔断 → 返回 `503` 统一降级 JSON：

```json
{"code":503,"message":"服务熔断降级，请稍后重试"}
```

**演示限流**：`curl -w "\n%{http_code}\n" http://localhost:8080/api/users/` 快速连续请求
> 10 次 → 返回 `429`：

```json
{"code":429,"message":"请求过于频繁，已被网关限流"}
```

## 目录结构

```
gateway/
├── pom.xml
├── src/main/java/com/interview/gateway/
│   ├── GatewayApplication.java        # 启动类
│   ├── config/SentinelGatewayConfig.java  # Sentinel 流控/熔断规则 + 降级响应
│   ├── filter/GatewayLogFilter.java   # 全局访问日志过滤器（扩展点示例）
│   └── fallback/FallbackController.java  # 统一降级端点
├── src/main/resources/application.yml # 路由 + Nacos + Sentinel 配置
├── scripts/                           # Nacos 单机启动脚本
└── README.md
```

## 扩展点

- **规则动态化**：接入 `sentinel-datasource-nacos`，规则存 Nacos 动态推送，改规则不重启
- **动态路由**：接入 Nacos 配置路由定义（`RouteDefinitionRepository`），路由热更新
- **链路追踪**：在 `GatewayLogFilter` 中追加 traceId 透传
- **统一鉴权**：新增 `GlobalFilter` 实现 Token 校验
