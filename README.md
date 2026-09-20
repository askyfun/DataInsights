# Data Insights

拖拽式 BI 可视化分析平台 MVP

## 功能特性

- **数据源管理** — 支持 PostgreSQL、MySQL、ClickHouse、StarRocks 四种数据库连接配置与连接测试
- **数据集管理** — 支持直接查询表模式或自定义 SQL 模式
- **拖拽式图表构建** — 通过拖拽字段快速创建可视化图表，支持 13 种图表类型：
  - 表格、透视表、KPI 卡
  - 柱状图、折线图、面积图、组合图（双轴）
  - 饼图、漏斗图、雷达图
  - 散点图、直方图、箱线图
- **分享功能** — 生成分享链接，支持密码保护

## 技术栈

### 前端
- React 19 + TypeScript
- Ant Design 6.x
- ECharts 6.x
- Zustand 5.x
- @dnd-kit 拖拽交互
- Vite 构建工具

### 后端
- Go 1.27+
- Gin API 框架
- uptrace/bun ORM
- PostgreSQL 12+

## 快速开始

### 前置条件

- Docker 和 Docker Compose（方式一），或仅 Docker（方式二需自备 PostgreSQL）

### 使用 Docker（单镜像单进程）

前后端编译进**同一个镜像**，由**一个 Go 进程、一个端口**同时提供 API 与页面（没有 nginx）。首次构建约 5-10 分钟。

**方式一：Docker Compose（推荐，零外部依赖）**

自带 PostgreSQL 容器，一条命令跑起来：

```bash
make docker-up
# 或
docker compose up -d --build
```

> 所有环境变量都有默认值，不需要 `.env` 文件即可启动（`DATABASE_URL` 之外都具备默认值；`SECURITY_KEY` 留空会自动生成并持久化到数据库）。

**方式二：直接用 Docker（需要自备可访问的 PostgreSQL）**

```bash
# 构建镜像
docker build -t data-insights .

# 启动（DATABASE_URL 是唯一必填项）
docker run -d -p 23352:23352 \
  -e DATABASE_URL=postgres://user:password@your-db-host:5432/dbname?sslmode=disable \
  --name data-insights data-insights

# 或通过 .env 文件传入（先 cp .env.example .env 再修改）
docker run -d -p 23352:23352 --env-file .env --name data-insights data-insights
```

服务启动后访问 http://localhost:23352

### 本地开发

#### 后端

```bash
make install-backend   # 安装 Go 依赖
make dev-backend       # 开发模式（热重载，需要 air）
make build-backend     # 构建二进制
```

> `make dev-backend` 依赖 `air`。若未安装，可先执行 `go install github.com/cosmtrek/air@latest`。
> 想直接调 go 命令的，见 [docs/setup.md](docs/setup.md)。

#### 前端

```bash
make install-frontend   # 安装依赖（只允许 pnpm，npm/yarn 会被拦截）
make dev-frontend       # 开发服务器
```

> 依赖**只允许用 pnpm** 安装。npm / yarn 会被 `preinstall` 钩子拦截并中止；若本机没有 pnpm，先执行 `npm install -g pnpm`。详见 [AGENTS.md](AGENTS.md)。

## 配置

前后端共用仓库根目录的**一份**环境变量文件，模板见 `.env.example`：

```bash
cp .env.example .env    # 然后按需修改
```

**优先级（低 → 高）：内置默认值 < `.env` 文件 < 真实系统环境变量。**

