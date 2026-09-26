# Data Insights

拖拽式 BI 可视化分析平台 —— 连上数据源、建好数据集，把字段拖进槽位即时出图，并能一键分享、拼装仪表盘。

## 功能特性

- **数据源管理** — PostgreSQL / MySQL / ClickHouse / StarRocks 四种数据库连接配置与连接测试
- **数据集管理** — 物理表或自定义 SQL 两种模式，列为维度 / 指标并带稳定 id
- **拖拽式图表构建** — 拖字段即时出图，内置 **13 种图表类型**（表格 / 透视 / KPI / 柱 / 折线 / 面积 / 组合双轴 / 饼 / 漏斗 / 雷达 / 散点 / 直方图 / 箱线）
- **分享** — 免登录只读链接，支持密码保护与过期
- **仪表盘 v1** — 12 列栅格布局，图表块 / 文本块 / 盘级筛选器，一次请求多块联动取数
- **查询记录** — 每次查询落库（内容 hash 去重）并生成短码直链，"地址栏即分享"

> 完整功能说明与「为什么这样设计」见 [docs/user-guide/features.md](docs/user-guide/features.md)。

## 快速开始

四种方式，从「只想看看」到「自己动手改」，挑一种即可。**还没装 PostgreSQL、只想最快看到界面 → 直接看方式三。** 跑起来后都访问 <http://localhost:23352>（方式四的开发模式是前端 23351）。

> ⚠️ **部署前必读**：本项目**无登录、无鉴权**，定位是单团队**内网**自助分析工具 —— **不要暴露到公网**。任何能访问 23352 的人都能读写全部数据源与数据集。部署到他人可达的主机上之前，至少改掉数据库密码、别把 PostgreSQL 端口映射出来，其余见[生产部署](docs/deployment/production.md)。

