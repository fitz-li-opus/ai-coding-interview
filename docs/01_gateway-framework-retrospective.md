# API Gateway 基础框架搭建复盘

> 时间：2026-08-20 ｜ 项目：ai-coding-interview ｜ 目录：`gateway/`
> 本文档记录从原始需求到最终交付的完整过程：技术选型、设计决策、遇到的问题与处理方式，以及未完成部分（PR 形式）与设计理由。

---

## 1. 背景与目标

### 1.1 原始需求

在面试脚手架（3 个 Go 后端服务：user/order/product）之上，使用 **Spring Gateway** 搭建底层基础框架，要求：

| 需求 | 初选方案 |
|------|----------|
| ali 熔断降级支持 | Sentinel（Spring Cloud Alibaba） |
| 动态配置管理 | Nacos Config |
| 注册中心 | Nacos Discovery |

### 1.2 需求演进（实际执行过程）

| 阶段 | 用户指令 | 范围 |
|------|----------|------|
| 1 | 搭建基础框架 + 三大能力 | 骨架 + Nacos + Sentinel |
| 2 | 使用 jdk17 和 spring3 | 版本约束：JDK 17 + Spring Boot 3.x |
| 3 | 先移除熔断支持 | 去掉熔断降级（编译受阻） |
| 4 | 仅仅保障路由正常 | 收敛到路由基础能力 |
| 5 | 移除 nacos 支持 | 去掉 Nacos 依赖与脚本 |
| 6 | 不需要 actuator | 去掉运维端点 |

**结论**：最终交付为"纯路由基础框架"，其余能力保留为待办（见第 6 节）。

---

## 2. 技术选型与设计决策

### 2.1 技术栈矩阵

| 项 | 选择 | 版本 | 理由 |
|----|------|------|------|
| 语言 | JDK 17 | temurin-17.0.15（本机已装） | 用户指定；Spring Boot 3 要求 17+ |
| 框架 | Spring Boot | 3.2.4 | 用户指定 Spring 3.x；SCA 2023.0.1.0 官方兼容矩阵对应 Boot 3.2.x |
| 网关 | Spring Cloud Gateway | 2023.0.1 | 响应式 WebFlux、云原生生态、路由谓词/过滤器模型成熟 |
| 微服务套件 | Spring Cloud Alibaba | 2023.0.1.0 | Nacos + Sentinel 一站式，与"ali"生态绑定 |

**为什么用 Spring Cloud Gateway 而非 zuul 2 / 手写**：Gateway 基于 WebFlux 非阻塞，路由谓词（Path/Predicate）+ 过滤器链（GlobalFilter）模型是生产网关的事实标准，且面试场景下表达力最强。

### 2.2 路由设计（当前交付）

```yaml
- id: user-service      # Path=/api/users/**    → ${USER_SERVICE_URI:http://localhost:8081}
- id: order-service     # Path=/api/orders/**   → ${USER_SERVICE_URI:http://localhost:8082}
- id: product-service   # Path=/api/products/** → ${USER_SERVICE_URI:http://localhost:8083}
```

**设计理由**：
- **直连 URL 而非 `lb://` 服务名**：3 个 Go 后端不会注册 Nacos（README 禁止修改 `services/`），`lb://` 需要服务发现参与。直连保证零依赖可跑通 demo；且 `uri` 用环境变量占位，后续切 `lb://` 只需改环境变量。
- **路径原样透传（无 StripPrefix）**：后端 Go 服务返回体带 `path` 字段，透传便于验证"转发目标正确"。
- **未匹配路径 404**：Gateway 默认行为，不配兜底路由，避免静默吞请求。

### 2.3 基础设施边界（用户决策）

- Nacos 等基础设施**仅放 `gateway/` 内**（脚本/README），不修改根目录 `docker-compose.yml` —— 面试仓库根目录是题目给的，应保持最小污染。

---

## 3. 实施过程

1. **骨架**：pom.xml（Boot 3.2.4 + Cloud 2023.0.1 + SCA 2023.0.1.0 BOM）+ 启动类 + application.yml 三条路由 + 日志过滤器 + 降级端点 → `mvn compile` 通过。
2. **Nacos 接入**：`spring.config.import: optional:nacos:api-gateway.yaml` + discovery/config 双配置，`NACOS_ENABLED` 开关控制注册。
3. **Sentinel 接入**：网关流控（GatewayFlowRule）+ 熔断降级（自定义）+ block handler。
4. **逐步精简**：按用户指令移除熔断 → 移除 Nacos → 移除 actuator，最终收敛为纯路由。

---

## 4. 遇到的问题与处理（重点）

### 4.1 编译失败：Sentinel 1.8.6 网关适配器 API 与印象不符

**现象**：`GatewayDegradeRule`、`GatewayCallbackManager` 找不到符号。

**排查**：用 `javap` 反编译本地 Maven 仓库（`D:/maven`）中实际下载的 jar：

