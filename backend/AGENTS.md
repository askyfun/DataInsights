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
│   ├── service/         # 业务逻辑层（按领域拆分：chart, dataset, datasource, share）
│   ├── domain/entity/   # 领域实体和接口定义
│   ├── model/           # bun ORM 模型（bi_datasource, bi_dataset, bi_chart, bi_share）
│   ├── idls/            # 请求/响应 DTO 定义
│   ├── query/           # SQL 查询构建、执行和结果处理
│   ├── datasource/      # 数据源驱动抽象（Driver/Connection 接口）
│   ├── router/          # 泛型路由注册工具（RegisterRoute[In, Out]）
│   ├── response/        # 统一响应格式（code/msg/trace/data）
│   └── middleware/       # 中间件（预留）
├── migrations/          # 数据库迁移脚本
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
- **query**: SQL 查询构建器（Builder）、执行器（Executor）和结果处理器（Processor）。
- **datasource**: 数据库驱动接口（Driver/Connection），支持 PostgreSQL、ClickHouse、MySQL、StarRocks。

## 关键设计模式

### 泛型路由注册

`router/router.go` 提供 `RegisterRoute[In, Out]` 泛型函数，自动绑定 query 参数和 JSON body，统一包装响应。

### 数据源驱动

`datasource/driver.go` 定义 `Driver` 和 `Connection` 接口。新增数据库驱动只需：
1. 实现 `Driver` 接口（`Type()`, `Connect()`, `TestConnection()`）
2. 实现 `Connection` 接口（`Close()`, `Ping()`, `GetTables()`, `GetColumns()`, `Execute()`）
3. 在 `NewDriver()` 工厂函数中注册

### 查询处理

`query/` 包包含完整的查询管道：
- `types.go` — 类型定义（ChartType, MetricConfig, FilterConfig 等）
- `builder.go` — SQL Builder（SELECT/FROM/WHERE/GROUP BY/ORDER BY/LIMIT）
- `bun_builder.go` — 使用 bun 框架的 SQL 构建器
- `executor.go` — 查询执行器，编排 Builder → 数据源执行 → Processor
- `processor.go` — 结果处理器（Table, Pie, Axis, Scatter, Pivot）
- `dialect.go` — SQL 方言适配
- `ast.go` — SQL AST 节点

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

优先级由低到高：**内置默认值 < `.env` 文件 < 真实系统环境变量**。

**没有配置文件。** `etc/` 目录已删除，`config.go` 不再依赖 go-toml。理由：容器化部署下每个要用户填的值都必须能从外部注入（`--env-file` / compose `env_file` / K8s env），多一份 TOML 只会让"改了没生效"变得难查。

- 入口：`cmd/main.go` 先 `config.LoadDotEnv(...)`，再 `Config.Load()`。`-env` 指定 .env（默认依次探测 `./.env`、`../.env`，覆盖"从仓库根启动"和"从 backend/ 启动"两种情形）。
- `.env` 默认落在**仓库根**（`../.env`），由 godotenv 加载且**不覆盖**已存在的环境变量 —— 这就是"真实环境变量 > .env"的实现方式，也是 `docker run --env-file` / K8s env 能覆盖它的原因。
- 环境变量（**无前缀**）：`PORT` / `DATABASE_URL` / `SECURITY_KEY` / `SENTRY_DSN` / `CORS_ALLOWED_ORIGINS`（留空 = CORS 放开所有来源，见 `cmd/main.go` `corsMiddleware`） / `STATIC_DIR`。**空字符串一律视为"未设置"**，所以 .env 里留空占位不会打掉内置默认值（Port `23352`；监听地址固定 `0.0.0.0`，不提供 env 覆盖）。
- `DATABASE_URL` 是**唯一必填项**：`main.go` 在 `Load()` 之后显式检查，为空即 `os.Exit(1)` 并提示变量名。
- `STATIC_DIR` 指向前端构建产物目录（镜像里是 `/app/web`）。**留空 = 只提供 API**，此时 `SetupRoutes` 才会注册 `/share/:token` 那条 302 落地页；配了静态目录则 `/share/<token>` 归前端路由（分享页本身就是 SPA 的 `/share/:token`，后端同路径的 302 会把它挡掉）。目录配错（读不到 `index.html`）同样 `os.Exit(1)`。

⚠️ **写测试的坑**：`t.Setenv(k, "")` 只是把变量设成"存在但为空"，而 godotenv 的语义是"已存在的变量一律不覆盖" —— 两者相遇会让 `.env` 里的值被空壳挡住。凡是要经过 `LoadDotEnv` 的用例必须用 `os.Unsetenv`（测试里的 `unsetEnv` helper），不能图省事用 `clearEnv`。

## 数据库表

| 表名 | 用途 |
|------|------|
| `bi_datasource` | 数据源连接配置 |
| `bi_dataset` | 数据集定义（表名或 SQL 查询） |
| `bi_dataset_lineage` | 数据集血缘关系 |
| `bi_chart` | 图表配置 |
| `bi_share` | 分享链接 |

## API 路由

所有路由前缀 `/api`，除 `GET /share/:token`（分享查看页面）和 `GET /health`（健康检查）。

| 领域 | 路由 |
|------|------|
| Datasource | CRUD + test, tables, columns, preview, field-distribution |
| Dataset | CRUD + columns, preview, query |
| Chart | CRUD + data, query |
| Share | list, create, get-by-token |

## 约束

- JSON 字段使用 snake_case
- 错误处理：禁止空错误块，必须记录日志并返回错误响应
- 新增功能必须补充单元测试
- 提交前运行 `go test -race ./...`
