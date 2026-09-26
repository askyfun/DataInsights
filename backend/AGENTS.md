# Backend AGENTS.md

Go 后端服务，提供 Data Insights 平台的 REST API。

## 技术栈

Go 1.27 + Gin + bun ORM + PostgreSQL + Sentry

## 目录结构

```
backend/
├── cmd/
│   ├── main.go          # 入口：加载配置、初始化 DB、注册中间件和路由、启动 HTTP 服务
│   ├── routes.go        # 路由注册：实例化 service → handler，挂载到 Gin RouterGroup
│   └── chart_query_test.go
├── internal/
│   ├── config/          # 配置加载（纯环境变量 + godotenv 读 .env；默认值 < .env < 真实环境变量）
│   ├── webui/           # 前端产物托管与路径分流（挂 NoRoute；命中文件→返回，未命中→index.html，保留前缀→JSON 404）
│   ├── database/        # DB 初始化（pgx + bun）和迁移
│   ├── handler/         # Gin HTTP 处理器（请求绑定、响应格式化）
│   ├── service/         # 业务逻辑层（按领域拆分：chart, dashboard, dataset, datasource, share, queryrecord）
│   ├── domain/entity/   # 领域实体和接口定义
│   ├── model/           # bun ORM 模型（bi_datasource, bi_dataset, bi_chart, bi_share, bi_dashboard, bi_query, bi_setting）
│   ├── idls/            # gen_types.go（由 make api-gen 从 api/openapi.yaml 生成的请求/响应 DTO）
│   ├── query/           # SQL 查询构建、执行和结果处理
│   ├── keystore/        # SECURITY_KEY 解析与持久化（留空则首次随机生成、落 bi_setting 表）
│   ├── crypto/          # AES-GCM 加解密（密钥来自 keystore）
│   ├── datasource/      # 数据源驱动抽象（Driver/Connection 接口）
│   ├── router/          # 泛型路由注册工具（RegisterRoute[In, Out]）
│   └── response/        # 统一响应格式（code/msg/trace/data）
├── migrations/          # 数据库迁移脚本（goose 版本化，embed.FS 内嵌）
└── bin/                 # 编译输出
```

## 分层架构

```
handler → service → domain/entity
                  → model (bun ORM)
                  → datasource (外部数据库连接)
                  → query (SQL 构建/执行)
```

- **handler**: 绑定请求参数，调用 service，格式化响应。不包含业务逻辑。
- **service**: 业务逻辑，操作 model 和 datasource。每个领域一个子包。
- **domain/entity**: 领域实体结构体和 service 接口定义。
- **model**: bun ORM 模型，直接映射数据库表。
- **query**: SQL 查询构建（`bun_builder.go` 为图表 SQL 唯一出口）、执行器（Executor）和结果处理器（Processor）。
- **datasource**: 数据库驱动接口（Driver/Connection），支持 PostgreSQL、ClickHouse、MySQL、StarRocks。

## 关键设计模式

### 泛型路由注册

`router/router.go` 提供 `RegisterRoute[In, Out]` 泛型函数（`RegisterGetRoute`/`RegisterPostRoute`/`RegisterPutRoute`/`RegisterDeleteRoute`），自动绑定 query 参数和 JSON body，统一包装响应。**40 个 API 端点已全部经此注册**（datasource 11 + dataset 9 + chart 8 + share 4 + queryrecord 2 + dashboard 6）。

- 签名 `API[In,Out] func(req Request[In], res *Response[Out]) error`——`res` 为指针，值传递会静默丢弃 handler 写入。
- 路由器按 HTTP 方法自动绑定 JSON body（POST/PUT/PATCH 绑定，GET/DELETE 不绑定）+ query 参数；handler 内部不再手写 `response.*`。例外只有 2 个：`/health` 在 cmd/main.go 用裸 `r.GET`+`response.Success`；share `View` 返回 302 重定向且**只在纯 API 模式下注册**（托管前端时 `/share/<token>` 归前端路由，后端同路径 302 会抢先把页面挡掉）。
- handler 入参用 handler-local In 镜像 struct（entity 不带 `form:"-"`，否则 query 参数会污染 body），其与 entity 的 json tag 一致性由 `internal/handler/contract_parity_test.go` 反射守卫。
- 已接受的两类"不可消除差异"（bind 错误文本含 struct 名、双非法输入时 body 绑定错误优先于 path）由 baseline 测试钉死（8 个 path+body 端点 × 双非法 Empty/Malformed 变体全覆盖）。迁移样板见 `handler/datasource.go` 顶部 package doc。
- PUT 更新遵循**"未提供则保留"**项目约定（datasource 密码、dataset 可选元数据同例）：payload 省略/空的可选字段保留存量值而非清零，显式 `"[]"` 仍可清空。

> 无独立 `middleware/` 包：requestID、CORS、Sentry 中间件均在 `cmd/main.go` 装配处定义并 `r.Use(...)` 挂载。

### 数据源驱动

`datasource/driver.go` 定义 `Driver` 和 `Connection` 接口。新增数据库驱动只需：
1. 实现 `Driver` 接口（`Type()`, `Connect()`, `TestConnection()`）
2. 实现 `Connection` 接口（`Close()`, `Ping()`, `GetTables()`, `GetColumns()`, `Execute()`）
3. 在 `NewDriver()` 工厂函数中注册

### 查询处理

`query/` 包包含完整的查询管道（手写字符串 SQL builder 已删除，`bun_builder.go` 是图表 SQL 的唯一出口）：

