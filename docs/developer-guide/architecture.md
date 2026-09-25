# Data Insights 架构文档

> 最后更新：2026-09-26
> 图表查询链路的完整设计（契约模型、SQL 生成、处理器分派台账）见同目录的 `chart-query-design.md`。

## 项目概述

Monorepo 结构，包含前端 (React/TypeScript) 和后端 (Go)。核心功能：数据源管理、数据集管理、拖拽式图表构建、分享功能。

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | React 19 + TypeScript + Ant Design 6.x + ECharts 6.x + Zustand 5.x + @dnd-kit + react-intl + react-grid-layout + Vite 8 + Biome（lint/格式化）+ Vitest（测试）+ Sentry |
| 后端 | Go 1.27 + Gin + bun ORM + PostgreSQL + Sentry |
| 部署 | Docker + docker-compose |

## 目录结构

```
.
├── frontend/                  # 前端项目
│   ├── src/
│   │   ├── api/              # API 端点封装（复用 lib/api/client 的单一 axios 实例）
│   │   ├── components/       # 可复用组件（DateFilter/、ChartBuilder/、DashboardFilterBlock/ 等）
│   │   ├── lib/              # 工具库（api/client、chartConfigSchema、dateFilter、dashboardLayoutSchema 等语义单一事实源）
│   │   ├── idls/             # API 类型定义（gen_types.ts，由 make api-gen 生成）
│   │   ├── store/            # Zustand 状态管理
│   │   ├── pages/           # 页面组件
│   │   ├── i18n/             # 国际化
│   │   └── styles/           # 样式文件
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
├── backend/                   # 后端项目
│   ├── cmd/main.go           # 入口 + 中间件装配（requestID / CORS / Sentry）+ 路由注册
│   ├── internal/
│   │   ├── config/           # 配置加载（纯环境变量 + godotenv 读 .env；无配置文件）
│   │   ├── webui/            # 前端产物托管与路径分流（挂 gin NoRoute）
│   │   ├── handler/          # Gin HTTP 处理器（请求绑定、响应格式化）
│   │   ├── router/           # 泛型路由注册 Register{Get,Post,Put,Delete}Route[In,Out]
│   │   ├── response/         # 统一响应信封（code/msg/trace/data）
│   │   ├── idls/             # gen_types.go（oapi-codegen 从 api/openapi.yaml 生成）
│   │   ├── service/          # 业务逻辑（按领域子包：chart, dashboard, dataset, datasource, share, queryrecord）
│   │   ├── domain/entity/    # 领域实体类型
│   │   ├── query/            # SQL 构造唯一出口（AST + bun_builder + raw.go）
│   │   │   ├── ast.go / planner.go  # QueryAST + QuerySpec → PlannedAST
│   │   │   ├── bun_builder.go / raw.go / dialect.go  # 参数化 SQL / 原始 SQL / 方言适配
│   │   │   ├── chart_spec.go / types.go  # 图表规格与类型定义
│   │   │   ├── datefilter.go  # 前端日期语义的 Go 侧镜像（双端共读用例表防漂移）
│   │   │   └── executor.go / processor*.go  # 执行 + 结果处理（Table/Pie/Axis/Scatter/Pivot/Stats）
│   │   ├── keystore/         # SECURITY_KEY 解析与持久化（留空则首次随机生成、落 bi_setting 表）
│   │   ├── crypto/           # AES-GCM 加解密（密钥来自 keystore，即 SECURITY_KEY）
│   │   ├── migration/        # 数据迁移（如 columnids 列 id 规范化）
│   │   ├── idcodec/ idgen/   # 列/分享 id 的 base58 编解码与生成
│   │   ├── database/         # 数据库连接与事务抽象（WithTx）
│   │   ├── datasource/       # 数据源驱动抽象
│   │   │   ├── driver.go     # Driver 接口（Execute(ctx, sql, args...) 参数化执行）
│   │   │   ├── postgresql.go
│   │   │   ├── mysql.go
│   │   │   ├── clickhouse.go
│   │   │   └── starrocks.go
│   │   └── model/            # 数据模型
│   ├── migrations/           # goose 版本化迁移（embed.FS 内嵌）
│   └── go.mod
├── Dockerfile                # 单镜像：前端产物 + Go 二进制（无 nginx，单进程单端口）
├── Makefile
└── docker-compose.yml
```

