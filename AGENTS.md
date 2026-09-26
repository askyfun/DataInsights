# Data Insights 开发规范

**项目**: Data Insights - 拖拽式 BI 可视化分析平台

## 项目概述

Data Insights 是一个拖拽式 BI 可视化分析平台（MVP）。Monorepo 结构，包含前端（React/TypeScript）和后端（Go）。核心功能：数据源管理、数据集管理、拖拽式图表构建、分享功能。

## 架构

### 技术栈

- **前端**: React 19 + TypeScript + Ant Design 6.x + ECharts 6.x + Zustand 5.x + @dnd-kit + Vite 8 + Biome（lint/格式化） + Vitest（测试）
- **后端**: Go 1.27 + Gin + bun ORM + PostgreSQL + Sentry
- **部署**: Docker + docker compose

### 后端分层架构

后端采用 handler → service → domain 分层模式：

| 层级 | 目录 | 职责 |
|------|------|------|
| Handler | `backend/internal/handler/` | Gin HTTP 处理器，请求绑定，响应格式化 |
| Service | `backend/internal/service/` | 业务逻辑（按领域：chart, dataset, datasource, share） |
| Domain | `backend/internal/domain/entity/` | 领域实体类型 |
| Query | `backend/internal/query/` | SQL 构造唯一出口：AST + bun_builder（结构化查询）+ raw.go（原始 SQL 构造） |
| Model | `backend/internal/model/` | 数据库模型（bun ORM） |
| Router | `backend/internal/router/` | 泛型路由注册 `RegisterRoute[In, Out]`（**已全面启用**：40 个 API 端点经此注册；`/health` 与 share `View` 为 2 个已记录例外，见下） |
| WebUI | `backend/internal/webui/` | 前端构建产物的托管与路径分流（挂在 gin 的 `NoRoute` 上）：命中磁盘文件就返回，未命中回落 `index.html`，保留前缀回 JSON 404 |
| Datasource | `backend/internal/datasource/` | 数据源驱动抽象（Driver 接口） |
| Crypto | `backend/internal/crypto/` | AES-GCM 加解密（密钥来自 `SECURITY_KEY`） |

查询链路：handler → service → `query` 包（AST + bun_builder / raw.go）→ datasource 驱动。**SQL 构造红线（参数化、标识符白名单、聚合函数白名单）的完整规则见 `backend/AGENTS.md`「查询处理」**，此处只记横切面：bun_builder 是图表 SQL 的唯一出口，`dialect.go` 的手写字符串 SQL builder 已作为死代码删除，仅保留 `DialectType`/`ParseDialect`/`BuildQueryStringWithBun`。

契约工程（Batch 2/3）：`api/openapi.yaml` 是前后端接口的单一事实源，`make api-gen` 生成 `backend/internal/idls/gen_types.go`（oapi-codegen）与 `frontend/src/idls/gen_types.ts`（openapi-typescript）。**前端运行时类型已切换到生成物打底（Batch 3）**：`frontend/src/api/index.ts` 的实体/响应/请求类型 alias 到 `components['schemas']`，窄联合处用薄手写层（`Omit<G.X,'type'> & { type: Union }`）重收紧；已判死的声明（手写 `typeConfig`——wire 实为 snake `type_config` 且前端零消费、`ColumnInfo` 幽灵字段、`GeneratedSQL` 等）随迁移删除。三条静默红线仍有效：生成 `*Response` 是信封包装（裸 payload 对应 `ChartDataResult` 等去后缀类型，禁按名替换）、`*FormData` 是 UI 数组约定保留手写、store `QueryConfig` 为 camelCase UI 模型不换。后端 handler In/Out 仍为 handler-local 镜像（json tag 由 `contract_parity_test.go` 反射守卫），**未**切换到 `idls.*`——生成 Go 类型无 gin `form:"-"` 语义，直接作 In 会重新引入 query 污染注入面，切换列入 Batch 4 重评。图表 `bi_chart.config` 已升级为带 `version:2` 的文档（`frontend/src/lib/chartConfigSchema.ts` 的 `ChartConfigDocument` + `migrateChartConfig`），旧结构在加载时自动迁移（`fieldId` 由位置 `field-N` 改为列的稳定 id `DatasetColumn.id`），ShareView 据此渲染新结构图表；图表查询请求线不含 `config` 字段（Batch 3 已移除从未生效的 pie 合并比例死链，`query.PieProcessor.MergeOtherBelowRatio` 能力保留但未接线）。

