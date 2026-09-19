# Backend AGENTS.md

Go 后端服务，提供 DataRay 平台的 REST API。

## 技术栈

Go 1.26 + Gin + bun ORM + PostgreSQL + Sentry

## 目录结构

```
backend/
├── cmd/
│   ├── main.go          # 入口：加载配置、初始化 DB、注册中间件和路由、启动 HTTP 服务
│   ├── routes.go        # 路由注册：实例化 service → handler，挂载到 Gin RouterGroup
│   └── chart_query_test.go
├── etc/
│   └── config.toml      # TOML 配置文件（Host, Port, Database.Url）
├── internal/
│   ├── config/          # 配置加载（go-toml）
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
cd backend
go mod download
go run ./cmd/main.go -f etc/config.toml   # 启动服务，端口 23352
go test ./...                              # 运行所有测试
go test -v ./path/to/pkg -run TestName     # 运行单个测试
go test -race ./...                        # 带竞态检测运行测试
```

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