## 关键约束

1. **配置**: **只有环境变量**（无配置文件）。优先级 内置默认值 < 仓库根 `.env` 文件 < 真实系统环境变量；`DATABASE_URL` 必填，缺失即启动失败
2. **CORS**: 默认放开所有来源（平台 API 无登录态，CORS 不构成安全边界；同源部署本就不产生跨域请求）；`CORS_ALLOWED_ORIGINS` 填非空白名单可收紧
3. **API 基础URL**: 未配置 `VITE_API_BASE_URL` 时生产构建自动同源、开发回退 `http://<当前主机名>:23352`；配置非空绝对地址仅用于前后端分开部署；唯一来源是 `frontend/src/lib/api/client.ts`
4. **前端端口**: Vite 开发服务器默认 23351；生产是单镜像单端口 23352（Go 同时提供 API 与静态页面）
5. **静态托管**: `backend/internal/webui` 挂在 gin 的 `NoRoute` 上 —— 命中文件就返回，未命中回落 `index.html`，`/api`、`/mcp`、`/health` 保留前缀一律 JSON 404。`STATIC_DIR` 留空时后端退化为纯 API 服务
6. **热重载**: 后端开发使用 `air` 工具 (`make dev`)


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

- `query` 包是 SQL 构造的唯一出口。结构化查询走 `planner.go`（`QuerySpec → PlannedAST`）→ `bun_builder.go`（同时收集值参数与构造 SQL，含 `bun_builder_{pivot,boxplot,histogram}.go` 各图表路径）；原始 SQL（表预览、字段分布等）走 `raw.go` 构造。`ast.go` 定义 QueryAST、`chart_spec.go` 定义图表规格、`processor*.go` 处理结果（Table/Pie/Axis/Scatter/Pivot/Stats）、`types.go` 收敛类型。`datefilter.go` 是前端 `lib/dateFilter.ts` 日期语义的 Go 侧镜像（分享页/仪表盘由后端读 config 组查询时解析「最近 7 天」这类意图），两侧共读用例表 `frontend/src/lib/__fixtures__/dateFilterCases.json` 防漂移，改语义必须同步双端。
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

`datasource.Connection.Capabilities(ctx)` 返回 `DialectCapabilities`（GROUPING SETS / 百分位 / 窗口函数支持 + 百分位策略），上层（如 `query.Executor` 的 pivot 路径）据此选择 SQL 路径。探测为懒加载（`sync.Once` 连接生命周期内缓存）：PostgreSQL 首次调用时对真实库跑轻量只读探针；**MySQL/StarRocks 已升级为「只升不降 + nil 保底」探针** —— 从保守基线出发，只对能「响亮失败」的布尔能力跑探针、成功才升 true，探针失败或无实例时保留基线（行为等同旧静态值，不回归）。ClickHouse 刻意保持静态（剩余布尔能力已是文档-true，percentile 需语义校验才敢翻转）。

| 方言 | GROUPING SETS | 百分位策略 | 窗口函数 | 验证状态 |
|------|--------------|-----------|---------|---------|
| PostgreSQL | ✓ | `percentile_cont` | ✓ | **实测验证**（dev PG 18.6，真实探针 + `capabilities_integration_test.go`） |
| ClickHouse | ✓（文档支持，CH 21.x+） | `unsupported`（保守） | ✓（文档支持） | grouping-sets/window 文档支持但无实例未运行验证；百分位保守 unsupported，待实例探针验证 `quantilesExactInclusive` 后翻转（刻意保持静态，不跑探针） |
| MySQL | ✗（无 GROUPING SETS，仅 WITH ROLLUP，基线恒 false） | `unsupported` | 只升不降探针（8+ 支持） | **只升不降 + nil 保底探针**，无实例时等同保守基线；窗口函数跑探针成功才升 |
| StarRocks | 只升不降探针（较新版本支持，基线 false） | 基线精确百分位（实例实测保留、不重探） | 只升不降探针 | **只升不降 + nil 保底探针**，独立探针不继承 MySQL 结果 |

