# syntax=docker/dockerfile:1

# =============================================================================
# Data Insights 单镜像：前端构建产物 + Go 二进制打进同一个镜像，一个进程、一个端口。
#
# 路由分工由 Go 进程按路径完成，不再需要 nginx：
#   /api、/mcp、/health  → 后端内部路由（未匹配到就是 JSON 404，不会回落成页面）
#   其余路径             → 静态目录里的文件；没有对应文件就回落 index.html（SPA 路由）
#
# 构建上下文必须是仓库根目录 —— 镜像要同时看到 frontend/ 与 backend/：
#   docker build -t data-insights .
#   docker run -d --name data-insights -p 23352:23352 --env-file .env data-insights
# =============================================================================


# ---- 阶段 1：构建前端静态产物 ----
FROM node:24-alpine AS webbuild

WORKDIR /app

RUN corepack enable

# corepack 会按 package.json 的 packageManager 拉取 pnpm@12.4.2。
# 无 TTY 时本来就不会交互，这里显式关掉确认提示，免得构建机行为不一致。
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0

# 先只拷依赖清单：改源码不会让依赖安装层缓存失效。
# 两个细节都必须照顾到，否则 install 直接失败：
#   · pnpm-workspace.yaml —— frontend/ 是独立的 pnpm 项目根，依赖构建白名单
#     （allowBuilds: esbuild）写在里面；
#   · scripts/ —— package.json 的 preinstall 钩子要执行 scripts/only-pnpm.mjs，
#     该文件不在这一层就会报 module not found。
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
COPY frontend/scripts/ ./scripts/
RUN pnpm install --frozen-lockfile

COPY frontend/ ./

# 无需强制同源：resolveApiBaseURL 在生产构建（vite build）下默认就是同源相对路径，
# 只有显式配置非空 VITE_API_BASE_URL 才会烧入外部地址（.dockerignore 已排除 .env）。

# Sentry DSN 同样是构建期内联的。默认留空 = 产物不含 DSN、运行时不初始化 Sentry。
ARG VITE_SENTRY_DSN=""
ENV VITE_SENTRY_DSN=${VITE_SENTRY_DSN}

# pnpm build = tsc + vite build，类型错误会让镜像构建失败
RUN pnpm build


# ---- 阶段 2：编译后端 ----
FROM golang:1.27-alpine AS gobuild

WORKDIR /src

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# CGO_ENABLED=0：运行阶段是 alpine，静态链接省掉 libc 依赖。
# -s -w：去掉符号表与调试信息，二进制体积大致减半。
# -trimpath：抹掉构建机路径，产物可复现。
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd


# ---- 阶段 3：运行 ----
FROM alpine:3.24

# ca-certificates：出站 HTTPS（Sentry、TLS 连库）。
# tzdata：不装的话容器里只有 UTC，日志时间与本地对不上。
RUN apk --no-cache add ca-certificates tzdata \
    && adduser -D -u 10001 app

WORKDIR /app

COPY --from=gobuild /out/server ./server
COPY --from=webbuild /app/dist ./web

# 镜像自带的只有静态目录（代码默认值是"空 = 只提供 API"，必须显式指到产物）。
# 监听地址/端口的内置默认值就是 0.0.0.0:23352，无需重复声明；
# 其余配置（DATABASE_URL 等）一律由运行时注入，不进镜像。
ENV STATIC_DIR=/app/web \
    TZ=Asia/Shanghai

# 健康检查直接探后端的 /health —— 同一端口同一进程，无需额外的探活工具。
# PORT 可在运行时覆盖；未设置则回退到默认 23352。
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT:-23352}/health" || exit 1

EXPOSE 23352

# 非 root 运行。二进制与静态产物都在 /app 下，静态文件对任意用户可读。
USER app

CMD ["./server"]
