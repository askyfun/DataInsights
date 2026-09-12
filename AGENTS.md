# DataRay 开发规范

**项目**: DataRay - 拖拽式 BI 可视化分析平台

## 项目概述

DataRay 是一个拖拽式 BI 可视化分析平台（MVP）。Monorepo 结构，包含前端（React/TypeScript）和后端（Go）。核心功能：数据源管理、数据集管理、拖拽式图表构建、分享功能。

## 架构

### 技术栈

- **前端**: React 18 + TypeScript + Ant Design 5.x + ECharts 5.x + Zustand 4.x + @dnd-kit + Vite 6 + Biome（lint/格式化） + Vitest（测试）
- **后端**: Go 1.26 + Gin + bun ORM + PostgreSQL + Sentry
- **部署**: Docker + docker-compose

### 后端分层架构

后端采用 handler → service → domain 分层模式：

| 层级 | 目录 | 职责 |
|------|------|------|
| Handler | `backend/internal/handler/` | Gin HTTP 处理器，请求绑定，响应格式化 |
| Service | `backend/internal/service/` | 业务逻辑（按领域：chart, dataset, datasource, share） |
| Domain | `backend/internal/domain/entity/` | 领域实体类型 |
| Query | `backend/internal/query/` | SQL 构造唯一出口：AST + bun_builder（结构化查询）+ raw.go（原始 SQL 构造） |
| Model | `backend/internal/model/` | 数据库模型（bun ORM） |
| Router | `backend/internal/router/` | 泛型路由注册 `RegisterRoute[In, Out]`（**已全面启用**：31 个 API 端点经此注册；`/health` 与 share `View` 为 2 个已记录例外，见下） |
| Datasource | `backend/internal/datasource/` | 数据源驱动抽象（Driver 接口） |
| Crypto | `backend/internal/crypto/` | AES-GCM 加解密（密钥来自 `DATARAY_SECURITY_KEY`） |

查询链路：handler → service → `query` 包（AST + bun_builder / raw.go）→ datasource 驱动。`Connection.Execute(ctx, sql string, args ...any)` 已支持参数化执行，bun_builder 是图表 SQL 的唯一出口，值参数一律通过 args 传递；标识符使用白名单校验（裸名 `datasource.IsValidIdentifier`，query 包 `safeIdentifier` 额外允许成对引号包裹的标识符）。`dialect.go` 的手写字符串 SQL builder（`SQLBuilder`/`baseSQLBuilder`/`BuildQueryString` 及各方言 builder）已作为死代码删除，仅保留 `DialectType`/`ParseDialect`/`BuildQueryStringWithBun`；聚合表达式 `aggExprPattern` 收紧为显式函数白名单（`count|sum|avg|min|max`），杜绝 `pg_sleep(1)` 之类经列 `FieldExpr` 注入任意函数名。

契约工程（Batch 2/3）：`api/openapi.yaml` 是前后端接口的单一事实源，`make api-gen` 生成 `backend/internal/idls/gen_types.go`（oapi-codegen）与 `frontend/src/idls/gen_types.ts`（openapi-typescript）。**前端运行时类型已切换到生成物打底（Batch 3）**：`frontend/src/api/index.ts` 的实体/响应/请求类型 alias 到 `components['schemas']`，窄联合处用薄手写层（`Omit<G.X,'type'> & { type: Union }`）重收紧；已判死的声明（手写 `typeConfig`——wire 实为 snake `type_config` 且前端零消费、`ColumnInfo` 幽灵字段、`GeneratedSQL` 等）随迁移删除。三条静默红线仍有效：生成 `*Response` 是信封包装（裸 payload 对应 `ChartDataResult` 等去后缀类型，禁按名替换）、`*FormData` 是 UI 数组约定保留手写、store `QueryConfig` 为 camelCase UI 模型不换。后端 handler In/Out 仍为 handler-local 镜像（json tag 由 `contract_parity_test.go` 反射守卫），**未**切换到 `idls.*`——生成 Go 类型无 gin `form:"-"` 语义，直接作 In 会重新引入 query 污染注入面，切换列入 Batch 4 重评。图表 `bi_chart.config` 已升级为带 `version:1` 的文档（`frontend/src/lib/chartConfigSchema.ts` 的 `ChartConfigDocument` + `migrateChartConfig`），旧结构在加载时自动迁移（`fieldId` 由位置 `field-N` 改为稳定列名），ShareView 据此渲染新结构图表；图表查询请求线不含 `config` 字段（Batch 3 已移除从未生效的 pie 合并比例死链，`query.PieProcessor.MergeOtherBelowRatio` 能力保留但未接线）。

