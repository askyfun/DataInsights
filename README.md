# Data Insights

拖拽式 BI 可视化分析平台 —— 连上数据源、建好数据集，把字段拖进槽位即时出图，并能一键分享、拼装仪表盘。Go 单二进制直连 PostgreSQL / MySQL / ClickHouse / StarRocks，前端 React 19 + ECharts。

## 功能特性

- **数据源管理** — PostgreSQL / MySQL / ClickHouse / StarRocks 四种数据库连接配置与连接测试
- **数据集管理** — 物理表或自定义 SQL 两种模式，列为维度 / 指标并带稳定 id
- **拖拽式图表构建** — 拖字段即时出图，内置 **13 种图表类型**（表格 / 透视 / KPI / 柱 / 折线 / 面积 / 组合双轴 / 饼 / 漏斗 / 雷达 / 散点 / 直方图 / 箱线）
- **分享** — 免登录只读链接，支持密码保护与过期
- **仪表盘 v1** — 12 列栅格布局，图表块 / 文本块 / 盘级筛选器，一次请求多块联动取数
- **查询记录** — 每次查询落库（内容 hash 去重）并生成短码直链，"地址栏即分享"

> 完整功能说明与「为什么这样设计」见 [docs/user-guide/features.md](docs/user-guide/features.md)。

## 技术栈

- **前端**：React 19 + TypeScript + Ant Design 6 + ECharts 6 + Zustand + Vite（Biome 校验、Vitest 测试）
- **后端**：Go 1.27 + Gin + bun ORM + PostgreSQL，goose 版本化迁移、Sentry 上报
- **契约**：`api/openapi.yaml` 是接口单一事实源，`make api-gen` 生成 Go / TypeScript 双端类型
- **部署**：Docker 单镜像单进程 —— 一个容器、一个端口（23352）同时提供 API 与页面

> 分层架构、目录结构与关键设计决策见 [docs/developer-guide/architecture.md](docs/developer-guide/architecture.md)。

## 快速开始

前后端编译进**同一个镜像**，由一个 Go 进程、一个端口同时提供 API 与页面（没有 nginx），首次构建约 5–10 分钟。

```bash
make docker-up      # = docker compose up -d --build（自带 PostgreSQL，零外部依赖）
```

启动后访问 <http://localhost:23352>。

> 所有环境变量都有默认值，**无需 `.env` 即可启动**。也可直接用 Docker（自备 PostgreSQL，`DATABASE_URL` 是唯一必填项）；两种方式的完整步骤与"连上第一个数据源 → 建数据集 → 拖图"见 [docs/getting-started/quick-start.md](docs/getting-started/quick-start.md)。

本地开发（`make dev`）、逐步命令与工具链准备见 [docs/developer-guide/dev-setup.md](docs/developer-guide/dev-setup.md)。

## 文档导航

| 区域 | 内容 |
|------|------|
| [docs/getting-started/](docs/getting-started/) | 概览（核心概念 / 应用场景 / 对外路线）、快速开始 |
| [docs/user-guide/](docs/user-guide/) | 配置与参数字典、功能指南 |
| [docs/deployment/](docs/deployment/) | 容器部署（Docker / Compose）、生产部署实践 |
| [docs/developer-guide/](docs/developer-guide/) | 架构与设计决策、API 文档、图表查询设计、排障、开发环境、贡献规范 |

- 配置项字典：[docs/user-guide/configuration.md](docs/user-guide/configuration.md)
- API 端点说明（契约事实源是 [`api/openapi.yaml`](api/openapi.yaml)）：[docs/developer-guide/api.md](docs/developer-guide/api.md)
- 遇到问题：[docs/developer-guide/troubleshooting.md](docs/developer-guide/troubleshooting.md)

## License

GNU General Public License v3.0 —— 见 [`LICENSE`](LICENSE)。