- `sentinel-api-gateway-adapter-common-1.8.6.jar`：**没有** `GatewayDegradeRule`，`GatewayRuleConverter` 只把 `GatewayFlowRule` → `FlowRule`（纯流控）；
- `sentinel-spring-cloud-gateway-adapter-1.8.6.jar`：`GatewayCallbackManager`/`BlockRequestHandler` 在 **`sc.callback`** 包（而非 common 包）；
- 反编译 `SentinelGatewayFilter.filter()`：用 `SentinelReactorTransformer` 按**路由 id** 做 entry（`EntryType.IN`），**不会自动把 5xx 响应计为异常**。

**结论与处理**：
1. 1.8.6 网关适配器**原生只支持流控**，无专用网关熔断规则类；
2. 熔断降级需用 sentinel-core 通用 `DegradeRule`（资源名 = 路由 id，与网关 entry 资源一致即可生效），异常比例统计需额外在过滤器中检测 5xx 并 `Tracer.trace`，慢调用 RT 由 reactor 适配器自动统计；
3. 用户评估后先移除熔断（第 4.2 节），待后续 PR 接回（见 PR-3）。

**经验**：**依赖版本 API 以反编译实物为准**，不要凭记忆写代码；maven 本地仓库可能同时存在多版本，先确认解析到的版本。

### 4.2 需求收敛：先移除熔断

**背景**：熔断实现需要额外定制（异常统计过滤器），用户明确"先移除熔断支持，仅仅保障路由正常"。

**处理**：删除熔断规则代码 → 保留网关流控 → 再次收敛时连流控也移除。核心判断：**面试时间有限，先让主干跑通，增强能力后置**，比一次性堆全所有能力更稳。

### 4.3 Nacos 客户端无限重试刷日志

**现象**：无 Nacos 环境启动，`ConfigBatchListenRequest` 循环报 `Client not connected, current status:STARTING`，日志刷屏。

**原因**：`optional:nacos:` 只保证启动不失败，但 nacos-client 一旦加载仍会持续尝试建立 gRPC 长连接并无限重试。

**处理**：曾用 `import-check.enabled: false` + optional 前缀保证可启动；用户最终选择直接移除 Nacos 依赖（需求收敛）。后续接入时（PR-1/2）建议用环境变量开关 `spring.cloud.nacos.config.enabled` 或 profile 隔离，避免无 Nacos 时的噪音。

### 4.4 本地 curl 全部 502，带 `Proxy-Connection` 响应头

**现象**：`curl http://localhost:8081/` 返回 502，响应头含 `Proxy-Connection: keep-alive`。

**根因**：本机配置了系统 HTTP 代理（http_proxy 环境变量），curl 将 localhost 请求也交给代理转发，代理无法访问本机端口返回 502。

**处理**：curl 加 `--noproxy "*"` 直连。同理，PowerShell `Invoke-RestMethod` 需显式绕过代理（`-Proxy` 指向无效地址无效，正确做法是 `-NoProxy` 或 curl）。

**经验**：**Windows 开发机验证本地服务，先确认代理拦截**；后续所有 curl 验证统一 `--noproxy "*"`。

### 4.5 `/actuator/gateway/routes` 返回 404

**现象**：已配置 `exposure.include: health,info,gateway`，但端点列表只有 health/info（日志 `Exposing 2 endpoint(s)`），gateway 端点未注册。

**排查**：Spring Cloud Gateway 4.x 的 `GatewayControllerEndpoint` 受 `@ConditionalOnAvailableEndpoint(endpoint = GatewayEndpoint.class)` 控制，理论上 include 后应暴露；准备加 `management.endpoint.gateway.enabled: true` 验证时，用户要求移除 actuator（需求收敛），放弃深挖。

**待办**：接入时（PR-5）先确认 GatewayEndpoint 是否在 classpath 与条件评估，必要时显式 `enabled: true`。

### 4.6 Edit 工具两次匹配失败

**现象**：编辑 application.yml 删除 management 段时，old_string 两次不匹配（一次写反了替换方向，一次漏掉空行）。

**处理**：先 `Read` 拿到真实文件内容（含空行），再精确匹配删除。

**经验**：**Edit 前先 Read**，多行片段注意空行；同文件多处修改优先整段 Write。

---

## 5. 当前交付（已验证）

### 5.1 文件清单

```
gateway/
├── pom.xml                          # JDK17 + Boot 3.2.4 + Spring Cloud 2023.0.1
├── src/main/resources/application.yml   # 端口 8080 + 3 条路由
└── src/main/java/com/interview/gateway/
    ├── GatewayApplication.java      # 启动类
    ├── filter/GatewayLogFilter.java # 全局访问日志（扩展点示例）
    └── fallback/FallbackController.java  # 统一降级端点
```

### 5.2 验证结果

| 验证项 | 结果 |
|--------|------|
| `mvn clean compile`（JDK 17 / release 17） | ✅ |
| 三条路由转发（响应 `service` 字段 = 目标服务） | ✅ 200 |
| 子路径透传（`/api/orders/1001` 原样到达后端） | ✅ |
| 未匹配路径返回 404 | ✅ |
| POST/DELETE 方法透传 | ✅ |

---

## 6. 未完成部分（PR 形式）

> 每项标注：为什么这么设计、关键实现要点、验收标准。按接入优先级排序。