- `.env` **不入库**（已在 `.gitignore`）；真实值只在本机或被部署环境注入。常用项只有两个：`DATABASE_URL`（必填）、`SECURITY_KEY`（留空也安全，见下）；其余（CORS 白名单、Sentry、静态目录、分开部署地址）都有合理默认值，见 `.env.example` 进阶注释。
- 后端变量**无前缀**（单进程单体不需要命名空间，且 `DATABASE_URL` / `PORT` 是 12-factor 标准名，云平台会自动注入）：`PORT`、`DATABASE_URL`、`SECURITY_KEY`、`SENTRY_DSN`、`CORS_ALLOWED_ORIGINS`（留空 = 放开所有来源）、`STATIC_DIR`（前端产物目录；留空 = 只提供 API，页面由别的进程提供）。监听地址固定 `0.0.0.0`，不提供 env 覆盖。
- `SECURITY_KEY` 用于加密数据源密码（AES-256-GCM）。**留空 = 首次启动自动随机生成并持久化到数据库 `bi_setting` 表**，此后重启/容器重建复用同一把，已加密密码始终可解；显式配置时环境变量优先。
- 前端变量（`VITE_` 前缀）：`VITE_SENTRY_DSN`；`VITE_API_BASE_URL` 仅前后端分开部署时才需要填。⚠️ 这类变量在**构建期**被内联进 JS，改完必须重新 build，且**不能放密钥**。
- 单容器/容器编排部署时，可用 `docker run --env-file .env ...` 或编排平台的 env 注入。

**没有配置文件。** 后端不再读取任何 TOML/YAML —— 所有需要配置的值都在环境变量里，`DATABASE_URL` 缺失时启动会直接报错退出并告诉你缺哪个变量。这样单镜像容器只要 `--env-file .env` 就能跑，不用往镜像里塞或往容器里挂文件。

### 一个进程怎么同时服务页面和 API

Go 进程按路径分流，替代了原先 nginx 的角色：

| 路径 | 去处 |
|------|------|
| `/api`、`/mcp`、`/health` 及其子路径 | 后端内部路由；没有匹配到就是 JSON 404（不会回落成 HTML） |
| 其余路径 | `STATIC_DIR` 里的文件；没有对应文件则回落 `index.html`（交给前端路由） |

`assets/` 下带内容哈希的产物走 `immutable` 长缓存，其余文件 `no-cache`。静态目录配错（读不到 `index.html`）会让进程**启动即失败**，不会跑起来之后整站 404。

## API 文档

| 接口 | 方法 | 说明 |
|------|------|------|
| /api/datasources | GET/POST | 数据源列表/创建 |
| /api/datasources/:id | GET/DELETE | 数据源详情/删除 |
| /api/datasources/test | POST | 测试连接 |
| /api/datasources/:id/tables | GET | 获取数据表列表 |
| /api/datasources/:id/tables/:table/columns | GET | 获取表字段列表 |
| /api/datasets | GET/POST | 数据集列表/创建 |
| /api/datasets/:id | GET/DELETE | 数据集详情/删除 |
| /api/datasets/:id/columns | GET | 获取字段列表 |
| /api/charts | GET/POST | 图表列表/创建 |
| /api/charts/:id | GET/PUT/DELETE | 图表 CRUD |
| /api/charts/:id/data | GET | 获取图表数据 |
| /api/shares | POST | 创建分享 |
| /api/shares/:token | GET | 获取分享信息 |
| /share/:token | GET | 访问分享链接 |
| /health | GET | 后端健康检查（统一响应格式） |

## 项目结构

```
.
├── Dockerfile                # 单镜像：前端产物 + Go 二进制，一个进程一个端口
├── docker-compose.yml        # PostgreSQL + app（零外部依赖一键启动）
├── frontend/                 # 前端项目（React 19 + TypeScript + Vite）
│   └── src/
│       ├── api/             # API 客户端与类型
│       ├── store/           # Zustand 状态管理
│       ├── pages/           # 页面组件
│       └── components/      # 可复用组件（图表构建器、拖拽交互等）
└── backend/                  # 后端项目（Go 1.27 + Gin + bun ORM）
    ├── cmd/                 # 入口与路由注册
    └── internal/
        ├── config/          # 环境变量加载
        ├── handler/         # HTTP 处理器
        ├── service/         # 业务逻辑
        ├── query/           # SQL 构造（AST + bun_builder）
        ├── datasource/      # 多数据库驱动抽象
        └── model/           # bun ORM 模型
```

## License

MIT
