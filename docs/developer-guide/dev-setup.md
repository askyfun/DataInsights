# Data Insights 环境搭建

> 最后更新：2026-09-26

## 前端

```bash
make install-frontend    # 安装依赖（只允许 pnpm）
make dev-frontend        # 开发模式 (端口 23351)
make build-frontend      # 构建 (tsc + vite build)
```

> **包管理器只用 pnpm**（机制硬约束，不是约定）：根目录与 `frontend/` 的 `package.json` 都带
> `preinstall` 钩子 `scripts/only-pnpm.mjs`，它读 `npm_config_user_agent`，非 pnpm（npm/yarn/bun）
> 直接退出码 1 中止安装。`packageManager` 钉死 `pnpm@12.4.2`（根 + frontend 一致），`.npmrc` 开启
> `package-manager-strict`。若本地 pnpm 版本低于钉死的 `packageManager` 版本，strict 会直接拒绝执行 ——
> 升级到钉死版本即可。依赖事实源只有 `pnpm-lock.yaml`。

前端 API 地址只有一处来源：`src/lib/api/client.ts` 的 `resolveApiBaseURL`。两档语义，默认零配置：

- 未配置 `VITE_API_BASE_URL`：开发默认 `http://<当前访问主机名>:23352`（换自定义域名如 `insights.localhost` 也自动跟随，后端 CORS 默认放开，无需任何配置）；生产构建自动同源。
- 配置了非空绝对地址：前端产物烧入该 API 地址 —— 仅前后端分开部署时需要。
- `VITE_SENTRY_DSN`：前端 Sentry。构建期内联，未设置则不初始化 Sentry。

配置统一走仓库根的 `.env`（模板 `.env.example`）：`cp .env.example .env` 后按需修改。
后端**不读任何配置文件**，只认裸名环境变量（无前缀）。优先级（低 → 高）：内置默认值 < `.env` < 真实系统环境变量；`.env` 由 `LoadDotEnv` 装载且**不覆盖**已存在的真实环境变量。

事实源：`backend/internal/config/config.go` 的 `Load()` 与仓库根 `.env.example`。各变量语义：

| 变量 | 语义 |
|------|------|
| `DATABASE_URL` | **唯一必填项**。PostgreSQL 连接串；`main.go` 在 `Load()` 之后显式检查，为空即 `os.Exit(1)` 并打印变量名（fail-fast，不退化成驱动层连接失败）。 |
| `PORT` | HTTP 监听端口，内置默认 `23352`。 |
| `SECURITY_KEY` | 数据源密码 AES-256-GCM 加密密钥，64 位 hex。**留空 = 系统首次启动随机生成（32 字节）并持久化到数据库 `bi_setting` 表**，重启/容器重建复用同一把，保证已加密密码始终可解；显式配置时环境变量优先。 |
| `SENTRY_DSN` | 后端 Sentry DSN，留空 = 关闭上报（前端是独立的 `VITE_SENTRY_DSN`）。 |
| `CORS_ALLOWED_ORIGINS` | 逗号分隔白名单；**留空 = 放开所有来源**（平台 API 无登录态，CORS 不构成安全边界）。 |
| `STATIC_DIR` | 前端构建产物目录；设置后本进程一并托管页面（未命中回落 `index.html`），**留空 = 只提供 API**（本地开发默认形态，页面走 Vite dev server）。目录配错（读不到 `index.html`）进程启动即失败。 |

监听地址固定 `0.0.0.0`，**不提供**环境变量覆盖（容器场景下它是唯一合理值；端口可配、地址不可）。
空字符串一律等同于「未设置」，所以 `.env` 里留空占位不会打掉内置默认值。

## 后端

```bash
make install-backend    # 安装 Go 依赖（等价 cd backend && go mod download）
make dev-backend        # 开发模式（热重载，需要 air）
make build-backend      # 构建后端二进制（bin/server）
make build              # 构建前后端
make serve              # 构建前端并让后端托管 frontend/dist（单端口 23352）
make api-gen            # 从 api/openapi.yaml 生成双端契约类型（Go + TS）
make clean              # 清理构建产物（frontend/dist、frontend/node_modules、backend/bin）
```

> `make dev-backend` 依赖 `air`。若未安装，执行 `go install github.com/air-verse/air@latest`。

需要直接调 go 命令的参考：

```bash
cd backend && go mod download

go run ./cmd                                # 开发模式（写成 cmd/main.go 会因同包 SetupRoutes 报 undefined）
go build -o bin/server ./cmd               # 构建
STATIC_DIR=../frontend/dist go run ./cmd   # 顺带托管前端构建产物（单端口 23352）

# 测试（backend/cmd 现有测试：TestExecuteChartQuery*、TestCORS*、
# TestHealthRoute*、TestRequestID*、TestSentry*、TestResolveSecurityKey*）
go test ./...
go test ./... -race                        # 提交前必跑（带竞态检测）
go test -v ./cmd -run TestExecuteChartQuery
go test -v ./cmd -run TestHealthRouteUsesUnifiedResponse

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