⚠️ **`make api-gen` 的「前端」半步当前必然失败**（不是环境问题）：`openapi-typescript@7.13.0` 依赖 `typescript` 包的运行时导出 `ts.factory`，而本仓 `typescript@7` 没有它 → `TypeError: Cannot read properties of undefined`。崩溃发生在写文件之前（生成物不会被写坏）。绕过方式见 [docs/developer-guide/troubleshooting.md](docs/developer-guide/troubleshooting.md) 的「版本与依赖」（入库）；彻底修法是把生成器与 `typescript@7` 解耦（独立 package.json 或钉一个 TS 5 别名），属独立改动。**后端那半步正常**，且只输出被 cfg 允许的子集（不含 `DashboardFilter*` 等 layout schema）。无论走哪条路，重跑生成后都要 `diff` 生成物，确认只漂了本次改动。

`backend/internal/router/router.go` 的泛型路由**已全面启用并接入 40 个 API 端点**；签名语义、In 镜像 struct、PUT「未提供则保留」约定等细节见 `backend/AGENTS.md`「泛型路由注册」。横切面：PUT 更新遵循**"未提供则保留"**项目约定（datasource 密码、dataset 可选元数据同例），payload 省略/空的可选字段保留存量值而非清零，显式 `"[]"` 仍可清空。

`datasource/` 包实现了 `Driver` 接口用于多数据库后端——新增驱动只需实现该接口。

数据库 schema 由 goose 版本化迁移管理（`backend/migrations/`，通过 `embed.FS` 内嵌），事务统一走 `database.WithTx`。

运维与安全（Sentry / CORS / 加密存储 / share bcrypt）见 `backend/AGENTS.md`「运维与安全」。

### 前端结构

```
frontend/src/
├── api/           # API 端点封装（40 个接口，复用 lib/api/client 的单一 axios 实例）
├── store/         # Zustand 状态管理
├── pages/         # 页面组件
├── components/    # 可复用组件（日期筛选器在 components/DateFilter/，过滤弹窗在 components/ChartBuilder/）
├── idls/          # API 类型定义（现仅 gen_types.ts 生成物；Batch 2 已删除手写 chart/dataset/datasource/share.ts）
├── lib/           # 工具库（API 客户端在 lib/api/client.ts；图表配置 schema 在 lib/chartConfigSchema.ts；日期筛选语义在 lib/dateFilter.ts）
├── i18n/          # 国际化
└── styles/        # 全局样式
```

路径别名：`@/*` 映射到 `./src/*`。

### 日期筛选（components/DateFilter + lib/dateFilter）

对齐火山引擎智能数据洞察的「日期筛选」，供图表查询页与仪表盘盘级筛选器共用：

- **`lib/dateFilter.ts` 是语义单一事实源**（纯逻辑、零 UI 依赖）：五种模式（动态日期 / 固定日期 /
  高级 / 特殊值 / 单个日期）、粒度（年-月-日 / 年-月 / 年-周，datetime 额外给小时）、全套快捷选项、
  周计算逻辑、`resolveDateFilter`（意图 → 具体区间）、`formatDateFilter*`（芯片摘要 / 时间预览）。
- **`DateFilterEditor` 是两个外壳共用的编辑体**：`DateFilterModal`（完整弹窗）与
  `DateFilterControl`（配置完成后留在页面上的行内控件）。不要给两者各写一份编辑逻辑。
- **意图与快照分离，两端各自解析同一个意图**：`FilterCondition.date` 存的是**意图**（`最近 7 天`），
  图表查询页在构造请求时解析（`expandDateFilterIntent`）；后端读 config 的路径（分享页 / 仪表盘）
  在 `internal/query/datefilter.go` 里解析 —— 所以动态日期在所有路径上都是动态的。
  后端那份是 TS 语义的**镜像**，靠双端共读的用例表
  `frontend/src/lib/__fixtures__/dateFilterCases.json` 防漂移（`datefilter_test.go` +
  `dateFilter.cases.test.ts` 各跑一遍）；改语义必须同时改两边。
  `materializeDateFilterSnapshot` 写下的区间快照**已不是取数依据**，只作「不认识 `date` 的下游」的兜底。
- 区间一律输出 `YYYY-MM-DD`（datetime/小时粒度到秒），展示格式只在 UI 层。
  区间口径统一以「昨天」为数据上界（`本周`=本周首日~昨天、`本月`=本月 1 日~昨天）。
- 日期字段拖入「筛选」区进日期筛选弹窗；`FilterCondition.fieldId` 存的是**列的稳定 id**
  （`DatasetColumn.id`，形如 `0000i529`），**不是列名**——列名只用于展示与 SQL 输出别名。

### 仪表盘盘级筛选器

