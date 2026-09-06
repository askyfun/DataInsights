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
│   │   ├── migrations/       # goose 版本化迁移（embed.FS 内嵌）
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
- 旧的 `query/builder.go` 与平铺参数兼容链路已删除。
- 数据库 schema 由 goose 版本化迁移管理（`backend/migrations/`，embed.FS 内嵌），事务统一走 `database.WithTx`。

当前落地状态：

- 对外接口仍使用旧协议：`chart_type` + `dims` + `metrics` + `filters` + `pagination` + `sort`。
- `backend/internal/service/chart/impl.go` 将旧请求转换为内部 `QuerySpec`，再经 `QueryPlanner` 生成 `PlannedAST`，由 `query.Executor` 消费 AST 生成参数化 SQL。
- `QueryAST` 第一阶段已支持结构化维度/指标元信息、`limit`，以及日期维度 `day` 粒度的 PostgreSQL / MySQL / ClickHouse 方言 SQL 生成。
- 分桶、复杂过滤组、更多时间粒度仍属于后续阶段。

### 响应层

负责将查询结果包装成统一响应 Envelope：

- `shape`
- `data`
- `meta`
- `fields`
- `sql`

兼容期内保留当前旧协议，同时逐步将前后端收敛到统一结构。