> 方式二、方式三用的是[发布流水线](docs/deployment/release.md)产出的镜像（**版本号以 [Releases](https://github.com/askyfun/DataInsights/releases) 页为准**）；方式四从源码构建，不依赖镜像。

### 方式一：在线预览站

> 🚧 即将提供，先跳过。

### 方式二：Docker 一条命令（自备 PostgreSQL）

前后端编译进**同一个镜像**，由一个进程、一个端口同时提供 API 与页面（没有 nginx）。直接拉现成镜像，不用克隆仓库、不用构建：

```bash
docker run -d --name data-insights -p 23352:23352 \
  -e DATABASE_URL=postgres://user:password@your-db-host:5432/dbname?sslmode=disable \
  kzzhr/datainsights:latest
```

- **`DATABASE_URL` 是唯一必填项**（一个可访问的 PostgreSQL），其余全部有默认值。
- ⚠️ 上面的 `user:password@your-db-host` 是**占位串**：它非空，所以能通过启动检查，但容器连不上库 —— 会「起来了却打不开数据源」。请换成你自己的连接串。
- 生产环境把 `latest` 换成具体版本号（`kzzhr/datainsights:vX.Y.Z`）钉住，别跟着 `latest` 漂。
- 镜像支持 `linux/amd64` 与 `linux/arm64`（Apple Silicon 原生跑，无需模拟）。

### 方式三：Docker Compose 一键起全套（含 PostgreSQL）

一条命令把 Data Insights 与 PostgreSQL 一起拉起来，不用自己准备数据库：

```bash
curl -fsSLO https://raw.githubusercontent.com/askyfun/DataInsights/master/docker-compose.hub.yml
docker compose -f docker-compose.hub.yml up -d
```

- 默认用 `latest`；钉版本：`TAG=vX.Y.Z docker compose -f docker-compose.hub.yml up -d`。
- 数据落在命名卷 `postgres_data` 里，`down` 不会删，加 `-v` 才清空。
- 要用本机客户端连这个数据库、想换端口、想改数据库密码：都在这份文件的注释里标了位置。

### 方式四：从源码运行

```bash
git clone https://github.com/askyfun/DataInsights.git
cd DataInsights

make docker-up        # = docker compose up -d --build，从源码构建镜像（首次约 5–10 分钟）
make docker-down      # 停止
make docker-logs      # 看日志
```

日常改代码用开发模式（前后端热重载）：

```bash
make install          # 装前后端依赖（只用 pnpm）
make dev              # 前端 23351 + 后端 23352，一起起
```

- 前置：**pnpm**、**`air`**（未装先 `go install github.com/air-verse/air@latest`）、一个**可访问的 PostgreSQL**（不想另外装就用 `docker compose up -d postgres` 只起仓库自带的那个）。`DATABASE_URL` 缺失时后端会直接退出。
- 上面 `make docker-up` 用的是仓库根的 [`docker-compose.yml`](docker-compose.yml)，**从源码构建**；方式三那份 `docker-compose.hub.yml` 是**拉现成镜像**，别混。
- 常用命令一览：`make help`。想连带一个现成的 StarRocks 试用，用 [docker-compose.allinone.yml](docker-compose.allinone.yml)。

> 四种方式的完整步骤与「连上第一个数据源 → 建数据集 → 拖图」：[docs/getting-started/quick-start.md](docs/getting-started/quick-start.md)
> 本地环境准备（工具链、数据库、逐条命令）：[docs/developer-guide/dev-setup.md](docs/developer-guide/dev-setup.md)

## 文档导航

| 区域 | 内容 |
|------|------|
| [docs/getting-started/](docs/getting-started/) | 概览（核心概念 / 应用场景 / 对外路线）、快速开始 |
| [docs/user-guide/](docs/user-guide/) | 配置与参数字典、功能指南 |
| [docs/deployment/](docs/deployment/) | 容器部署（Docker / Compose）、发布流程、生产部署实践 |
| [docs/developer-guide/](docs/developer-guide/) | 架构与设计决策、API 文档、图表查询设计、排障、开发环境、贡献规范 |

- 配置项字典：[docs/user-guide/configuration.md](docs/user-guide/configuration.md)
- API 端点说明（契约事实源是 [`api/openapi.yaml`](api/openapi.yaml)）：[docs/developer-guide/api.md](docs/developer-guide/api.md)
- 准备放到真实环境用：[docs/deployment/production.md](docs/deployment/production.md)
- 遇到问题：[docs/developer-guide/troubleshooting.md](docs/developer-guide/troubleshooting.md)

## 贡献

欢迎 Issue 与 PR。动手前先看 [`CONTRIBUTING.md`](CONTRIBUTING.md)，三条硬约束先记住：

- **包管理器只用 pnpm** —— 根目录与 `frontend/` 的 `preinstall` 钩子会直接拒绝 npm / yarn / bun。
- **提交前必跑门禁** —— 后端 `go test -race ./...`，前端 `pnpm build:check`。
- **接口契约的唯一事实源是 [`api/openapi.yaml`](api/openapi.yaml)** —— 改接口先改它，再跑 `make api-gen` 同步双端类型。

完整流程与编码规范见 [`docs/developer-guide/contributing.md`](docs/developer-guide/contributing.md)；**发布新版本**（打 tag + 推镜像到 Docker Hub）见 [`docs/deployment/release.md`](docs/deployment/release.md)。

### 技术栈

- **前端**：React 19 + TypeScript + Ant Design 6 + ECharts 6 + Zustand + Vite（Biome 校验、Vitest 测试）
- **后端**：Go 1.27 + Gin + bun ORM + PostgreSQL，goose 版本化迁移、Sentry 上报
- **契约**：`api/openapi.yaml` 是接口单一事实源，`make api-gen` 生成 Go / TypeScript 双端类型
- **部署**：Docker 单镜像单进程 —— 一个容器、一个端口（23352）同时提供 API 与页面

> 分层架构、目录结构与关键设计决策见 [docs/developer-guide/architecture.md](docs/developer-guide/architecture.md)。

## License

GNU General Public License v3.0 —— 见 [`LICENSE`](LICENSE)。
