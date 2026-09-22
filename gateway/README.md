# Go API Gateway

该网关通过 HTTPS 接收请求，按照 `Host` 与路径前缀选择 upstream，并在转发前删除匹配的路径前缀。

## 运行

在仓库根目录生成仅供本地演示使用的证书：

```bash
./gateway/scripts/generate-dev-cert.sh
```

运行测试镜像：

```bash
docker build --target test -t api-gateway-test gateway
```

启动三个后端和网关：

```bash
docker compose up --build
```

验证路径路由：

```bash
curl --cacert gateway/.local/certs/server.crt https://localhost:8443/api/users
curl --cacert gateway/.local/certs/server.crt https://localhost:8443/api/orders
curl --cacert gateway/.local/certs/server.crt https://localhost:8443/api/products
```

验证域名路由：

```bash
curl --noproxy '*' --resolve users.internal.test:8443:127.0.0.1 --cacert gateway/.local/certs/server.crt https://users.internal.test:8443/profile
curl --noproxy '*' --resolve orders.internal.test:8443:127.0.0.1 --cacert gateway/.local/certs/server.crt https://orders.internal.test:8443/profile
curl --noproxy '*' --resolve products.internal.test:8443:127.0.0.1 --cacert gateway/.local/certs/server.crt https://products.internal.test:8443/profile
```

检查网关状态：

```bash
curl --cacert gateway/.local/certs/server.crt https://localhost:8443/health
curl --cacert gateway/.local/certs/server.crt https://localhost:8443/ready
```

## 配置

启动参数全部为必填项：

```text
gateway --config <yaml-path> --tls-cert <certificate-path> --tls-key <private-key-path>
```

[`config.example.yaml`](./config.example.yaml) 包含三个 upstream、三条路径规则和三条域名规则。每个 upstream 可以配置多个 endpoint，网关只向健康实例发送请求，并使用 round-robin 选择实例。

业务请求的默认限制如下：

- 请求体上限为 1 MiB。
- 每条 route 与每个来源 IP 共享独立 token bucket。
- route 超时为三秒。
- TLS 最低版本为 1.2。
- `/health` 表示网关进程可以响应请求。
- `/ready` 表示所有已引用 upstream 均有健康实例。