`backend/internal/router/router.go` 的泛型路由（`RegisterGetRoute`/`RegisterPostRoute`/`RegisterPutRoute`/`RegisterDeleteRoute`）**已启用并接入 31 个 API 端点**（datasource 11 + dataset 9 + chart 7 + share 4；dataset 的 `PUT /:id` 为 Batch 3 补齐，修复前端编辑保存 404 的 ghost route）。签名 `API[In,Out] func(req Request[In], res *Response[Out]) error`——`res` 为指针，值传递会静默丢弃 handler 写入。路由器按 HTTP 方法自动绑定 JSON body（POST/PUT/PATCH 绑定，GET/DELETE 不绑定）+ query 参数，并统一包装 `response` 信封；handler 内部不再手写 `response.*`（2 个例外：`/health` 在 cmd/main.go 用裸 `r.GET`+`response.Success`；share `View` 返回 302 重定向，无法套 JSON 信封）。迁移样板见 `handler/datasource.go` 顶部 package doc；已接受的两类"不可消除差异"（bind 错误文本含 struct 名、双非法输入时 body 绑定错误优先于 path）由 baseline 测试钉死（8 个 path+body 端点 × 双非法 Empty/Malformed 变体全覆盖）。handler 入参用 handler-local In 镜像 struct（entity 不带 `form:"-"`，否则 query 参数会污染 body），其与 entity 的 json tag 一致性由 `internal/handler/contract_parity_test.go` 反射守卫。PUT 更新遵循**"未提供则保留"**项目约定（datasource 密码、dataset 可选元数据同例）：payload 省略/空的可选字段保留存量值而非清零，显式 `"[]"` 仍可清空。

`datasource/` 包实现了 `Driver` 接口用于多数据库后端——新增驱动只需实现该接口。

数据库 schema 由 goose 版本化迁移管理（`backend/migrations/00001_init_schema.sql`，通过 `embed.FS` 内嵌），`model.CreateTables` 已删除；事务统一走 `database.WithTx`。

运维与安全已落地：Sentry 通过 `[Sentry] Dsn` 配置接通（sentrygin Repanic）；CORS 按 `[CORS] AllowedOrigins` 配置化（默认 `localhost:3000`）；requestID 中间件返回非全零 ID；datasource 密码 AES-GCM 加密存储，API 响应中 `password` 字段 `json:"-"` 不外泄，share 使用 bcrypt 哈希并对外暴露 `has_password` 契约。

### 前端结构

```
frontend/src/
├── api/           # API 客户端（axios）
├── store/         # Zustand 状态管理
├── pages/         # 页面组件
├── components/    # 可复用组件
├── idls/          # API 类型定义（现仅 gen_types.ts 生成物；Batch 2 已删除手写 chart/dataset/datasource/share.ts）
├── lib/           # 工具库（API 客户端在 lib/api/client.ts；图表配置 schema 在 lib/chartConfigSchema.ts）
├── i18n/          # 国际化
└── styles/        # 全局样式
```

路径别名：`@/*` 映射到 `./src/*`。

## 常用命令

```bash
# 前端
cd frontend && pnpm install
pnpm dev               # 开发服务器，端口 3000
pnpm build             # tsc + vite 构建
pnpm format            # biome 格式化
pnpm lint              # biome lint 检查
pnpm check             # biome 完整检查（lint + 格式化）
pnpm test              # vitest 运行测试
pnpm build:check       # biome check + vitest（提交前验证）

# 后端
cd backend && go mod download
go run ./cmd/main.go -f etc/config.toml   # 开发服务器，端口 8080
go test ./...                              # 运行所有测试
go test -v ./path/to/pkg -run TestName     # 运行单个测试
go test -race ./...                        # 带竞态检测运行测试

# Makefile（项目根目录）
make dev               # 前后端同时启动，带热重载（air）
make dev-frontend      # 仅前端
make dev-backend       # 仅后端（使用 air 热重载）
make build             # 前后端构建
make docker-up         # docker-compose 启动所有服务
make docker-down       # 停止服务
make clean             # 清理 dist、node_modules、backend/bin
```

## 行为准则

### 先思考再编码

- 不确定时先探索，不要猜测。有歧义必须问用户。
- 如果存在多种理解方式，列出它们——不要默默选一种。
- 如果有更简单的方案，说出来。觉得不合理时应主动 push back。
- 遇到不清楚的地方停下来，说清楚哪里不明白，再提问。

### 极简优先

- 只写解决问题的最少代码，不做投机性设计。
- 不添加未要求的功能、抽象、灵活性或可配置性。
- 不为不可能发生的场景添加错误处理。
- 如果 50 行能解决的事写了 200 行，重写它。

### 精准修改

- 编辑现有代码时，只动必须动的部分，不顺手"改进"周边代码、注释或格式。
- 匹配现有代码风格，即使你会用不同的方式写。
- 你的改动导致的未使用导入/变量/函数，自行清理；不要删除改动之前就存在的死代码（除非被要求）。
- 检验标准：每一行改动都应能直接追溯到用户的需求。

### 接口先行与双端测试

