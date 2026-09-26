# 容器部署（Docker）

> 最后更新：2026-09-26
> 本文讲容器镜像、Compose 与镜像版本。生产环境实践见[生产部署](production.md)，如何发布一个新版本见[发布流程](release.md)。

## 1. 单镜像单进程模型

前端构建产物与 Go 二进制打进**同一个镜像**，由一个进程、一个端口（23352）同时提供 API 与页面 —— **没有 nginx**。

路径分流由 Go 进程自己完成：

| 路径 | 去处 |
|------|------|
| `/api`、`/mcp`、`/health` | 后端内部路由；未匹配到返回 JSON 404，**不会回落成 HTML** |
| 其余路径 | `STATIC_DIR`（镜像内 `/app/web`）里的文件；未命中回落 `index.html`（交给前端路由） |

- `assets/` 下带内容哈希的产物走 `immutable` 长缓存，其余 `no-cache`。
- 静态目录**配错即启动失败**（读不到 `index.html` 就退出），不会跑起来之后整站 404。

## 2. 取镜像的两条路

| | 发布镜像（推荐给使用者） | 本地构建（推荐给改代码的人） |
|---|---|---|
| 来源 | Docker Hub `kzzhr/datainsights` | 本仓库 `Dockerfile` |
| 命令 | `docker pull kzzhr/datainsights:latest` | `docker build -t data-insights .` |
| 耗时 | 拉取几十 MB | 首次约 5–10 分钟 |
| 架构 | `linux/amd64` + `linux/arm64` | 仅本机架构 |
| 版本 | tag 与 GitHub Releases 一一对应 | 跟随当前源码 |

发布镜像由 [GitHub Actions 发布流水线](release.md)自动构建推送，**架构、tag 规则与推送目标都以那份文档为准**。

## 3. Compose：一键起全套（发布镜像 + PostgreSQL）

[`docker-compose.allinone.yml`](../../docker-compose.allinone.yml) 起 **PostgreSQL + app** 两个服务，app 直接用发布镜像（不构建）：

```bash
curl -fsSLO https://raw.githubusercontent.com/askyfun/DataInsights/master/docker-compose.allinone.yml
docker compose -f docker-compose.allinone.yml up -d

# 钉版本
TAG=v1.0.0 docker compose -f docker-compose.allinone.yml up -d
```

- 零外部依赖：所有环境变量都有默认值，**不需要 `.env` 即可启动**。
- 数据落在命名卷 `postgres_data`：`down` 保留，`down -v` 清空。
- ⚠️ 库里是公开的演示密码 `insights123`，仅本地体验用；放到他人可达的主机上前先改密码（并同步改 app 的 `DATABASE_URL`），或换成外部数据库只跑 app。本项目**无登录、无鉴权**，不要暴露到公网 —— 见[生产部署](production.md)。
- 变量覆盖优先级（高 → 低）：① shell 环境变量；② 同目录 `.env`（compose 自动读来做插值）；③ 文件内默认值。
- 容器内监听端口与宿主机映射端口一致，由 `PORT` 统一控制。

## 4. 直接用 Docker（自备 PostgreSQL）

```bash
# 拉发布镜像（或先 docker build -t data-insights . 自建）
docker pull kzzhr/datainsights:latest

docker run -d -p 23352:23352 \
  -e DATABASE_URL=postgres://user:password@your-db-host:5432/dbname?sslmode=disable \
  --name data-insights kzzhr/datainsights:latest

# 或通过 .env 文件传入（需要在本地先有仓库：cp .env.example .env 后按需修改）
docker run -d -p 23352:23352 --env-file .env --name data-insights kzzhr/datainsights:latest
```

`DATABASE_URL` 是**唯一必填项**。生产构建默认同源，**不需要** CORS 白名单；只有前后端分开部署时才用 `VITE_API_BASE_URL` 烧入外部 API 地址。

## 5. 镜像构建细节

`Dockerfile` 分三个阶段：

| 阶段 | 基础镜像 | 作用 |
|------|----------|------|
| webbuild | `node:24-alpine` | 装前端依赖并构建静态产物 |
| gobuild | `golang:1.27-alpine` | 编译后端二进制（`CGO_ENABLED=0` 静态链接、`-trimpath` 可复现） |
| runtime | `alpine:3.24` | 只放二进制 + 产物，**非 root 用户**运行，带 `/health` HEALTHCHECK |

几个容易踩的点：

- ⚠️ **`preinstall` 钩子需要 `scripts/` 已 COPY**：依赖安装层必须同时 COPY `package.json` + `pnpm-lock.yaml` + `pnpm-workspace.yaml` + `scripts/`（`preinstall` 会执行 `scripts/only-pnpm.mjs`）。漏了 `scripts/` 会报 module not found。
- ⚠️ **`.dockerignore` 必须排除 `.env`**：Vite 构建阶段的 `envDir` 指向仓库根，`.env` 一旦进入构建上下文，其中的 `VITE_*` 会被内联进浏览器产物。
- 依赖安装层与源码层分开，改源码不会让依赖安装缓存失效。
- `VITE_*` 只能以 **build arg** 传入（构建期内联），容器起来后再给环境变量没有任何作用。
- 健康检查直接探同一端口的 `/health`（同一进程，无需额外探活工具）；`PORT` 可在运行时覆盖。

## 6. 排障

- **端口冲突**：宿主 23352 被占用时改 `PORT`（compose 的 `ports` 映射已随之同步）。
- **构建缓存**：改了依赖清单后仍怀疑命中旧缓存时，用 `docker build --no-cache ...` 重建。
- **静态目录配错**：`STATIC_DIR` 读不到 `index.html` 会**启动失败**（这是特性，不是 bug）。
- **拉不到镜像**：确认 tag 写对（`docker manifest inspect kzzhr/datainsights:latest`）；发布镜像的 tag 与 GitHub Releases 一致。
- 更多公共坑见[排障文档](../developer-guide/troubleshooting.md)。
