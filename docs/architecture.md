# DataRay 架构文档

## 项目概述

Monorepo 结构，包含前端 (React/TypeScript) 和后端 (Go)。核心功能：数据源管理、数据集管理、拖拽式图表构建、分享功能。

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | React 18 + TypeScript + Ant Design 5.x + ECharts 5.x + Zustand 4.x + @dnd-kit + Vite 6 + Sentry |
| 后端 | Go 1.26 + Gin + bun ORM + PostgreSQL + Sentry |
| 部署 | Docker + docker-compose |

## 目录结构

```
.
├── frontend/                  # 前端项目
│   ├── src/
│   │   ├── api/              # API 调用 (axios)
│   │   ├── store/            # Zustand 状态管理
│   │   ├── pages/           # 页面组件
│   │   └── styles/           # 样式文件
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
├── backend/                   # 后端项目
│   ├── cmd/main.go           # 入口 + 路由注册
│   ├── internal/
│   │   ├── config/           # 配置加载 (TOML)
│   │   ├── database/         # 数据库连接与事务抽象（WithTx）
│   │   ├── crypto/           # AES-GCM 加解密（DATARAY_SECURITY_KEY）
│   │   ├── domain/entity/    # 领域实体类型
│   │   ├── query/            # SQL 构造唯一出口（AST + bun_builder + raw.go）
│   │   ├── datasource/       # 数据源驱动抽象
│   │   │   ├── driver.go     # Driver 接口（Execute(ctx, sql, args...) 参数化执行）
│   │   │   ├── postgresql.go
│   │   │   ├── mysql.go
│   │   │   ├── clickhouse.go
│   │   │   └── starrocks.go
│   │   └── model/            # 数据模型
│   ├── migrations/           # goose 版本化迁移（embed.FS 内嵌）
│   ├── etc/config.toml       # 配置文件
│   └── go.mod
├── Makefile
└── docker-compose.yml
```

## 关键约束

1. **配置文件**: 后端使用 TOML 格式 (`etc/config.toml`)
2. **CORS**: 后端配置 CORS 中间件允许跨域
3. **API 基础URL**: 前端默认连接 `http://localhost:8080`，修改 `frontend/src/lib/api/client.ts`
4. **前端端口**: Vite 默认 3000
5. **热重载**: 后端开发使用 `air` 工具 (`make dev`)


## 图表查询与可视化语义分层

图表构建相关能力后续按以下职责分层演进：

### Chart 语义层

负责定义和解释图表语义，包括：

- 图表类型
- 维度组 / 指标组
- 组展示名，例如“维度”“行”“列”“主轴”“次轴”
- 图表样式配置
- 图表特有查询配置，例如饼图“其他”合并

目标结构包括：

- `ChartDefinition`：定义某类图表需要什么信息
- `ChartSpec`：定义某个图表实例绑定了什么信息

### Query 语义层

负责定义和执行查询语义，包括：

- 结构化维度表达式
- 结构化指标表达式
- 过滤组
- 排序 / 分页 / limit
- 时间粒度
- 分桶
- QueryAST 和 SQL 生成

目标结构包括：

- `QuerySpec`：查询模块需要查什么
- `QueryPlanner`：将 QuerySpec 转换为 QueryAST
- `QueryAST`：数据库无关的查询中间表示

## 查询链路（Batch 1 收敛后）

Batch 1 完成查询通道收敛后，图表/数据集查询只有一个通道：

```
handler → service → query 包（QueryAST + bun_builder / raw.go）→ datasource 驱动 Connection.Execute(ctx, sql, args...) 参数化执行
```

要点：

- `query` 包是 SQL 构造的唯一出口。结构化查询走 `QueryPlanner → QueryAST → bun_builder`（bun_builder 同时收集值参数与构造 SQL）；原始 SQL（表预览、字段分布等）走 `raw.go` 构造。
- `Connection.Execute(ctx, sql string, args ...any)` 支持参数化执行，值参数一律通过 args 传递，禁止字符串拼接进 SQL。
- 标识符一律通过白名单校验：裸名用 `datasource.IsValidIdentifier`；query 包 `safeIdentifier` 额外允许成对引号包裹的标识符。
- 旧的 `query/builder.go` 与平铺参数兼容链路已删除；Batch 2 进一步删除 `query/dialect.go` 中手写字符串 SQL builder 死路径（`SQLBuilder`/`baseSQLBuilder`/`BuildQueryString`/各方言 builder），`dialect.go` 仅保留 `DialectType`/`ParseDialect`/`BuildQueryStringWithBun`；聚合表达式 `aggExprPattern` 收紧为显式函数白名单（`count|sum|avg|min|max`），堵截经列 `FieldExpr` 注入任意函数名。
- 数据库 schema 由 goose 版本化迁移管理（`backend/migrations/`，embed.FS 内嵌），事务统一走 `database.WithTx`。