`components/DashboardFilterBlock/` + `lib/dashboardFilterValue.ts`（取值下发契约），
日期族的语义与控件复用 `components/DateFilter/`：

- **三族**（`filterWidgetFamily` ↔ `classifyFieldKind`）：`date` 用日期控件（行内浮层切换 +
  「配置」进完整弹窗）、`string` 用枚举多/单选（候选值实查该列）、`number` 用算子下拉 + 数值输入。
  三条链路的取值下发是**同一条**，只有控件形态与算子词表分族（`FAMILY_OPERATORS`）。
- **布局侧**：`DashboardFilterWidget`（`type: 'filter'`）带 `binding.datasetId + binding.column`
  （**column 是列 ID**，见 `dashboardLayoutSchema.ts` 的注释——盘级条件与图表自身条件的
  「覆盖可见标识」判定靠它求交集）、`label`、`dataType`、`operator`、`multi`、
  `defaultValue`（该筛选器的默认选中值，重开盘时做控件初值；日期族是 `DateFilterValue`，
  其余族是数组）与日期族专有的 `date.granularity/weekStart`。
- **下发侧**：`POST /api/dashboards/{id}/query` 只收**筛选器当前取值** `{widgetId, value[]}`，
  与图表自身条件的合并由后端单点完成（PRD §6.3）。取值形状必须按算子分流：
  `in/notIn` 数组、`between` 两元素、**其余标量算子一元素**、`isNull/notNull` 靠「数组非空」
  表示已激活 —— 见 `lib/dashboardFilterValue.ts` 与后端 `service/dashboard/query.go` 的 `buildOverrides`。
- **两个已知边界**：① 未落库的筛选器后端读不到（盘级取数按已落库 layout 建索引），
  块上会提示「保存仪表盘后生效」；② 盘级筛选每个筛选器每次查询最多产生**一条**合并条件，
  所以「包含空日期」（区间 OR IS NULL）在仪表盘侧表达不出来。

## 常用命令

命令细节（go test 单测、biome 各子命令等）见 `backend/AGENTS.md` 与 `frontend/AGENTS.md`，此处只留高频项：

```bash
make dev               # 前后端同时启动，带热重载（air），前端 23351 / 后端 23352
make serve             # 本地验证单进程形态：构建前端后由后端托管 frontend/dist（单端口 23352）
make build             # 前后端构建
make docker-build / docker-up / docker-down / docker-logs / clean

cd backend && go test ./...        # 后端测试（提交前 go test -race ./...）
cd frontend && pnpm build:check    # 前端提交前验证（biome check + vitest）
```

## 包管理器：只用 pnpm

npm / yarn / bun 会被**硬性拦截**，不是约定而是机制。

- 根目录与 `frontend/` 的 `package.json` 都带 `preinstall` 钩子调用 `scripts/only-pnpm.mjs`。该脚本读 `npm_config_user_agent`，非 pnpm 直接退出码 1 中止安装，并顺手删除 npm/yarn 在 preinstall 之前就写下的 `package-lock.json` / `yarn.lock`。
- `packageManager` 字段钉死 `pnpm@12.4.2`（根 + frontend 一致），`.npmrc` 开启 `package-manager-strict`。⚠️ 若本地 pnpm 版本低于钉死的版本，`package-manager-strict` 会直接拒绝执行 —— 升级到钉死版本即可。
- **依赖事实源只有 `pnpm-lock.yaml`**（根一份、`frontend/` 一份）。其他 lockfile 已在 `.gitignore` 屏蔽，不要提交。
- pnpm 全局 store 默认在本机用户目录下，`frontend/node_modules` 走硬链接。

⚠️ **`pnpm install` 在 lock 未变时直接跳过（"Already up to date"），不会清理孤儿。** `--force` 同样无效。若 `node_modules/.pnpm` 里出现 `pnpm why <pkg>@<ver>` 查不到的包（历史遗留的旧大版本），必须**先删掉 `node_modules` 再 install** 才能真正重建。

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

- 开始工作前先读 [docs/developer-guide/troubleshooting.md](docs/developer-guide/troubleshooting.md)（技术坑，按场景查），并查阅本地工作笔记中的工作方式红线，获取历史经验。
- 主动反思，不依赖用户指出问题。当意识到自己可能犯错、可能需要改进时，就进行反思记录。
- 反思后将根因和教训追加到本地工作笔记（当日 `YYYY-MM-DD.md` 或红线文件），**本机笔记一律写入本地工作笔记目录、不再写入仓库**，避免同类错误重复发生。

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
- **文档同步**: 大型业务逻辑调整或架构调整必须同步更新 `docs/` 下的相关文档（四区分区见 `docs/AGENTS.md`）、`README.md`、`AGENTS.md`。
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

