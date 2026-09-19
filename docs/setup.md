# DataRay 环境搭建

## 前端

```bash
cd frontend && pnpm install

pnpm dev             # 开发模式 (端口 23351)
pnpm build           # 构建 (tsc + vite build)
pnpm preview         # 预览构建结果
```

前端 API 地址只有一处来源：`src/lib/api/client.ts` 的 `resolveApiBaseURL`。

- 未设置 `VITE_API_BASE_URL`：开发默认 `http://<当前访问主机名>:23352`。
  换访问域名（如 `insights.localhost`）时必须把对应来源加进后端 `[CORS] AllowedOrigins`，
  否则预检不返回 CORS 头、前端只报 `Network Error`。
- 设置 `VITE_API_BASE_URL=""`（空串）：前端只发同源相对路径 `/api`，由反向代理转发（见下方 Docker）。

## 后端

```bash
cd backend && go mod download

go run ./cmd -f etc/config.toml            # 开发模式（写成 cmd/main.go 会因同包 SetupRoutes 报 undefined）
go build -o bin/server ./cmd               # 构建

# 测试
go test ./...
go test -v ./cmd -run TestHandleDatasourcesGET

# 覆盖率
go test -cover ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## Docker

```bash
make docker-up      # 启动所有服务
make docker-logs    # 查看日志
make docker-down    # 停止服务
make dev            # 本地开发 (使用 air 热重载)
```

前端镜像是**两阶段构建**（`frontend/Dockerfile`）：`pnpm build` 产出 `dist`，再由 `nginx:1.27-alpine`
按 `frontend/nginx.conf` 托管，监听 23351：

- `/api/` 反代到 `backend:23352`（依赖 compose 服务名，故该配置只在容器网络内有效）；
  前端构建时 `VITE_API_BASE_URL` 置空 → 走同源相对路径，**生产因此不需要 CORS 白名单**。
- 前端路由回落 `index.html`（`/datasets/12`、`/share/<token>` 直接刷新不会 404）。
- 对外只需暴露 23351；23352 可以不再对外开。