受能力门控的图表落地参差：**boxplot 仅 PostgreSQL 端到端可用**（StarRocks percentile 策略已实测但翻转需真实实例；ClickHouse 待语义校验；MySQL 无标量 percentile 路径，暂不支持）。pivot 全源可用：数据源不支持 GROUPING SETS 时（`SupportsGroupingSets=false`，如 MySQL/StarRocks）自动回退 UNION ALL 查询路径。待有 MySQL/ClickHouse/StarRocks 真实实例时，按各驱动 `Capabilities()` 探针实测后翻转保守默认。

## 契约与路由（Batch 2）

- **契约单一事实源**：`api/openapi.yaml` 定义全部端点请求/响应 + `Envelope`（`code/msg/trace/data`）+ `ChartSpec`/`QuerySpec`；`make api-gen` 生成 `backend/internal/idls/gen_types.go`（oapi-codegen）与 `frontend/src/idls/gen_types.ts`（openapi-typescript）。前端运行时类型已切换到生成物打底：`frontend/src/api/index.ts` 的实体/响应/请求类型 alias 到 `components['schemas']`，窄联合处用薄手写层重收紧。⚠️ `make api-gen` 的前端半步当前必然失败（`openapi-typescript` 依赖 `typescript` 运行时导出 `ts.factory`，与本仓 `typescript@7` 不兼容，崩溃在写文件之前），彻底修法是把生成器与 `typescript@7` 解耦；后端那半步正常。
- **泛型路由**：`backend/internal/router/router.go` 的 `API[In,Out] func(req Request[In], res *Response[Out]) error`（`res` 必须是指针，值传递会丢弃 handler 写入）已接入全部 40 个 API 端点。路由器按 HTTP 方法绑定 JSON body（POST/PUT/PATCH）+ query 参数并统一信封；`cmd/routes.go` 用 `Register{Get,Post,Put,Delete}Route` 注册，迁移样板见 `handler/datasource.go` 顶部 package doc。例外：`/health`（cmd/main.go）与 share `View`（302 重定向无法套 JSON 信封）。
- **图表配置 v2**：`bi_chart.config` 为 `{version:2, chartType, title, query:{dimensionGroups,metricGroups,filters,sort,limit}, fieldMeta, style, queryOptions}` 文档。字段组的 `fields`（列名数组）升级为 `bindings: BindingInstance[]`，每个「拖入槽位的字段实例」持全局唯一 `bindingId`（形如 `b-0`/`b-1`）；`fieldMeta` 的键由列名改为 `bindingId`（同列拖入两个组各得独立元数据），`sort` 亦以 `bindingId` 引用目标绑定，`bindings[].fieldId`/`filters[].fieldId` 引用的是列的稳定 id（`DatasetColumn.id`）。`frontend/src/lib/chartConfigSchema.ts` 的 `migrateChartConfig` 在加载时把旧结构（无 version：`queryConfig` + 5 个平铺 Record + 位置 `field-N`）与 v1 中间表示逐层迁移到 v2，ShareView 据此渲染。

### 响应层

负责将查询结果包装成统一响应 Envelope：

- `shape`
- `data`
- `meta`
- `fields`
- `sql`

兼容期内保留当前旧协议，同时逐步将前后端收敛到统一结构。

## 仪表盘 v1

拖拽式仪表盘（`backend/internal/service/dashboard` + `frontend/src/components/Dashboard*`）：