- 新增功能时，先定义前后端接口（请求/响应结构），再以此接口为标准分别编写前后端单元测试。
- 接口是前后端的契约，测试围绕契约编写，便于隔离定位问题属于前端还是后端。

### 反思与记录

- 开始工作前先读 `./MEMORY.md`，获取历史经验。
- 主动反思，不依赖用户指出问题。当意识到自己可能犯错、可能需要改进时，就进行反思记录。
- 反思后将根因和教训追加到 `./MEMORY.md`，避免同类错误重复发生。

### 目标驱动执行

- 将任务转化为可验证的目标，循环执行直到验证通过。
- 多步任务先给出简要计划：
  ```
  1. [步骤] → 验证: [检查方式]
  2. [步骤] → 验证: [检查方式]
  ```
- 强成功标准让独立工作成为可能；弱标准（"让它跑起来"）会导致反复确认。

## 关键约束

- **JSON 字段命名**: 前后端 API 通信使用 snake_case（如 `table_name`、`query_sql`、`created_at`）。TypeScript 接口必须与后端字段名完全一致。
- **TypeScript**: 使用 `unknown` 代替 `any`，禁止使用 `@ts-ignore` 或 `as any`。
- **Go 错误处理**: 禁止空错误块，必须记录日志并返回错误响应。
- **测试**: 后端新增功能或修复 bug 后必须补充单元测试，提交前运行 `go test -race ./...`。
- **Bug 修复测试**: 每个 bug 修复前先写能复现问题的失败测试，修复后再确认测试通过。前后都要有对应单元测试。
- **代码可测试性**: 设计代码必须具备可测试性，禁止提交无法测试的代码。
- **文档同步**: 大型业务逻辑调整或架构调整必须同步更新 `docs/*.md`、`README.md`、`AGENTS.md` 等文档。
- **Pre-commit 钩子**: Husky 在前端运行 `lint-staged`，对暂存的 `.ts/.tsx` 文件执行 `biome check`。

## 零容忍

以下行为严格禁止：
- 部分实现（"简化版本"）
- 未经授权的范围变更
- 删除失败测试
- 类型压制（`as any`、`@ts-ignore`）
- 空 catch 块
- 顺手重构与需求无关的代码
- 添加未被要求的功能或抽象

## Agent 工作流

### 意图识别

| 表面形式 | 真实意图 | 路由方式 |
|----------|----------|----------|
| "explain X", "how does Y work" | 研究/理解 | explore → 回答 |
| "implement X", "add Y", "create Z" | 实现（显式） | plan → delegate |
| "look into X", "check Y" | 调查 | explore → 报告 |
| "I'm seeing error X" | 修复 | 诊断 → 最小化修复 |
| "refactor", "improve" | 开放性变更 | 评估 → 建议方案 |

### Agent 选择

| Agent | 用途 | 使用场景 |
|-------|------|----------|
| `explore` | 代码库上下文搜索 | 搜索现有模式、查找文件 |
| `librarian` | 外部文档/参考 | 不熟悉的库、官方文档 |
| `oracle` | 架构评审、复杂调试 | 2+ 次修复失败、架构决策 |
| `plan` | 任务规划 | 2+ 步任务、范围不清 |
| `momus` | 计划评审 | 工作计划质量审查 |

### PLAN 调用条件

- 任务有 2+ 步
- 范围不清晰
- 需要架构决策

### TODO 管理

1. 非平凡任务前创建 TODO
2. 每步标记 `in_progress`
3. 完成后立即标记 `completed`
4. 范围变更时更新 TODO

### 并行执行

独立任务并行运行，explore/librarian 使用 `run_in_background=true`。

## 配置

- 后端配置: `backend/etc/config.toml`（TOML 格式）
- 前端 API 基础 URL: `frontend/src/lib/api/client.ts`（默认 `http://localhost:8080`）
- Docker compose: `docker-compose.yml` — PostgreSQL（端口 5432）、后端（8080）、前端（3000）

## 开发资源

| 文档 | 说明 |
|------|------|
| [docs/setup.md](docs/setup.md) | 环境搭建、运行命令 |
| [docs/architecture.md](docs/architecture.md) | 目录结构、技术栈、图表查询与可视化语义分层 |
| [docs/api.md](docs/api.md) | API 接口文档（统一规范，含图表查询兼容协议与目标契约草案） |
| [docs/api-spec.md](docs/api-spec.md) | API 规范详情（含图表查询契约演进草案） |
| [docs/chart-builder-plan.md](docs/chart-builder-plan.md) | 图表构建器功能设计与契约演进方向 |
| [docs/coding-style.md](docs/coding-style.md) | 代码风格指南 |
| [docs/todo.md](docs/todo.md) | 开发任务清单 |

## 已知限制

- 前端无 ESLint/Prettier 配置（使用 Biome）
- 后端无 golangci-lint 配置
- 缺少端到端测试