当前落地状态：

- 对外接口仍使用旧协议：`chart_type` + `dims` + `metrics` + `filters` + `pagination` + `sort`。
- `backend/internal/service/chart/impl.go` 将旧请求转换为内部 `QuerySpec`，再经 `QueryPlanner` 生成 `PlannedAST`，由 `query.Executor` 消费 AST 生成参数化 SQL。
- `QueryAST` 第一阶段已支持结构化维度/指标元信息、`limit`，以及日期维度 `day` 粒度的 PostgreSQL / MySQL / ClickHouse 方言 SQL 生成。
- 分桶、复杂过滤组、更多时间粒度仍属于后续阶段。

## 方言能力矩阵（Task 3-0 探针）

`datasource.Connection.Capabilities(ctx)` 返回 `DialectCapabilities`（GROUPING SETS / 百分位 / 窗口函数支持 + 百分位策略），上层（如 `query.Executor` 的 pivot 路径）据此选择 SQL 路径；能力不足时显式报错，不静默降级。探测为懒加载：PostgreSQL 首次调用时对真实库跑轻量只读探针 SQL 并在连接生命周期内缓存（`sync.Once`）；其余方言本机无实例，返回诚实保守的静态值。

| 方言 | GROUPING SETS | 百分位策略 | 窗口函数 | 验证状态 |
|------|--------------|-----------|---------|---------|
| PostgreSQL | ✓ | `percentile_cont` | ✓ | **实测验证**（dev PG 18.6，真实探针 + `capabilities_integration_test.go`） |
| ClickHouse | ✓（文档支持，CH 21.x+） | `unsupported`（保守） | ✓（文档支持） | grouping-sets/window 文档支持但**本会话无 CH 实例未运行验证**（失败模式为显式 SQL 报错）；百分位保守 unsupported（silent-wrong-data 风险），待实例探针验证 `quantilesExactInclusive` 后翻转 |
| MySQL | ✗（无 GROUPING SETS，仅 WITH ROLLUP） | `unsupported`（保守） | 待探针（8+ 支持） | **保守默认，未实测（无实例）**；探针 SQL 见驱动 TODO |
| StarRocks | ✗（保守，独立于 MySQL） | `unsupported`（保守） | 待探针 | **保守默认，未实测（无实例）**；独立探针不继承 MySQL 结果 |

待有 MySQL/ClickHouse/StarRocks 真实实例时，按各驱动 `Capabilities()` TODO 注释中的探针 SQL 实测后翻转保守默认。

## 契约与路由（Batch 2）

- **契约单一事实源**：`api/openapi.yaml` 定义全部端点请求/响应 + `Envelope`（`code/msg/trace/data`）+ `ChartSpec`/`QuerySpec`；`make api-gen` 生成 `backend/internal/idls/gen_types.go`（oapi-codegen）与 `frontend/src/idls/gen_types.ts`（openapi-typescript）。当前生成物作为契约与校验基线；运行时类型尚未全量切换到生成物（列入 Batch 3）。
- **泛型路由**：`backend/internal/router/router.go` 的 `API[In,Out] func(req Request[In], res *Response[Out]) error`（`res` 必须是指针，值传递会丢弃 handler 写入）已接入全部 31 个 API 端点。路由器按 HTTP 方法绑定 JSON body（POST/PUT/PATCH）+ query 参数并统一信封；`cmd/routes.go` 用 `Register{Get,Post,Put,Delete}Route` 注册，迁移样板见 `handler/datasource.go` 顶部 package doc。例外：`/health`（cmd/main.go）与 share `View`（302 重定向无法套 JSON 信封）。
- **图表配置 v1**：`bi_chart.config` 为 `{version:1, chartType, title, query:{dimensionGroups,metricGroups,filters,sort,limit}, fieldMeta, style, queryOptions}` 文档；`frontend/src/lib/chartConfigSchema.ts` 的 `migrateChartConfig` 在加载时把旧结构（`queryConfig` + 5 个平铺 Record + 位置 `field-N` + 恒为 null 的 `xAxisField`）迁移到 v1，`fieldId` 改用稳定列名，ShareView 据此正常渲染。

### 响应层

负责将查询结果包装成统一响应 Envelope：

- `shape`
- `data`
- `meta`
- `fields`
- `sql`

兼容期内保留当前旧协议，同时逐步将前后端收敛到统一结构。