- **布局**：12 列栅格（`frontend/src/lib/dashboardLayoutSchema.ts` 的 `DASHBOARD_GRID_COLS = 12`，渲染基于 react-grid-layout），落库为 `bi_dashboard.layout_json`，块含图表 widget 与筛选器 widget。
- **盘级筛选器三族**（`frontend/src/lib/dashboardFilterValue.ts` 的 `filterWidgetFamily`）：`date`（复用 `components/DateFilter` 语义）、`string`（枚举多/单选，候选值实查）、`number`（算子下拉 + 数值输入）；三族只有控件形态与算子词表分族，取值下发是同一条链路。筛选器绑定 `(datasetId, column)` 二元组，`column` 是列的稳定 id。
- **盘级批量取数**：`POST /api/dashboards/{id}/query` 只收各筛选器当前取值 `{widgetId, value[]}`，与图表自身条件的合并由后端单点完成（`service/dashboard/query.go` 的 `buildOverrides`），逐块复用图表取数管道。日期族取值的意图解析仍走 `query/datefilter.go` 的 Go 侧镜像。

## 关键设计决策（Key Design Decisions）

本节记录「为什么这样设计」——每条给「**决策 / 背景 / 取舍 / 现状**」四段。决策点由架构约束驱动、由代码验证，读者是开发者；产品侧的「为什么做这个功能」见用户指南。

### D1 单镜像单进程（无 nginx，Go 按路径分流）

- **决策**：前端产物与 Go 二进制打进同一镜像，由一个进程、一个端口（23352）同时提供 API 与页面。
- **背景**：容器化 + 12-factor 部署下，多容器编排带来的配置面远大于收益；且前端路由需要 SPA 回落。
- **取舍**：不用 nginx 反代 → 少一个容器与一份 nginx.conf，但 Go 进程要自己承担路径分流（见 `internal/webui`，`/api` `/mcp` `/health` 保留前缀 JSON 404、其余回落 `index.html`）。静态目录配错即启动失败，换取"配置错误立刻暴露"。
- **现状**：已实现。证据：根 `Dockerfile` 三阶段、`backend/internal/webui`、`make serve` 本地验证同形态。

### D2 配置仅环境变量、不提供配置文件

- **决策**：所有配置只从环境变量读取，不提供 `config.toml` / `config.yaml`。
- **背景**：容器化部署下，每个要用户填的值都必须能从外部注入（`docker run -e` / `--env-file` / compose `environment` / K8s env）。
- **取舍**：不挂配置文件 → 消除"改了没生效"的歧义（同一项两处可写、还得记优先级）；代价是没有集中查看处，键名清单靠 `.env.example`。
- **现状**：已实现。证据：`backend/internal/config/config.go` 的 `Load()`、根 `.env.example`、`backend/etc/` 已删除。

### D3 契约单一事实源（openapi.yaml）+ 生成物打底

- **决策**：`api/openapi.yaml` 是前后端接口的唯一事实源，`make api-gen` 生成 Go（oapi-codegen）与 TypeScript（openapi-typescript）双端类型，前端运行时类型 alias 到生成物。
- **背景**：手写双端类型长期漂移，字段名不一致是联调成本主因。
- **取舍**：不手写运行时类型 → 换来字段名逐字一致；代价是窄联合处需薄手写层，且生成器与 `typescript@7` 不兼容时前端半步会失败（崩溃发生在写文件之前，属已知问题）。
- **现状**：后端生成物正常；前端消费生成物打底、窄联合处薄手写层。证据：`backend/internal/idls/gen_types.go`、`frontend/src/api/index.ts`。

### D4 泛型路由 RegisterRoute[In,Out] + handler-local In 镜像

- **决策**：handler 统一为 `func(req Request[In], res *Response[Out]) error`，由 `internal/router` 按 HTTP 方法自动绑定 body/query 并包装信封；In 用 handler-local 镜像 struct。
- **背景**：早期每个 handler 手写 `response.*` 与 gin 绑定，重复且易漏。
- **取舍**：不直接拿 `entity` 作 In（entity 缺 `form:"-"`，会让 query 参数污染 body）→ 用 handler-local 镜像，代价是需反射守卫其与 entity 的 json tag 一致。
- **现状**：已接入全部 40 个 API 端点。证据：`backend/internal/router/router.go`、`handler/contract_parity_test.go`。

### D5 bun_builder 是图表 SQL 唯一出口（参数化 + 标识符白名单）

