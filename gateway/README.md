# API Gateway

Go 标准库实现的轻量 API Gateway。请求路径会原样转发给匹配的上游服务。

## 运行

先在仓库根目录启动后端服务：

```bash
docker compose up --build
```

然后启动 Gateway：

```bash
cd gateway
go run .
```

## 配置

| 环境变量 | 默认值 |
|---|---|
| `GATEWAY_ADDRESS` | `:8080` |
| `USER_SERVICE_URL` | `http://localhost:8081` |
| `ORDER_SERVICE_URL` | `http://localhost:8082` |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` |
| `MAX_REQUEST_BODY_BYTES` | `1048576` |

## 验证

```bash
go test ./...
curl -i http://localhost:8080/api/users
curl -i 'http://localhost:8080/api/orders/1001?verbose=true'
curl -i http://localhost:8080/api/products
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```