### PR-1 注册中心（Nacos Discovery）

- **为什么**：网关自注册让运维能看到实例、支持服务实例摘除；路由 `uri` 可切 `lb://service-name` 实现负载均衡（当前 Go 后端不注册，直连是唯一选择）。
- **实现要点**：
  - 恢复依赖 `spring-cloud-starter-alibaba-nacos-discovery` + SCA BOM；
  - `spring.cloud.nacos.discovery.server-addr` + `enabled` 环境变量开关（默认 false，避免无 Nacos 启动噪音）；
  - 提供 `gateway/scripts/start-nacos.ps1/.sh`（standalone 启动，仅 gateway 内处理）。
- **验收**：启动 Nacos 后 `enabled=true` 重启网关，Nacos 控制台服务列表可见 `api-gateway`；`lb://` 路由到已注册服务可转发。

### PR-2 动态配置管理（Nacos Config + 动态路由）

- **为什么**：路由规则/网关参数集中到配置中心，**修改不重启**即可生效；面试点在于"配置热更新"而不只是"配置外置"。
- **实现要点**：
  - `spring.config.import: optional:nacos:api-gateway.yaml`（optional 前缀保证无 Nacos 兜底启动）；
  - 动态路由：实现 `RouteDefinitionRepository`（如 `NacosRouteDefinitionRepository`），监听 dataId（如 `router.json`）变更 → `routeDefinitionWriter.save/delete` + 发布 `RefreshRoutesEvent`；
  - 非路由参数用 `@RefreshScope` + `@ConfigurationProperties` 演示热更新。
- **验收**：Nacos 控制台修改路由定义 → 网关不重启，`/api/xxx` 新路由立即生效。

### PR-3 熔断降级（Sentinel）

- **为什么**：保护网关不把流量打在已故障/高延迟的下游上；面试考察"降级返回统一 JSON"。
- **实现要点（1.8.6 实测结论）**：
  - 网关适配器**无专用熔断规则类** → 用 sentinel-core `DegradeRule`，**资源名 = 路由 id**（与 `SentinelGatewayFilter` 的 entry 资源一致）；
  - 异常比例熔断：需自定义 GlobalFilter 在响应码 ≥ 500 时 `Tracer.trace`（reactor 适配器不自动统计 5xx）；
  - 慢调用熔断：`DEGRADE_GRADE_RT` + `setSlowRatioThreshold`，RT 由 `SentinelReactorTransformer` 自动统计；
  - `GatewayCallbackManager.setBlockHandler`（**`sc.callback` 包**）统一降级响应：熔断 503 / 限流 429；
  - 注意 `sentinel-spring-cloud-gateway-adapter` 传递依赖旧版 `spring-cloud-gateway-server`，需 exclude。
- **验收**：停掉后端 → 连续请求触发熔断 → 返回 503 统一降级 JSON；窗口期后自动恢复。

### PR-4 网关限流（Sentinel 网关流控）

- **为什么**：突发流量防打挂后端，是最常用的网关保护；1.8.6 原生支持，实现成本最低。
- **实现要点**：`GatewayFlowRule`（resourceMode = 路由 id）+ `GatewayRuleManager.loadRules`；规则接入 Nacos 数据源（`sentinel-datasource-nacos`）实现改规则不重启。
- **验收**：对某路由设置 QPS=10，`curl` 连打 > 10 次返回 429 降级 JSON。

### PR-5 运维端点（actuator）

- **为什么**：`/actuator/gateway/routes` 查看路由、健康检查，面试 demo 直观展示路由表。
- **实现要点**：恢复 `spring-boot-starter-actuator`；若 gateway 端点未注册（见 4.5），加 `management.endpoint.gateway.enabled: true` 并验证 `@ConditionalOnAvailableEndpoint` 评估条件。
- **验收**：`/actuator/health` = UP；`/actuator/gateway/routes` 返回 3 条路由 JSON。

### PR-6 统一鉴权/链路日志（扩展点示例）

- **为什么**：生产网关必备；`GatewayLogFilter` 已留扩展位（order=0），加 token 校验 GlobalFilter / traceId 透传即可。
- **验收**：带/不带 token 请求分别返回 200/401；日志含 traceId。

---

## 7. 经验总结

1. **先跑通主干，再堆能力**：面试 40 分钟，路由主干是最低交付线；能力按优先级排队（本次因编译受阻果断移除熔断，保住交付）。
2. **依赖 API 以实物为准**：`javap` 反编译本地 jar 比记忆可靠；版本矩阵（Boot/Cloud/SCA）必须对齐官方兼容表。
3. **本地验证先排除代理干扰**：Windows 开发机 curl 本地服务统一 `--noproxy "*"`。
4. **范围管理靠显式确认**：每次需求收敛都来自用户明确指令，避免我自行删减（如 actuator 是用户主动要求移除）。
5. **基础设施不污染题目仓库**：一切辅助（脚本/文档）落在 `gateway/` 内，根目录仅改 `.gitignore`（补充 `target/`）。
6. **工具纪律**：Edit 前先 Read；同文件多处修改用 Write 整段。
