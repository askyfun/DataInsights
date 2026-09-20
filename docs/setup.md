# Data Insights 环境搭建

## 前端

```bash
make install-frontend    # 安装依赖（只允许 pnpm）
make dev-frontend        # 开发模式 (端口 23351)
make build-frontend      # 构建 (tsc + vite build)
```

> 若本机没有 pnpm，先执行 `npm install -g pnpm`。npm / yarn 会被 `preinstall` 钩子拦截并中止。

前端 API 地址只有一处来源：`src/lib/api/client.ts` 的 `resolveApiBaseURL`。两档语义，默认零配置：

- 未配置 `VITE_API_BASE_URL`：开发默认 `http://<当前访问主机名>:23352`（换自定义域名如 `insights.localhost` 也自动跟随，后端 CORS 默认放开，无需任何配置）；生产构建自动同源。
- 配置了非空绝对地址：前端产物烧入该 API 地址 —— 仅前后端分开部署时需要。
- `VITE_SENTRY_DSN`：前端 Sentry。构建期内联，未设置则不初始化 Sentry。

配置统一走仓库根的 `.env`（模板 `.env.example`）：`cp .env.example .env` 后按需修改。
后端**不读任何配置文件**，只认裸名环境变量（无前缀），其中 `DATABASE_URL` 必填。

## 后端

```bash
make install-backend    # 安装 Go 依赖（等价 cd backend && go mod download）
make dev-backend        # 开发模式（热重载，需要 air）
make build-backend      # 构建二进制
make serve              # 构建前端并让后端托管 frontend/dist（单端口 23352）
```

> `make dev-backend` 依赖 `air`。若未安装，执行 `go install github.com/cosmtrek/air@latest`。

需要直接调 go 命令的参考：

```bash
cd backend && go mod download

go run ./cmd                                # 开发模式（写成 cmd/main.go 会因同包 SetupRoutes 报 undefined）
go build -o bin/server ./cmd               # 构建
STATIC_DIR=../frontend/dist go run ./cmd   # 顺带托管前端构建产物（单端口 23352）

# 测试
go test ./...
go test -v ./cmd -run TestHandleDatasourcesGET

# 覆盖率
go test -cover ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## Docker

单镜像单进程：前后端编译进**一个镜像**，由**一个 Go 进程、一个端口（23352）**同时提供 API 与页面。首次构建约 5-10 分钟。

```bash
make docker-build   # 构建镜像（等价 docker build -t data-insights .）
make docker-up      # 启动（compose：PostgreSQL + app，自动 --build）
make docker-logs    # 查看日志
make docker-down    # 停止服务
make dev            # 本地开发 (使用 air 热重载)
```

> compose 路径所有环境变量都有默认值，不需要 `.env` 文件即可启动。

直接用 Docker（不经过 compose，需要自备 PostgreSQL）：

```bash
docker build -t data-insights .

# DATABASE_URL 是唯一必填项
docker run -d -p 23352:23352 \
  -e DATABASE_URL=postgres://user:password@your-db-host:5432/dbname?sslmode=disable \
  --name data-insights data-insights

# 或通过 .env 文件传入（cp .env.example .env 后按需修改）
docker run -d -p 23352:23352 --env-file .env --name data-insights data-insights
```

`Dockerfile` 分三个阶段：`node:24-alpine` 构建前端产物 → `golang:1.27-alpine` 编译后端 →
`alpine:3.24` 运行（二进制 + `dist` 一起拷进去，非 root 用户运行，带 `/health` HEALTHCHECK）。

**没有 nginx。** 路径分流由 Go 自己完成（`backend/internal/webui`）：

| 路径 | 去处 |
|------|------|
| `/api`、`/mcp`、`/health` | 后端内部路由；未匹配到返回 JSON 404，**不会回落成 HTML** |
| 其余路径 | `STATIC_DIR`（镜像内 `/app/web`）里的文件；未命中回落 `index.html` |

- 前端构建时 `VITE_API_BASE_URL` 被强制置空 → 走同源相对路径，**生产不需要 CORS 白名单**。
- 前端路由回落 `index.html`（`/datasets/12` 这类深链直接刷新不会 404）。
- `assets/` 下带哈希的产物 `immutable` 长缓存，其余 `no-cache`。
- 对外只需暴露 23352。静态目录配错（读不到 `index.html`）会让进程启动即失败。
- 不想用 Docker 也可以验证同一形态：`make serve`（构建前端产物后由后端托管 `frontend/dist`）。

