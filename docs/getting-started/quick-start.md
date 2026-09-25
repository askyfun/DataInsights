# 快速开始（Quick Start）

> 最后更新：2026-09-26
> 目标：数分钟内从零跑到能画第一张图。默认读者是刚拿到仓库的人。

## 1. 前置条件

- Docker 和 Docker Compose（**推荐**，一条命令起全部，零外部依赖）；或
- 仅 Docker + 自备一个可访问的 PostgreSQL。

## 2. 一条命令跑起来（推荐）

前后端编译进**同一个镜像**，由**一个 Go 进程、一个端口**同时提供 API 与页面（没有 nginx）。

```bash
make docker-up
# 或
docker compose up -d --build
```

然后访问 <http://localhost:23352>。

- 所有环境变量都有默认值，**不需要 `.env` 文件即可启动**（`DATABASE_URL` 之外都具备默认值；`SECURITY_KEY` 留空会自动生成并持久化到数据库）。
- 首次构建约 5–10 分钟（要装前端依赖并编译 Go）。
- 停止 / 看日志：`make docker-down` / `make docker-logs`。

## 3. 直接用 Docker（自备 PostgreSQL）

```bash
# 构建镜像（构建上下文是仓库根目录）
docker build -t data-insights .

# 启动：DATABASE_URL 是唯一必填项
docker run -d -p 23352:23352 \
  -e DATABASE_URL=postgres://user:password@your-db-host:5432/dbname?sslmode=disable \
  --name data-insights data-insights

# 或通过 .env 文件传入（先 cp .env.example .env 再修改）
docker run -d -p 23352:23352 --env-file .env --name data-insights data-insights
```

启动后同样访问 <http://localhost:23352>。

## 4. 本地开发（可选）

装好依赖后，一条命令同时起前后端（前端 23351 + 后端 23352，均带热重载）：

```bash
make dev
```

> `make dev-backend` 依赖 `air`（热重载）；未安装时先 `go install github.com/air-verse/air@latest`。
> 逐步命令、工具链与数据库准备见[本地开发环境](../developer-guide/dev-setup.md)。

## 5. 连上你的第一个数据源 → 建数据集 → 拖图

### 5.1 新增数据源

进入「数据源」→ 新建，选择方言（PostgreSQL / MySQL / ClickHouse / StarRocks），填连接信息并测试连接。创建后可浏览该库的表 / 列与表数据。

### 5.2 创建数据集

在数据源之上新建数据集，二选一：

- **物理表**：直接选一张表；
- **自定义 SQL**：写一段查询 SQL 作为数据来源。

创建向导里为各列确认类型与角色（维度 / 指标）。列会被分配**稳定 id**，后续图表引用它。

### 5.3 拖字段配图

进入图表构建器：从字段库把字段拖进维度 / 指标槽位，**任一改动即时出图**（没有单独的"Run"）。可加过滤、排序，切换图型。内置 **13 种图型**：表格、透视表、KPI 卡、柱状图、折线图、面积图、组合图（双轴）、饼图、漏斗图、雷达图、散点图、直方图、箱线图。满意后保存，或生成分享链接。

> 各功能的完整用法与「为什么这样设计」见[功能指南](../user-guide/features.md)。

> ⚠️ 待补截图：本轮无图，以上各步骤的界面截图待后续补充。

## 6. 遇到问题

- 排障（外部开发者也会踩的坑）：[troubleshooting.md](../developer-guide/troubleshooting.md)
- 配置项字典：[configuration.md](../user-guide/configuration.md)