**优先级（低 → 高）：内置默认值 < `.env` 文件 < 真实系统环境变量。项目不提供配置文件。**

- **唯一配置来源是环境变量。** 没有 `config.toml`/`config.yaml` 这类文件（`backend/etc/` 已删除）。理由：容器化部署下每个要用户填的值都必须能从外部注入（`docker run -e` / `docker run --env-file` / compose `environment` / K8s env），再挂一份文件只会制造"改了没生效"的歧义 —— 同一项两处可写、优先级还得记。
- **环境变量文件（本地开发入口）**: 仓库根 `.env`（**不入库**，模板见 `.env.example`）。前后端共用这一份：后端读**裸名** env（无前缀，如 `DATABASE_URL`），前端只消费 `VITE_` 前缀（Vite 由 `vite.config.ts` 的 `envDir` 指向仓库根）。`docker run --env-file .env` 可直接使用，因此文件保持 `KEY=value` 裸格式：不加引号、不写 `export`。
- **后端环境变量**: `PORT` / `DATABASE_URL` / `SECURITY_KEY` / `SENTRY_DSN` / `CORS_ALLOWED_ORIGINS` / `STATIC_DIR`。**刻意不加前缀**：单进程单体没有命名空间要抢，而 `DATABASE_URL` / `PORT` 是 12-factor 标准名，云平台（Heroku / Railway / Render / Fly）会自动注入，裸名能直接吃。监听地址固定 `0.0.0.0`，**不提供** env 覆盖。空字符串一律等同于"未设置"，所以 `.env` 里留空占位不会打掉内置默认值。`DATABASE_URL` 是**唯一必填项**，缺失时启动 fail-fast 并打印变量名，不会退化成驱动层看不懂的连接失败。装配逻辑全在 `internal/config/config.go` 的 `Load()`；`.env` 由 `LoadDotEnv` 装载且**不覆盖**已存在的真实环境变量 —— 这正是优先级的实现方式。启动参数只剩 `-env`（默认依次探测 `./.env`、`../.env`，覆盖"从仓库根启动"与"从 backend/ 启动"）。`SECURITY_KEY` 留空不再降级为明文：首次启动自动随机生成（32 字节 hex）并持久化到数据库 `bi_setting` 表（`internal/keystore`），重启/容器重建复用同一把，保证已加密数据源密码始终可解；显式配置时环境变量优先。
- 前端 API 基础 URL: **唯一来源**是 `frontend/src/lib/api/client.ts` 的 `resolveApiBaseURL`（`api/index.ts` 只消费该实例，**不得再 `axios.create`**）。两档语义：`VITE_API_BASE_URL` 配了非空绝对地址则直接采用（前后端分开部署的逃生口）；未配置则生产构建自动同源（请求路径自带 `/api` 前缀，baseURL 留空串）、开发回退 `http://<当前访问主机名>:23352`（后端 CORS 默认放开，换自定义域名无需任何配置）。⚠️ `VITE_*` 是**构建期内联**进 JS 的，改完必须重新 build 才生效，也正因如此**绝不可**在里面放密钥。
- 前端 Sentry DSN: `VITE_SENTRY_DSN`（**构建期内联**）。未设置则 `main.tsx` 完全跳过 `Sentry.init`，不产生任何上报。与后端 `SENTRY_DSN` 是两个独立项目。
- **单镜像单进程（`Dockerfile`，构建上下文 = 仓库根目录）**: 三阶段 —— `node:24-alpine`（Active LTS）构建前端产物 → `golang:1.27-alpine` 编译后端 → `alpine:3.24` 运行（二进制 + `dist` 一起拷进去）。**没有 nginx**：Go 进程自己按路径分流，同时提供 API 与页面，对外只有一个端口 23352。
  - 分流规则（`internal/webui`）：`/api`、`/mcp`、`/health` 前缀 → 后端路由；命中不到就是 JSON 404（**绝不回落成 HTML**）。其余路径 → `STATIC_DIR`（镜像内 `/app/web`）里的文件，未命中回落 `index.html` 交给前端路由。
  - 缓存策略继承原来的 nginx 配置语义：`assets/` 下带内容哈希的产物 `immutable` 长缓存，其余 `no-cache`。
  - 静态目录**配错即启动失败**（`webui.New` 读不到 `index.html` 就退出），不会跑起来之后整站 404。
  - 镜像自带的只有 `STATIC_DIR=/app/web`（代码默认值是"空 = 只提供 API"，必须显式指）；监听地址/端口的内置默认值本就是 `0.0.0.0:23352`，无需重复声明。其余全部运行时注入。