- **决策**：图表 SQL 只由 `query/bun_builder.go` 经 AST 生成，值参数一律走 `args`；标识符过白名单校验。
- **背景**：手写字符串拼 SQL 存在注入面，且方言差异散落各处。
- **取舍**：删除手写字符串 builder 死路径 → 收敛到单一出口；代价是复杂查询需先建 AST（`planner.go`）。
- **现状**：已实现。证据：`query/bun_builder.go`；`query/dialect.go` 仅保留 `DialectType`/`ParseDialect`/`BuildQueryStringWithBun`。链路详见 `chart-query-design.md`。

### D6 日期语义双端镜像（共用用例表防漂移）

- **决策**：前端 `frontend/src/lib/dateFilter.ts` 是语义单一事实源，后端 `internal/query/datefilter.go` 是其镜像；双端共读用例表防漂移。
- **背景**：分享页 / 仪表盘由后端读 config 组查询，「最近 7 天」这类动态日期在**后端**也必须成立，不能只在前端解析。
- **取舍**：不做两份独立实现 → 一个意图、两端各自解析；代价是改语义必须同时改两边（共读用例表会直接抓出不一致）。
- **现状**：已实现。证据：`datefilter_test.go` + `dateFilter.cases.test.ts` 各跑一遍 `frontend/src/lib/__fixtures__/dateFilterCases.json`。

### D7 软删 + 显式级联（同一事务）

- **决策**：5 张业务表用 `deleted_at` 软删，级联显式写在同一事务；`bi_query` 永不物理删。
- **背景**：BI 资产（图表 / 数据集 / 数据源）被其它资源引用（仪表盘、分享），物理删会断链。
- **取舍**：不物理删 → 保留可回溯与引用完整性；代价是查询恒带 `deleted_at IS NULL`，且级联软删的子查询**刻意不过滤**软删（父行刚软删，加过滤会断链）。
- **现状**：已实现。证据：`service/dashboard/impl.go` 的 `Delete`（显式 `SET deleted_at`，不走整行 UPDATE）、各 service 的级联软删。

### D8 仪表盘 layout 归前端、后端防御性投影

- **决策**：`bi_dashboard.layout_json` 的布局文档（DashboardLayout）归前端所有，后端只做防御性投影读取。
- **背景**：布局是纯前端渲染关注点（12 列栅格 + react-grid-layout），后端无需理解其内部形态。
- **取舍**：后端不做 layout schema 强校验 → 前端迭代布局不必改后端；代价是后端要防御非法 / 缺失 layout（解析不出时返空 `results` 而非报错）。
- **现状**：已实现。证据：`frontend/src/lib/dashboardLayoutSchema.ts`、`service/dashboard` 的投影读取。

### D9 盘级筛选合并后端单点完成

- **决策**：`POST /api/dashboards/{id}/query` 只收筛选器当前取值，与图表自身条件的合并由后端单点完成（`buildOverrides`），**绝不写回 `bi_chart.config`**。
- **背景**：合并若下放前端，需前端解析 chart config，且多块联动易出现不一致。
- **取舍**：前端不解析、不下发合并结果 → 合并逻辑单点；代价是取值形状必须按算子分流（`in`/`notIn` 数组、`between` 两元素、其余标量一元素、`isNull`/`notNull` 靠数组非空表示激活）。
- **现状**：已实现。证据：`service/dashboard/query.go` 的 `buildOverrides`、`frontend/src/lib/dashboardFilterValue.ts` 同构。

### D10 ChartConfigDocument 版本化 + migrate

- **决策**：`bi_chart.config` 是带 `version` 的 `ChartConfigDocument`，加载时经 `migrateChartConfig` 从任意历史结构无损迁移。
- **背景**：字段组从「列名位置索引」升级为「binding 实例（`bindingId`）」，旧图表必须仍可打开。
- **取舍**：不做破坏性 schema 变更 → 靠迁移函数兼容旧结构；代价是迁移层需长期维护，且迁移必须无损。
- **现状**：已实现（当前 `version: 2`）。证据：`frontend/src/lib/chartConfigSchema.ts`；字段组演变详见 `chart-query-design.md`。
