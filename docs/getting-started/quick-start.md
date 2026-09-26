# 快速开始（Quick Start）

> 最后更新：2026-09-26
> 目标：数分钟内从零跑到能画第一张图。默认读者是刚拿到这个项目的人。
> 四条路径对应的命令在 [README](../../README.md#快速开始) 有浓缩版，本文给完整步骤。

## 1. 前置条件

| 路径 | 需要什么 |
|------|----------|
| 方式二：单条 Docker 命令 | Docker + 一个可访问的 PostgreSQL |
| 方式三：Docker Compose | 只要 Docker（含 Compose v2），零外部依赖 |
| 方式四：从源码运行 | Docker；或本地开发所需的 Go 1.27 / Node 24 / pnpm / `air` + 一个可访问的 PostgreSQL（见[环境搭建](../developer-guide/dev-setup.md)） |

⚠️ 本项目**无登录、无鉴权**，定位是单团队**内网**工具 —— 不要暴露到公网；放到真实环境前先看[生产部署](../deployment/production.md)。

前后端编译进**同一个镜像**，由**一个 Go 进程、一个端口（23352）**同时提供 API 与页面（没有 nginx）。

## 2. 方式一：在线预览站

> 🚧 即将提供，先跳过。对外路线见[概览](overview.md)。

## 3. 方式二：Docker 一条命令（最快）

从 Docker Hub 拉取已发布镜像，不需要克隆仓库、不需要构建：

```bash
docker run -d --name data-insights -p 23352:23352 \
  -e DATABASE_URL=postgres://user:password@your-db-host:5432/dbname?sslmode=disable \
  kzzhr/datainsights:latest
```

- **`DATABASE_URL` 是唯一必填项**，其余环境变量都有默认值；`SECURITY_KEY` 留空会由系统首次启动时随机生成并持久化到数据库。
- ⚠️ 命令里的 `user:password@your-db-host` 是**占位串**。它非空，所以能通过「缺 `DATABASE_URL` 就退出」的启动检查，但容器连不上库 —— 症状是「起来了，但建数据源时连不通」。请换成你自己的连接串（参数含义见[配置项字典](../user-guide/configuration.md)）。
- 镜像 tag 与 [GitHub Releases](https://github.com/askyfun/DataInsights/releases) 的版本号一一对应：从 Releases 页挑一个版本号，把 `kzzhr/datainsights:vX.Y.Z` 里的 `X.Y.Z` 换成它。生产环境钉住版本，不要跟着 `latest` 漂。
- 镜像支持 `linux/amd64` 与 `linux/arm64`；`docker pull kzzhr/datainsights:latest` 可先单独拉取。
- 启动后访问 <http://localhost:23352>，看日志：`docker logs -f data-insights`。

想用 `.env` 文件传参（需要在本地先有仓库：`cp .env.example .env` 再按需修改）：

```bash
docker run -d -p 23352:23352 --env-file .env --name data-insights kzzhr/datainsights:latest
```

## 4. 方式三：Docker Compose 一键起全套

[`docker-compose.hub.yml`](../../docker-compose.hub.yml) 把 Data Insights 与 PostgreSQL 一起拉起来，零外部依赖：

```bash
curl -fsSLO https://raw.githubusercontent.com/askyfun/DataInsights/master/docker-compose.hub.yml
docker compose -f docker-compose.hub.yml up -d
```

若本地已有仓库，直接 `docker compose -f docker-compose.hub.yml up -d` 即可（不必再下载）。

- 默认用 `latest`，钉版本：`TAG=vX.Y.Z docker compose -f docker-compose.hub.yml up -d`（版本号见 [Releases](https://github.com/askyfun/DataInsights/releases)）。
- 数据存在命名卷 `postgres_data` 里：`down` 保留数据，`down -v` 才清空。
- 变量覆盖优先级（高 → 低）：shell 环境变量 > 同目录 `.env` > 文件内默认值。
- ⚠️ 库里用的是公开的演示密码 `insights123`，且 PostgreSQL **默认不映射到宿主机**。要用本机客户端连进去看表、换端口（`PORT`）、改数据库密码，都在该文件注释里标了位置。

## 5. 方式四：从源码运行

```bash
git clone https://github.com/askyfun/DataInsights.git
cd DataInsights

make docker-up      # = docker compose up -d --build（首次构建约 5–10 分钟）
make docker-down    # 停止
make docker-logs    # 看日志
```

- 这条路径用的是仓库根的 [`docker-compose.yml`](../../docker-compose.yml)：从**源码构建**，不是拉镜像。
- 想连带一个现成的 StarRocks 试用，用 [`docker-compose.allinone.yml`](../../docker-compose.allinone.yml)：`docker compose -f docker-compose.allinone.yml up --build`。
- 只用 Dockerfile 手工构建与运行（自备 PostgreSQL，见[容器部署](../deployment/docker.md)）：

```bash
docker build -t data-insights .   # 构建上下文 = 仓库根目录
docker run -d -p 23352:23352 \
  -e DATABASE_URL=postgres://user:password@your-db-host:5432/dbname?sslmode=disable \
  --name data-insights data-insights
```

日常改代码走开发模式（前端 23351 + 后端 23352，均带热重载）：

```bash
make install
make dev
```

> `make dev` 会先检查 23351 / 23352 是否被占用并快速失败；`make dev-backend` 依赖 `air`（未安装时先 `go install github.com/air-verse/air@latest`）。`DATABASE_URL` 没配时后端会直接退出（连带把前端一起带走），没有现成 PostgreSQL 就 `docker compose up -d postgres` 只起仓库自带的那个。
> 上面 `make docker-up` 用的是仓库根的 [`docker-compose.yml`](../../docker-compose.yml)（**从源码构建**）；方式三那份 `docker-compose.hub.yml` 是**拉现成镜像**，别混。
> 逐步命令、工具链与数据库准备见[本地开发环境](../developer-guide/dev-setup.md)，全部命令见 `make help`。

## 6. 连上你的第一个数据源 → 建数据集 → 拖图

### 6.1 新增数据源

进入「数据源」→ 新建，选择方言（PostgreSQL / MySQL / ClickHouse / StarRocks），填连接信息并测试连接。创建后可浏览该库的表 / 列与表数据。

### 6.2 创建数据集

在数据源之上新建数据集，二选一：

- **物理表**：直接选一张表；
- **自定义 SQL**：写一段查询 SQL 作为数据来源。

创建向导里为各列确认类型与角色（维度 / 指标）。列会被分配**稳定 id**，后续图表引用它。

### 6.3 拖字段配图

进入图表构建器：从字段库把字段拖进维度 / 指标槽位，**任一改动即时出图**（没有单独的"Run"）。可加过滤、排序，切换图型。内置 **13 种图型**：表格、透视表、KPI 卡、柱状图、折线图、面积图、组合图（双轴）、饼图、漏斗图、雷达图、散点图、直方图、箱线图。满意后保存，或生成分享链接。

> 各功能的完整用法与「为什么这样设计」见[功能指南](../user-guide/features.md)。

> ⚠️ 待补截图：本轮无图，以上各步骤的界面截图待后续补充。

## 7. 遇到问题

- 排障（外部开发者也会踩的坑）：[troubleshooting.md](../developer-guide/troubleshooting.md)
- 配置项字典：[configuration.md](../user-guide/configuration.md)
- 部署到真实环境：[生产部署](../deployment/production.md)
- 镜像拉取 / 版本对应：[容器部署](../deployment/docker.md) · [发布流程](../deployment/release.md)