- **参数化红线**：值参数一律通过 `Connection.Execute(ctx, sql string, args ...any)` 的 args 传递；标识符使用白名单校验（裸名 `datasource.IsValidIdentifier`，query 包 `safeIdentifier` 额外允许成对引号包裹的标识符）；聚合表达式 `aggExprPattern` 收紧为显式函数白名单（`count|sum|avg|min|max`），杜绝 `pg_sleep(1)` 之类经列 `FieldExpr` 注入任意函数名。
- `types.go` — 类型定义（ChartType, MetricConfig, FilterConfig 等）
- `ast.go` — QueryAST 节点定义
- `planner.go` — QuerySpec → PlannedAST 的查询规划
- `chart_spec.go` — 图表规格定义
- `bun_builder.go` — 使用 bun 框架的参数化 SQL 构建器（含 `bun_builder_{pivot,boxplot,histogram}.go` 各图表路径）
- `raw.go` — 原始 SQL 构造（表预览、字段分布等）
- `executor.go` — 查询执行器，编排 AST → 数据源执行 → Processor
- `processor.go` / `processor_pivot.go` / `processor_stats.go` — 结果处理器（Table, Pie, Axis, Scatter, Pivot, Stats）
- `datefilter.go` — 前端 `lib/dateFilter.ts` 日期语义的 Go 侧镜像（分享页/仪表盘后端读 config 解析日期意图），与 TS 共读用例表 `frontend/src/lib/__fixtures__/dateFilterCases.json` 防漂移
- `dialect.go` — SQL 方言适配（仅 `DialectType`/`ParseDialect`/`BuildQueryStringWithBun`）

### 统一响应格式

所有 API 响应以及 `GET /health` 健康检查响应使用 `response.Success/Error/BadRequest` 包装：
```json
{"code": 20000, "msg": "success", "trace": "req-id", "data": {...}}
```

## 常用命令

```bash
# 推荐走仓库根 Makefile
make install-backend   # 安装 Go 依赖
make dev-backend       # 启动服务，端口 23352（使用 air 热重载）
make build-backend     # 构建后端二进制

# 直接调 go 命令（需要时）
cd backend
go mod download
go run ./cmd                               # 启动服务，端口 23352（写成 cmd/main.go 会缺 routes.go）
go build -o bin/server ./cmd               # 构建后端二进制
go test ./...                              # 运行所有测试
go test -v ./path/to/pkg -run TestName     # 运行单个测试
go test -race ./...                        # 带竞态检测运行测试
```

## 配置加载

优先级、变量字典、fail-fast 语义与容器注入方式的**完整说明见根 `AGENTS.md` 的「配置」一节**（单一事实源，不要在本文件重复展开）。后端侧要点：装配逻辑全在 `internal/config/config.go` 的 `Load()`；启动参数只有 `-env`（默认依次探测 `./.env`、`../.env`）；`STATIC_DIR` 留空 = 只提供 API，此时 `SetupRoutes` 才注册 `/share/:token` 302 落地页，目录配错（读不到 `index.html`）启动即退出。

⚠️ **写测试的坑**：`t.Setenv(k, "")` 只是把变量设成"存在但为空"，而 godotenv 的语义是"已存在的变量一律不覆盖" —— 两者相遇会让 `.env` 里的值被空壳挡住。凡是要经过 `LoadDotEnv` 的用例必须用 `os.Unsetenv`（测试里的 `unsetEnv` helper），不能图省事用 `clearEnv`。

## 数据库表

| 表名 | 用途 |
|------|------|
| `bi_datasource` | 数据源连接配置 |
| `bi_dataset` | 数据集定义（表名或 SQL 查询） |
| `bi_dataset_lineage` | 数据集血缘关系 |
| `bi_chart` | 图表配置 |
| `bi_share` | 分享链接 |
| `bi_dashboard` | 仪表盘配置（layout_json 12 列栅格） |
| `bi_query` | 查询记录（queryrecord 领域） |
| `bi_setting` | 运行期设置（keystore 持久化 SECURITY_KEY） |

## API 路由

所有路由前缀 `/api`，除 `GET /health`（健康检查，cmd/main.go）和 `GET /share/:token`（分享查看页 302，仅纯 API 模式注册）外，共 **40 个端点** 经泛型路由注册（datasource 11 + dataset 9 + chart 8 + share 4 + queryrecord 2 + dashboard 6）。

| 领域 | 路由 |
|------|------|
| Datasource (11) | CRUD + test, tables, columns, table data, preview, field-distribution |
| Dataset (9) | CRUD + columns(读/写), preview, query |
| Chart (8) | CRUD + data, query, references（references 由 dashboard handler 提供，归属 chart 资源） |
| Share (4) | list, create, get-by-token, verify |
| QueryRecord (2) | save, get |
| Dashboard (6) | CRUD + query（`POST /api/dashboards/{id}/query` 盘级批量取数） |

## 数据库与迁移

数据库 schema 由 goose 版本化迁移管理（`backend/migrations/00001_init_schema.sql`，通过 `embed.FS` 内嵌），`model.CreateTables` 已删除；事务统一走 `database.WithTx`。

## 运维与安全

- Sentry 通过 `SENTRY_DSN` 接通（sentrygin Repanic）。
- CORS 默认放开所有来源（平台 API 无登录态，CORS 不构成安全边界）；`CORS_ALLOWED_ORIGINS` 填了非空白名单则只回显名单内来源，供将来引入认证后收紧。
- requestID 中间件返回非全零 ID。
- datasource 密码 AES-GCM 加密存储（密钥见根「配置」的 `SECURITY_KEY` 说明），API 响应中 `password` 字段 `json:"-"` 不外泄。
- share 使用 bcrypt 哈希并对外暴露 `has_password` 契约。

## 约束

- JSON 字段使用 snake_case
- 错误处理：禁止空错误块，必须记录日志并返回错误响应
- 新增功能必须补充单元测试
- 提交前运行 `go test -race ./...`