- `docker compose`（唯一一份，`docker-compose.allinone.yml`）: PostgreSQL（端口 5432，本地体验用）+ **一个 app 服务**（23352，前后端同容器），app 用 Docker Hub 发布镜像 `kzzhr/datainsights:${TAG:-latest}` **不构建**；生产环境换外部高可用 PostgreSQL 只跑 app（见 README）。镜像由 `.github/workflows/release.yml` 手动触发发布（校验版本号 + 双端测试 → 冒烟 → 推 amd64/arm64 镜像到 Docker Hub → 打 tag + 建 Release），运维手册见 `docs/deployment/release.md`。
- 本地不起 Docker 也能验证同一形态：`make serve`（构建前端产物后 `STATIC_DIR=../frontend/dist go run ./cmd`）。不设 `STATIC_DIR` 时后端退化为纯 API 服务，此时 `/share/:token` 才会注册。
- ⚠️ **`frontend/vite.config.js` / `.d.ts` 是 tsc 产物，且会遮蔽 `vite.config.ts`**（Vite 解析 `vite.config.js` 优先于 `.ts`）。曾出现改了 `.ts` 却完全不生效的情况。发现配置改动「没反应」时先 `ls frontend/vite.config.*`，删掉这两个文件再验证。

## 开发资源

| 文档 | 说明 |
|------|------|
| [docs/getting-started/overview.md](docs/getting-started/overview.md) | 核心概念、应用场景与对外 roadmap |
| [docs/getting-started/quick-start.md](docs/getting-started/quick-start.md) | 极简上手（四条部署路径：预览站 / 单条 Docker / Compose / 源码） |
| [docs/user-guide/configuration.md](docs/user-guide/configuration.md) | 配置与参数字典 |
| [docs/user-guide/features.md](docs/user-guide/features.md) | 核心功能使用说明（含「为什么做这个功能」） |
| [docs/deployment/docker.md](docs/deployment/docker.md) | 容器镜像（发布镜像 / 本地构建）、Docker Compose |
| [docs/deployment/release.md](docs/deployment/release.md) | 发布流程（打 tag + 推镜像到 Docker Hub + 建 Release） |
| [docs/deployment/production.md](docs/deployment/production.md) | 生产环境部署实践 |
| [docs/developer-guide/architecture.md](docs/developer-guide/architecture.md) | 系统架构 + 关键设计决策 |
| [docs/developer-guide/api.md](docs/developer-guide/api.md) | API 接口文档（叙述性视图；契约事实源是 api/openapi.yaml） |
| [docs/developer-guide/chart-query-design.md](docs/developer-guide/chart-query-design.md) | 图表查询链路设计（契约模型 / SQL 生成 / 处理器） |
| [docs/developer-guide/troubleshooting.md](docs/developer-guide/troubleshooting.md) | 排障（外部开发者也会踩的坑） |
| [docs/developer-guide/backlog.md](docs/developer-guide/backlog.md) | 活跃待办 / 方言能力矩阵 |
| [docs/developer-guide/dev-setup.md](docs/developer-guide/dev-setup.md) | 环境搭建、运行命令 |
| [docs/developer-guide/design-system.md](docs/developer-guide/design-system.md) | 前端表面系统规范（页面骨架、色彩、字体、组件样式） |
| [docs/developer-guide/contributing.md](docs/developer-guide/contributing.md) | 贡献流程 + 代码风格 + 提交前门禁 |

分区索引与维护规则见 `docs/AGENTS.md`。目录另存有一部分**不入库**的本机材料（历史规划、竞品调研等），只作本地参考；入库文档不得链接这类本机文件。

## 已知限制

- 后端无 golangci-lint 配置
- 缺少端到端测试
- **数据源方言能力验证不均衡**：4 种外部数据源（postgresql/mysql/clickhouse/starrocks）连接与元数据读取均已实现，但**只有 PostgreSQL 有真实能力懒探针**。MySQL/StarRocks 已升级为"只升不降 + nil 保底"探针（无实例时行为等于保守基线，不回归），ClickHouse 刻意保持静态（剩余布尔能力已是文档-true、percentile 需语义校验）。受能力门控的图表落地参差：**boxplot 仅 PG 端到端可用**（StarRocks percentile 策略已实测但翻转需实例；CH 待语义校验；MySQL 无标量 percentile 路径暂不支持）；pivot 全源可用（GROUPING SETS 缺失时自动回退 UNION ALL）。不要向未列明实例验证的后端过度声称图表支持。

