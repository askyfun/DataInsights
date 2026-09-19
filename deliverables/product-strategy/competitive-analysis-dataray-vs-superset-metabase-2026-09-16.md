# DataRay 竞品分析（两轮 | 三阵营 25 家）

> **⚠️ 标题与文件名说明**：文件名沿用 `vs-superset-metabase`（第一轮范围），**内容已扩展至三阵营 25 家**（开源 9 / 国内 11 / 国际+AI 原生 8）。第二轮见文末「**第二轮竞品调研（2026-09-19）**」起（§十四~§十八）。为遵守文档集「禁止再建增量修订版本文档」的纪律，**未另建文件**；如需与文件名对齐，可整份重命名为 `competitive-analysis-dataray-2026-09-19.md`（**未擅自执行，待产品负责人裁定**）。

**日期**：2026-09-16（第一轮）· **2026-09-19（第二轮并入）**
**类型**：竞品分析 + 改造需求规划（工作流 1 × 工作流 2 合并）
**参与成员**：竞析（竞品分析师）／瑞思（用户研究员）／数析（数据分析师）／析客（需求分析师）／路径（路线图规划师）
**主理人**：方向明（产品舵手）
**效力范围**：第一轮（§一~§十三）针对「Superset / Metabase 对标 + 五阶段路线」，其**排期结论已被 `roadmap-dataray-2026-09-16.md` v4.0 取代**；**能力域差距矩阵与门槛能力清单仍在效**。第二轮（§十四~§十八）为 2026-09-19 最新竞品定位结论。

---

## 📌 TL;DR（执行摘要）

- **核心答案**：DataRay 缺的不是图表能力，而是 **9 项"入门门槛级"（table-stakes）能力**——身份与权限、看板容器、指标语义层、自由 SQL、调度与订阅、缓存加速、嵌入、审计、数据源广度。图表类型只是其中最小的一块。
- **最致命的三件事**：① 无登录、任何人打开即全权限（全仓 `jwt|session|login|user_id|tenant` **0 处命中**）——IT 侧直接判"不能上线"；② 无仪表盘（只有单图分享）——分析师的产出**交付不出去**，只能发 8 个链接；③ 无指标层——同一指标各图各写，"数字对不上一次就永久回 Excel"。
- **关键决策**：不要正面自研全栈。SQL Lab、调度/订阅告警、权限治理、缓存层这四块自研投入**不亚于重做一个 Superset 后端**且无差异化，应依赖成熟组件；把差异化压在 **指标语义层 + QueryAST 安全下推 + Go 轻部署**（这正是 Superset 的软肋）。
- **规模与节奏**：全量改造 **约 254–368 人日（估算）**，分 5 个阶段。**最短见价值路径约 50–65 人日**（认证 + 看板容器，2 人并行约 5–7 周）即可对外演示"登录后访问 8 图看板"。
- **下一步**：先拍板 7 个待确认决策（认证方式、多租户模型、调度选型、指标层落点、缓存后端、部署形态、是否开放插件图表），其中"调度选型 + 部署形态"直接决定后三个阶段节奏。

---

## 🎯 核心结论卡片

| 项目 | 内容 |
|------|------|
| 推荐方案 | 五阶段推进：**P0 身份地基 → P1 可信消费 → P2 探索分发 → P3 嵌入与广度 → P4 治理与智能**；门槛能力（权限/调度/缓存/SQL 编辑器）依赖成熟组件而非自研 |
| 优先级 | **P0 = 账号权限（R-01~R-03）+ 看板容器（R-04）+ 指标语义层（R-06）** —— 三者缺一，前四类用户流失都堵不住 |
| 预期影响 | 加权验收总分率从现状 **≈0.29** 提升到 **≥0.75**（且每个 P0 能力域 ≥70%），即"可被企业真正采用"而非"功能像 BI" |
| 资源需求 | 全量 **254–368 人日（估算）**；关键路径串行段 R-01→R-02→R-03 约 35–45 人日；最短可演示里程碑 50–65 人日 |
| 风险等级 | **高**（主要风险：RLS 经 `raw.go` 旁路逃逸、31 端点鉴权接线回归、引入 Redis/worker 破坏"轻部署"叙事） |

---

## 一、结论先行：一句话回答"还要做哪些改造"

> DataRay 已经完成了 BI 的**"画图"部分**（数据源 → 数据集 → 图表），现在缺的是把它变成**"组织级系统"**的四层：**身份与权限层、消费容器层（看板）、口径语义层、触达与性能层**。继续加图表类型是收益最低的方向。

支撑这个判断的是两条独立证据链，它们指向同一个结论：

| 视角 | 判断 | 指向的优先级 |
|------|------|--------------|
| 竞品基线（竞析） | 对标 Superset 6.0 / Metabase 63，DataRay 在 12 个能力域中**仅 1 个（可视化）达到及格线** | 门槛能力 9 项需补齐 |
| 用户门槛（瑞思） | 缺口不在"图表能力"，在"从能用到被采用"；五个角色的流失点全部落在身份/看板/口径/性能上 | 账号权限 → 口径可信 → 看板消费 → 触达性能 |

---

## 二、竞品全景（来自竞析）

**对标版本已联网核实（2026-09）**：Apache Superset **6.0.0**（2025-12，Apache 2.0，ASF，GitHub ≈7.2 万星）；Metabase **63**（2026-07，LTS=58 / 2025-12，AGPL 核心 + 付费 EE）。

### Apache Superset 6.0 — 平台团队主导的 warehouse-native 自助 BI
- **杀手能力**：① SQL Lab（多方言、autocomplete、异步执行、查询历史、结果一键转图表/虚拟数据集、Jinja 宏 `current_username()` / `filter_values()` 做动态筛选与 RLS 注入）；② **开源版即含 RBAC + 行级安全 RLS（SQL predicate）+ OAuth/SAML/LDAP + SIP-152 安全组**；③ 80+ 数据源（SQLAlchemy）；④ 40–50+ 图表、deck.gl 地理、AG Grid 大表（50 万行服务端分页）；⑤ Redis 结果/缩略图/看板缓存；⑥ 插件式自定义图表。
- **短板**：① **运维重**——metadata DB + Redis + Celery worker，生产基本要上 K8s；② **语义层弱**——metrics/columns 只是 dataset 内的表达式，非 metrics-as-code，无强制单一事实源；③ 无官方托管；④ 审计能力有限；⑤ 原生多租户需变通。

### Metabase 63 — 面向业务人员的 no-code 自助分析
- **杀手能力**：① 拖拽查询构建器（免 SQL）+ X-ray 自动洞察；② **嵌入式分析最强**——React SDK、modular embedding、guest/static embed、v58 tenants 多租户 + v59 JWT SSO 租户下发；③ **Metabot AI**（自然语言问数 / 生成与调试 SQL / 解释图表）；④ Data Studio（v59，SQL/Python 建模 + 依赖图 + git 同步语义库）。
- **短板**：**付费墙分明**——RLS / data sandboxing、SAML SSO、audit logs、高级嵌入均在 Pro/Enterprise；语义层（segments/metrics）偏弱。

### 其他参照
| 产品 | 定位与关键特征 | 对本项目的参考意义 |
|------|----------------|--------------------|
| Redash | SQL-first、50+ 源、查询结果复用、告警；已被 Databricks 收购进入**维护模式** | 反面教材：优秀但停止演进 |
| Lightdash | **dbt 原生语义层**，指标定义在 dbt YAML + git 治理，单一事实源最强 | 语义层的正向标杆（不必绑定 dbt） |
| Grafana | 可观测性非 BI，100+ 源、实时统一告警、SLO 一流；**无中心语义模型** | 告警/缓存/多源的工程参考 |
| Power BI / Tableau / Looker | 商业，成熟语义层（DAX/LookML）+ RLS/OLS + 企业治理 + 移动端 | 治理与移动端的能力上限参照 |
| Quick BI / FineBI / 网易有数 | 国产：大模型问数、行级权限/水印/脱敏、钉钉企微飞书多端、中国式报表、信创 | **国产化 + 中国式报表**是 DataRay 可切的市场缝隙 |

---

## 三、能力域对比矩阵（来自竞析）

| 能力域 | DataRay 现状 | Superset 6.x | Metabase 63 | 判定 |
|---|---|---|---|---|
| 数据连接 / 源广度 | 4 驱动（PG/MySQL/CK/SR） | 80+ | 20–25+ | table stakes（广度是门槛；MVP 4 个够） |
| 数据集 / 建模 / 语义层 | 数据集 + 字段元数据；无 join / 计算字段 / 指标定义 | dataset + metrics（弱）、虚拟数据集、dbt manifest | models + segments/metrics（弱），Data Studio 增强 | **differentiator（DataRay 的机会）** |
| 自由 SQL + 结果复用 | ❌ 无 | SQL Lab（强） | native query（中） | table stakes |
| 图表类型 / 配置深度 | 7 种基础图 | 40–50+ | 若干 + 自定义表达式 | table stakes（基础）/ differentiator（深度） |
| **仪表盘 / 看板组织** | ❌ 仅单图 | 布局 + 原生筛选 + 跨图联动 + Tab/行嵌套 + 钻取 | 看板 + 筛选 + 交叉筛选 + Tab | **table stakes（最核心）** |
| 订阅与告警 | ❌ 无 | Reports & Alerts（Celery） | alerts + subscriptions | table stakes |
| 定时刷新 / 物化 / 缓存 | ❌ 无 | Redis 缓存 + 异步查询 + GAQ | 预判式缓存（v53）+ transforms | table stakes（缓存） |
| **权限与治理** | ❌ 无登录 | RBAC + RLS + 安全组 + OAuth/SAML/LDAP + 审计（弱） | RBAC（社区）+ RLS/SAML/audit（付费） | **table stakes** |
| 分享与嵌入 | 单图链接 + bcrypt 密码 | 公开链接 + iframe + Guest Token SDK + RLS 兼容 | **最强**：React SDK / modular / guest / 多租户 | table stakes（链接）/ differentiator（SDK + 多租户） |
| AI 辅助 | ❌ 无 | 核心未内置成熟 NL2SQL（**未核实**） | **Metabot 强** | differentiator |
| 可观测与运维 | 无查询日志 / 慢查询 / 隔离 | 查询日志 + SQL 历史 + 异步隔离 | 查询日志 + 诊断 | table stakes（查询日志） |
| 扩展机制 | OpenAPI + 泛型路由 | 插件图表 + REST API + CLI | React SDK + API + plugins | differentiator |

### 门槛能力的移植成本评估（竞析）

| # | table-stakes 门槛 | 竞品实现方式 | 移植/自研成本 |
|---|---|---|---|
| 1 | 登录 / 用户 / 角色 / 组 + SSO | Superset: Flask-AppBuilder + security groups（SIP-152）；Metabase: RBAC + collections（SAML 付费） | **高**（认证中间件/OIDC 须与现有 31 端点全线挂钩） |
| 2 | 行级权限 RLS | Superset: SQL predicate + `current_user_rls_rules()`；Metabase: data sandboxing（付费） | **高** |
| 3 | 仪表盘（多图 / 布局 / 全局筛选 / 联动 / Tab 嵌套） | 各家核心能力 | **高**（新前端画布 + 跨图状态 + 查询编排） |
| 4 | 自由 SQL（SQL Lab）+ 结果复用 | Superset SQL Lab / Metabase native query | 中高（编辑器 / 历史 / 结果落地 / 权限拦截 / 防注入） |
| 5 | 订阅告警 + 调度基建 | 均依赖 **Celery/Redis/worker**；DataRay 无 worker | **高**（需引队列 + 调度 + 通知通道） |
| 6 | 缓存 / 加速层 | Superset: Redis；Metabase: 预判式缓存 | 中高 |
| 7 | 分享 / 嵌入（iframe / SDK / 多租户 / 白名单） | 公开链接 + 密码已有，余下需补 | **高** |
| 8 | 查询日志 / 审计 | 各家内置 | 中 |
| 9 | 数据源广度 | Driver 抽象已在，加驱动即可 | **中低**（边际成本最低的一项） |

---

## 四、DataRay 现状的事实基线（主理人取证）

这一节不是推断，是对仓库实测的结果，作为差距矩阵的对照锚点。

**已具备（4 个领域，31 个 API 端点）**
- `datasource`（11 端点）：`Driver` 接口抽象 + 4 驱动（PostgreSQL / MySQL / ClickHouse / StarRocks）；密码 AES-GCM 加密存储，API 响应不回传明文。
- `dataset`（9 端点）：数据集与字段元数据管理。
- `chart`（7 端点）：拖拽式图表构建（表格 / 柱状 / 折线 / 面积 / 饼图 / 散点 / 透视表）；维度展示名、指标聚合/别名/单位/格式化、样式（颜色/线型/行高）、饼图"其他"合并。
- `share`（4 端点）：单图表分享链接 + bcrypt 密码（`/share/{token}` 为 302 重定向）。

**已建立的良好工程底座（应继续加注）**
- `api/openapi.yaml` 为前后端契约单一事实源，`make api-gen` 生成双端类型。
- 泛型路由 `RegisterRoute[In,Out]` 覆盖 31 端点 + 统一响应信封 `{code,msg,trace,data}`。
- `query` 包为 SQL 唯一出口，全参数化执行 + 标识符白名单 + 聚合函数白名单（`count|sum|avg|min|max`）。
- `ChartSpec` / `QuerySpec` / `QueryAST` / `QueryPlanner` 第一阶段已落地——**这是差异化支点**。
- Sentry、requestID 中间件、CORS 配置化、goose 版本化迁移、`database.WithTx` 事务。

**缺失取证（本次实测，非推断）**
| 取证动作 | 结果 | 结论 |
|---|---|---|
| 全仓（`*.go`/`*.toml`/`*.yml`/`*.yaml`/`*.json`）搜索 `redis\|asynq\|celery\|cron` | **0 处命中** | 无缓存层、无调度、无 worker（R-07/R-09/R-12 是真实缺口） |
| `backend/internal/**` 搜索 `jwt\|session\|login\|user_id\|tenant` | **0 处命中** | 无任何账号/租户字段与方法，31 端点当前"任何人全权限"（R-01~R-03 是真实缺口） |
| `backend/migrations/` 内容 | 仅 **1 个** 迁移（`00001_init_schema.sql`） | 数据模型无租户/归属维度，T-02 的迁移是真实工作量 |
| `frontend/src/pages/` 内容 | 9 个页面，**无登录页、无看板页、无 SQL 页** | 消费容器与自由查询在前端完全空白 |

---

## 五、用户视角：为什么这些改造是"必须"（来自瑞思）

### 5.1 五类角色与卡点

| 角色 | 要完成的任务 | 当前卡点 | 对 DataRay 的直接反应 |
|------|--------------|----------|----------------------|
| 数据分析师 / BI 开发 | 接需求 → 找表 → 写逻辑 → 交付可视化 | 无指标/口径层（同指标每图重写）；无仪表盘（只能一张张发链接）；无目录（产出散落） | "图表够用，但交付链断了，是半成品" |
| 业务人员（运营/销售/市场） | 每天看自己那一亩三分地；临时拉数 | 无登录=人人看全量（安全不允许）；无全局筛选（筛不到"我部门"）；无移动端/收藏/推送 | 单图分享点一次就再不回来 |
| 管理者 / 决策者 | 临时追问一个数；看大盘；拍板 | 无看板（无法一屏看全）；无订阅推送；查询要等；口径不统一 | 用一次就弃（"看不到全景 + 数字和别处对不上"） |
| 数据工程师 / 数仓 | 保数据准、护住库、别被 BI 打爆 | 无账号体系→无法做行列级权限；无缓存→每次真查库 | 会明确反对"在没权限和缓存之前开放给业务" |
| IT / 平台管理员 | 安全合规、可管可审计 | 任何人打开即全权限，合规红线 | 直接判定"不能上线" |

### 5.2 三个关键场景的断点

- **场景 A｜分析师从接需求到交付一个看板**：接需求 → 配数据源/数据集（✅）→ 做图（✅）→ **多图拼成一个看板（❌断）** → **全局筛选让业务自己筛（❌断）** → **配权限发给对应的人（❌断 / 无账号）** → **定时推送（❌断）**。图做完了却没有"交付容器"，只能发 8 个链接。
- **场景 B｜业务人员每天早会前看数**：**找入口（❌无登录、无收藏/搜索）第 1 步就断** → 打开要等（❌无缓存）→ 看到全量而非"我的区域"（❌无权限/联动）→ 想换维度（❌单图无联动）→ 手机上瞅一眼（❌无移动端）。
- **场景 C｜管理者临时追问一个数字**：要"华东上月 GMV" → **去看板看（❌无看板）** → 转问分析师临时做图（等）→ **数字和财务口径对不上（❌无指标层）** → 让人导 Excel 自己算。典型"信任崩塌"路径：对不上一次，就永久回 Excel。

### 5.3 采用门槛分级

- **致命门槛（没有它 → 直接不用 / 无法上线）**：① 登录与账号体系（IT/安全红线，中大型企业直接否决）；② 用户/角色/权限（含行级）；③ 口径可信 / 指标层（同一指标全公司一个数）；④ 看板级消费（多图拼装 + 全局筛选联动，**当前最大的产品级缺口**）；⑤ 性能（10–15 秒加载 = 被弃用）。
- **重要门槛（能用起来，但不爱用）**：⑥ 目录/收藏/搜索；⑦ 推送与提醒（订阅/告警，养成习惯的关键）；⑧ 移动端适配；⑨ 导出/Excel 兼容（不要试图取代 Excel，要让它"在受控环境里用"）。
- **加分项（有了才愿意推荐给别人）**：查询缓存与加速、看板/图表模板、协作与注释、审计日志、SSO、分享链接水印/有效期。

### 5.4 用户流失风险排序（按当前现状）

1. **业务人员"第 1 步就流失"（最高危）**——无登录入口 + 无目录收藏 + 无推送，复访 ≈ 0。
2. **IT / 管理者"上线即否决"**——无权限/无账号是合规红线，项目还没开始就卡死。
3. **决策者"信任崩塌"**——无指标层 + 无看板 + 数字对不上，一次即永久回 Excel。
4. **分析师"交付不了"**——无看板容器，工具价值无法闭环。
5. **数仓"不敢放开"**——无缓存 + 无权限，并发压库 + 安全风险，会主动限制使用。

> **倒推含义**：**P0 = 账号权限 + 看板 + 指标层**（任缺其一，前 4 类流失都堵不住）；**P1 = 全局筛选联动 + 缓存 + 目录搜索**；**P2 = 推送 / 移动端 / 模板**。

---

## 六、差距矩阵（来自析客）

| 能力域 | 权重 | DataRay 现状（缺什么实体/接口/组件） | 对标要求 | 差距等级 | 改造项 |
|---|---|---|---|---|---|
| 数据接入 | 12% | 4 驱动已够 MVP；无连接池治理 / 敏感字段脱敏 | 广度是门槛 | 一般 | R-14 |
| 建模 / 语义层 | 13% | 无 metric / dimension 实体，无 join / 计算字段，无口径单一事实源；仅有 QuerySpec/AST/Planner | 强语义层 = 差异化 | **致命** | R-06 |
| 查询与探索 | 13% | 无自由 SQL 编辑器 / 历史 / 结果复用，无行数上限保护 | SQL Lab | **致命** | R-05（联动）、R-10（SQL Lab） |
| 可视化 | 12% | 7 图；无导出 | 核心集足够 | 一般 | R-16 |
| 看板 | 12% | 无 dashboard 实体、无布局画布、无全局筛选 / 联动 | 最核心 | **致命** | R-04, R-13 |
| 权限治理 | 12% | 无 user / role / RLS / 审计，任何人全权限 | RBAC + RLS 是红线 | **致命** | R-01, R-02, R-03, R-08 |
| 协作分发 | 10% | 仅单图链接 + bcrypt；无订阅 / 告警 / 嵌入 | 触达养成习惯 | 重大 | R-07, R-10, R-11 |
| 性能与缓存 | 8% | 无缓存 / 物化 / 异步查询，每次真查库 | 命中率 ≥70% | **致命** | R-09, R-12 |
| 扩展与嵌入 | 5% | 无 API Key / Guest Token / iframe | SDK / 嵌入 | 重大 | R-11, R-15 |
| 运维可观测 | 3% | 无查询日志 / 慢查询 / 成功率看板 | 查询日志 | 一般 | R-08, R-17 |

---

## 七、改造需求池（来自析客）

### P0（不做就不能上线 / 不被采用）

**R-01 账号与认证体系** `[L, 15–20 人日·估算]` `[依据: 瑞思#致命① / 数析权限域]`
- 方案：新增 `service/auth`、`domain/entity/user.go`、`middleware/auth.go`（JWT，预留 OIDC 接口）；表 `users(id,tenant_id,email,password_hash,status)`、`roles(id,code,name)`、`user_roles`、`refresh_tokens`；端点 `POST /api/auth/login|logout|refresh`、`GET /api/auth/me`；前端 `pages/Login` + store 增 auth slice。
- 验收：未登录 `GET /api/datasets` → 401；admin 建 A/B 后，A 登录仅见 A 资源（证据：两账号录屏）。依赖：无。

**R-02 RBAC 授权** `[M, 8–10 人日·估算]` `[依据: 瑞思#致命② / 竞析 table-stakes]`
- 方案：`service/authz`（Casbin 或自研 policy）；表 `permissions`、`role_permissions`、`resource_grants(resource_type,resource_id,subject)`；在 `router` 泛型注册层统一注入鉴权装饰器，31 端点全覆盖，白名单仅 `/health` + `/share/{token}`。
- 验收：viewer 角色调 `POST /api/datasets` → 403；权限矩阵单测全绿。依赖：R-01。

**R-03 行级安全 RLS** `[L, 12–15 人日·估算]` `[依据: 瑞思#致命② / 竞析差异化③]`
- 方案：`QueryAST` 增 `rls_predicates []Predicate`，`QueryPlanner` 在 build 阶段 merge 进 WHERE，`sanitizer` 校验 predicate 仅用白名单列/算子；表 `rls_rules(id,dataset_id,subject_type,subject_id,predicate_sql)`；`raw.go` 路径同等拦截。
- 验收：A 仅华东、同一 dataset B 见全量；predicate 引用非白名单列必须拒绝。依赖：R-01, R-02。

**R-04 仪表盘（多图看板）** `[XL, 30–40 人日·估算]` `[依据: 瑞思#致命④ / 竞析最核心 table-stakes]`
- 方案：新 `service/dashboard`；表 `dashboards(id,tenant_id,owner_id,title,layout_json)`、`dashboard_items(id,dashboard_id,chart_id,pos_json)`；端点 `GET/POST /api/dashboards`、`GET/PUT/DELETE /api/dashboards/{id}`、`PUT /api/dashboards/{id}/items`；前端新 `pages/Dashboard` + `DashboardEdit`，`@dnd-kit` 网格画布，store 增 dashboard slice。
- 验收：8 张图拼装保存后刷新持久化；首屏热缓存 P95 ≤2s。依赖：R-01。

**R-05 全局筛选与跨图联动** `[L, 15–20 人日·估算]` `[依据: 瑞思#致命④ / 数析看板域]`
- 方案：看板级 `filters_json`，前端筛选组件把变量下发、参数化注入各卡 QuerySpec；引入类 Jinja 宏占位（`{{filter.dept}}`）在 `query` 包预编译层解析。
- 验收：看板选"华东"后所有卡同步过滤。依赖：R-04。

**R-06 语义层 / 指标定义（差异化收口）** `[L, 18–25 人日·估算]` `[依据: 瑞思#致命③ / 竞析差异化①]`
- 方案：`service/metric`、`domain/entity/metric.go`；表 `metrics(id,dataset_id,name,expr,agg,format,synonyms)`、`dimensions`；`QuerySpec` 增 `metric_refs`，`QueryPlanner` 将 metric 编译为 AST 片段（复用现有 `sanitizer` 白名单）。
- 验收：定义 GMV，改口径后所有引用图一次生效（证据：改一处、看板全变）。依赖：R-01。

**R-07 订阅与告警** `[L, 12–18 人日·估算]` `[依据: 瑞思#重要⑦ / 竞析 table-stakes]`
- 方案：`service/alert`；表 `alerts(id,chart_id,cron,threshold,channel,recipients)`、`alert_runs`；端点 `POST/GET /api/alerts`；渠道先邮件/webhook。
- 验收：阈值命中且调度跑完后收到一次推送（邮件/webhook 截图）。依赖：R-12。

**R-08 查询日志与审计** `[M, 6–8 人日·估算]` `[依据: 瑞思#加分项 / 数析运维域]`
- 方案：中间件写 `query_logs(id,user_id,dataset_id,sql_hash,duration_ms,row_count,status,trace_id)` + `audit_logs`；端点 `GET /api/logs/queries`。
- 验收：后台可见查询成功率与慢查询 Top10。依赖：R-01。

**R-09 查询缓存与加速** `[L, 12–18 人日·估算]` `[依据: 瑞思#致命⑤ / 数析命中率 ≥70%]`
- 方案：引入 Redis；缓存键 `hash(dataset_id, spec_canonical, tenant_id, rls_scope, data_version)`，`data_version` 由刷新递增；命中路径在 `executor.go` 前短路。
- 验收：同查询二次命中缓存（Redis keyspace 截图 + P50 降幅）。依赖：R-01。

### P1（能用，但不爱用）

**R-10 SQL Lab 自由查询** `[L, 15–20 人日·估算]` `[依据: 瑞思#场景A / 竞析 table-stakes]`
- 方案：`service/sqlrunner`（仅白名单库/表、强制 LIMIT、超时）；端点 `POST /api/sql/execute`、`GET /api/sql/history`；前端 `pages/SqlLab`（CodeMirror + autocomplete）。依赖：R-01, R-02, R-12。

**R-11 嵌入与开放** `[L, 12–18 人日·估算]` `[依据: 竞析 differentiator / 瑞思加分项]`
- 方案：表 `api_keys`；Guest Token 签名（JWT）；`/embed/{token}` iframe 路由 + RLS 兼容；`GET/POST /api/api-keys`。依赖：R-03。

**R-12 调度基建 + 定时刷新 / 物化** `[L, 15–20 人日·估算]` `[依据: 竞析结论④ / 瑞思#重要⑦]`
- 方案：新增 `cmd/worker`（asynq + Redis），与 API 同镜像不同入口；表 `schedules(id,dataset_id,cron,mode)`；物化表 `mat_*` 或 PG 物化视图；异步查询迁移到 worker；**保留单机 compose all-in-one** 以免破坏"轻部署"叙事。依赖：R-09。

**R-13 多租户 / 工作空间** `[L, 12–20 人日·估算]` `[依据: 竞析差异化② / 用户门槛]`
- 方案：所有业务表补 `tenant_id`（见 T-02）；登录后按 tenant 隔离；表 `workspaces`。依赖：R-01。

**R-14 数据源广度（+驱动）** `[S/个, 3–5 人日·估算]` `[依据: 竞析 table-stakes]`
- 方案：实现 `datasource/driver.go` 接口新增驱动（Oracle / SQLServer / Doris 等）。依赖：无。

**R-15 目录 / 收藏 / 搜索** `[M, 6–10 人日·估算]` `[依据: 瑞思#重要⑥ / 数析 TTFV]`
- 方案：表 `favorites(subject_id,resource_type,resource_id)`；端点 `GET /api/search?q=`、`POST/DELETE /api/favorites`；前端顶栏搜索。依赖：R-01。

**R-16 导出增强** `[S, 3–5 人日·估算]` `[依据: 瑞思#重要⑨ / 数析可视化域]`
- 方案：`POST /api/charts/{id}/export?format=png|csv`（后端 headless 渲染或前端 canvas 导出 + 服务端 CSV）。依赖：无。

**R-17 移动端适配** `[M, 8–12 人日·估算]` `[依据: 瑞思#重要⑧]`
- 方案：响应式断点 + 看板卡片流式布局 + 拇指热区。验收：375px 宽下看板可滚动查看、筛选可用。依赖：R-04。

### P2（治理与智能）
- **R-18 运维可观测**（查询成功率 / 慢查询 Top10 看板 + Sentry 关联 `trace_id`）`[M, 6–10 人日]`
- **数据血缘 / 轻量目录**（表 `lineage_edges`）`[L, 15–25 人日]`
- **版本化 / 发布评审**（`revisions` 表 + 草稿/发布态）`[M, 8–12 人日]`
- **AI 辅助 NL2SQL**（LLM → QueryAST，受 `sanitizer` 约束）`[L, 15–25 人日]`

---

## 八、技术改造项（不直接对应用户可见功能，但必须做）

| 编号 | 改造项 | 落地方案 | 风险 |
|---|---|---|---|
| **T-01** | 统一认证中间件接线 | `middleware.Auth()` 在 router 泛型注册处统一注入，定义 401/403 语义，公共白名单 `/health` + `/share/{token}` | 31 端点回归 → 扩充现有 parity 契约测试兜底 |
| **T-02** | 数据模型租户/归属迁移 | 所有业务表加 `tenant_id` / `owner_id` / `created_by`；goose 增量迁移"先加列(可空) → 回填 → 加约束"，避免大表锁 | 存量数据归属决策（需用户拍板） |
| **T-03** | RLS 在 AST 层注入位置 | `QueryAST.rls_predicates` 由 `QueryPlanner` build 阶段 merge；`sanitizer` 二次校验 | `raw.go` 旁路需同等拦截（最高危逃逸面） |
| **T-04** | 缓存键设计 | `dataset_id + canonical_spec + tenant + user(RLS 时) + data_version`；`data_version` 递增失效 + TTL | RLS 下 key 空间放大，命中率可能 <70% |
| **T-05** | 调度 / worker 引入 | `cmd/worker` + asynq + Redis，同镜像不同入口；保留单机 all-in-one compose | 部署拓扑变重，可能被迫上 K8s |
| **T-06** | 迁移与灰度 | goose 版本化 + feature flag（按租户） + 双写过渡；**禁止 big-bang** | 迁移窗口与回滚准备 |

---

## 九、改造路线图（来自路径）

路径**保留**析客的批次骨架（依赖方向正确），但做 4 处修正：① R-08 审计从 Batch1 提前到 P0/P1 并行（只依赖 R-01，且是合规证据）；② P1 内部明确"**先 R-04 容器、再 R-06 口径**"顺序（metric 需要容器落地）；③ R-09 缓存放 P1 末尾（spec 形态由 R-04/05/06 决定后才稳定）；④ P2 以 R-12 调度为前置，R-09 → R-12 合并为一条基建并行轨。

### 9.1 路线图总览

| 阶段 | 主题 | 关键交付 | 覆盖需求 | 出口 Gate（证据制） | 关键路径依赖 | 风险 |
|---|---|---|---|---|---|---|
| **P0 身份地基** | 能上线、不违规 | 登录态、RBAC、RLS | R-01, R-02, R-03, (R-08), T-01~T-03 | 未登录访问 `/api/datasets` = 401；viewer 建库 = 403；A 见华东 / B 见全量；parity 契约测试全绿 + 录屏 | R-01→R-02→R-03 串行 | 高 |
| **P1 可信消费** | 从"能画"到"敢用" | 8 图看板、全局联动、语义层、缓存 | R-04, R-05, R-06, R-09, (R-08) | 8 图拼装刷新持久化；选"华东"全卡同步；改 GMV 口径一次生效；热缓存命中 ≥70%、首屏热 P95 ≤2s（压测报告） | R-01→R-04→R-05 | 高 |
| **P2 探索分发** | 触达与交付 | SQL Lab、订阅告警、定时刷新 | R-10, R-12, R-07, R-15, R-16 | 自由 SQL 出结果 + 历史；cron 触发邮件/webhook；定时物化命中 | R-12→R-07 / R-10 | 中高 |
| **P3 嵌入与广度** | 走出围墙 | iframe 嵌入、多租户、移动端 | R-11, R-13, R-14, R-17 | Guest Token 嵌入 + RLS 兼容；tenant 隔离演示；375px 看板可用 | R-03→R-11；R-04→R-17 | 中 |
| **P4 治理与智能** | 长效与智能 | 可观测、血缘、发布评审、NL2SQL | R-18 + lineage + revision + NL2SQL | 血缘图 + 审计查询；草稿/发布态；NL → QueryAST 受 sanitizer 约束 | P1/P2 完成后 | 低 |

### 9.2 工作量与节奏（全部为估算）

**分阶段人日**：P0 **43–57**｜P1 **81–111**｜P2 **51–73**｜P3 **35–55**｜P4 **44–72** → **总计约 254–368 人日**。最大单体是 R-04（30–40）与 R-06（18–25）。

**关键路径（串行、不可压缩）**
- 硬地基：**R-01 → R-02 → R-03**（身份/鉴权/RLS 必须逐层建，约 35–45 人日串行）
- 消费链：**R-01 → R-04 → R-05**（看板容器必须先于联动）
- 基建链：**R-09 → R-12 →（R-07 / R-10）**

**可并行机会**：R-04 仅依赖 R-01，**可在 R-02/R-03 施工时同步开工**；R-06（语义层）只依赖 R-01，可与 R-04 并行；R-08、R-14、R-16 依赖少，可作"填隙任务"任意插入。

| 人力情形 | P0 | P1 | P2 | P3 | P4 | 合计 |
|---|---|---|---|---|---|---|
| 1 人全职 | 9–11 周 | 16–22 周 | 10–15 周 | 7–11 周 | 9–14 周 | **≈51–73 周** |
| 2 人并行 | 5–7 周 | 9–13 周 | 6–9 周 | 4–7 周 | 6–9 周 | **≈30–45 周** |
| 3 人并行 | 4–5 周 | 6–9 周 | 4–6 周 | 3–5 周 | 4–6 周 | **≈21–31 周** |

> **1 人全职不建议做全量 v1，只做 P0 + P1 窄切片。**

**最短见价值路径**：**R-01（认证）+ R-04（看板容器，复用现有 chart）** ≈ **50–65 人日**，2 人并行约 **5–7 周**即可对外演示"登录后访问 8 图看板"。这是第一个能让业务人员"第 1 步不流失"的里程碑。

### 9.3 阶段详解

- **P0 身份地基**｜目标：合规红线达标。交付：`service/auth`（JWT + OIDC 预留）、`middleware.Auth()` 全端点接线、Casbin 鉴权装饰器（白名单仅 `/health` + `/share/{token}`）、`QueryAST.rls_predicates` + Planner 注入 + sanitizer 二次校验、`raw.go` 同等拦截。Gate：401/403 语义正确、A/B 数据隔离录屏、契约测试全绿。可延后：OIDC/SSO 实际对接。退路：31 端点回归 → parity 测试兜底；存在旁路 → 强制走 `query` 包。
- **P1 可信消费**｜目标：看板级消费 + 口径可信 + 性能达标。交付：`service/dashboard`（layout_json / items）、网格画布、看板级 `filters_json` + `{{filter.x}}` 注入、`service/metric`（编译到 QueryAST）、Redis 缓存（短路 executor）。Gate：8 图首屏热 P95 ≤2s（压测报告）、改口径全局生效录屏、命中率 ≥70%。可延后：导出增强。退路：RLS 导致命中率低 → 分层缓存键（无 RLS 共享 / 有 RLS 加 user scope）。
- **P2 探索分发**｜目标：触达。交付：`cmd/worker`（asynq + Redis，同镜像不同入口）+ `schedules` 物化、订阅告警（邮件/webhook）、SQL Lab（强制 LIMIT + 超时 + 白名单）、目录收藏搜索、导出。Gate：cron 触发告警、SQL Lab 出结果 + 历史、定时刷新命中。退路：部署变重 → 保留单机 all-in-one compose。
- **P3 嵌入与广度**｜目标：走出围墙。交付：Guest Token + `/embed/{token}`、多租户 `tenant_id` 全表 + `workspaces`、新驱动（Oracle/SQLServer/Doris）、375px 响应式看板。Gate：第三方 iframe 嵌入 + RLS 兼容、tenant 隔离演示。退路：迁移锁表 → goose"加可空列 → 回填 → 加约束"。
- **P4 治理与智能**｜交付：可观测、`lineage_edges` 血缘、`revisions` 发布评审、NL2SQL（受 sanitizer 约束）。Gate：血缘图 + 审计查询、NL → QueryAST 安全下推。

### 9.4 风险登记表

| 风险 | 类型 | 概率 | 影响 | 缓解措施 | 触发信号 |
|---|---|---|---|---|---|
| RLS 经 `raw.go` 旁路逃逸 | 技术 | 高 | 高 | sanitizer 二次校验 + 旁路拦截 + 安全测试 | 出现绕过 predicate 的查询路径 |
| 31 端点鉴权接线回归 | 技术 | 中 | 高 | 扩充 parity 契约测试 + 最小白名单 | 契约测试变红 |
| 缓存命中 <70%（RLS key 放大） | 技术 | 中 | 中 | 分层缓存键 + 命中率监控 | 命中率低于 70% |
| 范围蔓延（追 40–50 图表 / 80 源） | 范围 | 高 | 中 | Non-goals 锁定 + RICE 收敛 | 需求池新增未评分项 |
| 引入 Redis/worker 破坏轻部署 | 技术/组织 | 中 | 高 | all-in-one compose 保留、worker 可选 | 部署文档显著变复杂 / 被迫上 K8s |
| 存量数据 tenant/owner 归属未决 | 数据 | 中 | 高 | 可空列 → 回填策略 → 约束 + feature flag 灰度 | 迁移违约束 / 空值 |
| 少人团队 → 关键路径串行、价值晚暴露 | 组织 | 高 | 中 | 优先最短见价值路径，先发 M0/M1 窄切片 | 超过预期周期无里程碑 |
| 大表冷缓存 P95 ≤20s 达不到 | 技术 | 中 | 中 | 物化表 + R-12 预热 + TPC-DS 门禁 | 压测超标 |
| 指标口径与 dataset 表达式双轨不一致 | 技术 | 中 | 中 | metric 单一编译出口，禁止内联旁路 | 同名指标两处取值不同 |

---

## 十、里程碑与验收锚点

| 里程碑 | 阶段末可演示什么 | 对应数析评分卡达标项 |
|---|---|---|
| **M0** | 两账号登录隔离 + RLS（A 华东 / B 全量）+ 未登录 401 | 权限治理 P0 域 **≥70%** |
| **M1** | 8 图看板 + 全局筛选联动 + 改口径全局生效 + 首屏热 P95 ≤2s | 看板 / 建模语义 / 性能缓存域达标，**加权总分率 ≥75%**（双门槛达标） |
| **M2** | SQL Lab + 订阅邮件触发 + 定时刷新 | 协作分发 P1 域 **≥50%** |
| **M3** | 第三方 iframe 嵌入 + 多租户 + 375px 移动端 | 扩展与嵌入域 **≥50%** |
| **M4** | 血缘 + 审计 + NL2SQL | 运维可观测 P2 域 |

> 每项证据 = 截图/录屏 + 可复现步骤 + 压测报告 + 测试全绿。**无证据按 0 分。**

---

## 十一、"比肩"的可量化验收标准（来自数析）

### 11.1 验收原则（怎么定义"比肩"才不自欺）
1. **口径 = 可完成任务，而非功能数量相等**——对标"Superset/Metabase 核心能力集"（覆盖约 80% 日常场景的条目集），长尾图表/长尾数据源只做**非计分参考项**单独披露。
2. **证据制**——每条能力必须附"可现场演示证据"（截图/录屏 + 可复现步骤 + 数据），无证据按 0 分。
3. **可复算**——权重、条目分、达标线、数据来源全部写死，第三方可独立复算。
4. **双门槛**——总分达标 **且** P0 能力域逐个达标，任一不达标即整体不达标。

### 11.2 功能覆盖度评分卡（0 = 无 / 0.5 = 部分 / 1 = 达到对标可用水平）
**达标线：P0 域 ≥70%，P1 域 ≥50%，加权总分率 ≥75%。**

| 能力域 | 权重 | 优先级 | 现状自评 | 达标线 | 现场证据要求 |
|---|---|---|---|---|---|
| 数据接入 | 12% | P0 | 0.5（4 驱动） | ≥70% | 新增 1 个真实数据源并成功建数据集（录屏） |
| 建模 / 语义层 | 13% | P0 | 0（缺失） | ≥70% | 定义一个指标（如 GMV）+ 一处改口径全局生效 |
| 查询与探索 | 13% | P0 | 0.2（无 SQL Lab） | ≥70% | SQL Lab 执行 + 参数化 + 行数上限保护演示 |
| 可视化 | 12% | P0 | 0.7（7 图型） | ≥70% | 建 1 张图并导出 PNG/CSV |
| 看板 | 12% | P0 | 0（缺失） | ≥70% | 8 图看板 + 全局筛选联动 |
| 权限治理 | 12% | P0 | 0（无登录） | ≥70% | 两角色登录，A 看不到 B 的数据集 |
| 协作分发 | 10% | P1 | 0.1（仅分享） | ≥50% | 订阅/告警/分享各自触发一次 |
| 性能与缓存 | 8% | P1 | 0.2（无缓存） | ≥50% | 同查询二次命中缓存，QPS 日志对比 |
| 扩展与嵌入 | 5% | P1 | 0 | ≥50% | 第三方页面嵌入 1 张图 |
| 运维可观测 | 3% | P2 | 0.5（Sentry + 非功能） | ≥50% | 后台可见查询成功率 / 慢查询 Top10 |
| **加权总分** | 100% | — | **≈0.29** | **≥75%** | — |

**判定规则**：达标 = 加权总分率 ≥75% **且** 全部 P0 域 ≥70%；总分达标但任一 P0 域未达标 → **不达标**。

### 11.3 性能 SLI / SLO
分档：小表 <10 万行 / 中表 ≈100 万行 / 大表 >1 亿行。测量：脚本化压测（k6 / locust）在**冷缓存**与**热缓存**分别跑，P50/P95 取实测分位。

| SLI | 热缓存目标 | 冷缓存目标 | 依据 |
|---|---|---|---|
| 单图查询 P50 小表 | ≤300ms | ≤500ms | Metabase 公开经验 10 万行 200–500ms（二手） |
| 单图查询 P95 小表 | ≤800ms | ≤1.5s | 团队建议值，待实测校准 |
| 单图查询 P50 中表 | ≤1s | ≤3s | Metabase 公开经验 100 万行 2–5s（二手） |
| 单图查询 P95 中表 | ≤2s | ≤6s | 团队建议值 |
| 单图查询 P50 大表 | ≤1s | ≤8s | Superset + Trino 实测：500M 行缓存后 P95 1.2–3s（二手） |
| 大表 P95 | ≤3s | ≤20s（兜底超时） | 团队建议值 |
| 8 图看板首屏 P95 | ≤2s | ≤8s | Metabase 5–8 卡 2–4s；Superset 缓存后 0.8–1.5s（二手） |
| 并发承载 | 20 并发查询 P95 不劣化 >30% | — | Metabase 20 并发稳定、200+ 降级（二手） |
| 缓存命中率 | ≥70% | — | Superset 生产目标 >70%，实测 78–85%（二手） |
| 查询超时率 | ≤1% | — | 团队建议值 |
| 查询失败率 | ≤0.5% | — | 团队建议值 |
| 连接池饱和度 | 峰值 ≤80% | — | Superset 建议单应用不超过 DB max_connections 的 60–70%（二手） |

### 11.4 可靠性指标
| 指标 | 定义 | 目标 | 数据来源 |
|---|---|---|---|
| 查询成功率 | 成功查询 ÷ 总查询 | ≥99.5% | 后端埋点 |
| 查询超时率 | 超时查询 ÷ 总查询 | ≤1% | 后端埋点 |
| 错误可归因率 | Sentry 中带 stacktrace + request_id 的错误 ÷ 总错误 | ≥95% | Sentry |
| API P95 延迟 | 全 API P95（非查询类） | ≤500ms | 网关 / Access Log |
| 发布回滚时间 MTTR | 发现 → 回滚完成 | ≤15 min | 发布记录 |

### 11.5 采用度指标（含埋点口径）
| 指标 | 分子 | 分母 | 周期 |
|---|---|---|---|
| 周活用户 WAU | 周期内 ≥1 次关键行为的去重 uid | — | 自然周 |
| 看板周访问 | 看板浏览事件数 | 看板数 | 自然周 |
| 人均周查询 | 查询事件数 | 活跃 uid | 自然周 |
| 分享链接打开率 | 被打开的分享链接数 | 创建的分享链接数 | 自然周 |
| 指标复用率 | 被 ≥2 图表引用的语义指标数 | 语义指标总数 | 自然周 |
| 7 日留存 | 第 W 周新增活跃且第 W+1 周仍活跃的 uid | 第 W 周新增活跃 uid | 周 |
| 首次上手时长 TTFV | 首张可用图落地时间 − 首次登录时间 | 新用户 | 单次，取中位数 |

> 关键行为需产品/工程冻结为埋点事件名（如 `query.executed`、`dashboard.viewed`、`chart.saved`）。

**建议**：用 TPC-DS（1GB / 10GB / 100GB）三档数据集建立可复现基准，作为性能 SLI 的唯一权威测量入口，逐条回填"待校准"值。

---

## 十二、差异化机会与"自研 vs 开源"取舍（来自竞析）

### 12.1 差异化机会（DataRay 可以打的方向）
1. **语义层收口**——Superset/Metabase 的 metrics 定义都偏弱、Redash 干脆没有；DataRay 已有 `QuerySpec` / `QueryAST` / `QueryPlanner`，可顺势做成**强类型、可版本化、可复用的指标/维度语义层**（对标 Lightdash 的"单一事实源"，但不必绑定 dbt）。这是它们共同的软肋。
2. **Go 侧性能与轻部署**——单二进制 + 无 Python/Celery 重栈，主打"比 Superset 轻、比 Metabase 可控"，直击 Superset 运维重这一最大痛点。
3. **AST 驱动的安全下推**——已有 `QueryAST` → 可在 AST 层注入 RLS/行级过滤，天然支持多数据源且不依赖各驱动的 SQL 方言，比 Superset 的 SQL predicate 拼接更稳。
4. **国产数据源 + 中国式报表**——StarRocks / ClickHouse 已覆盖，可再加国产库与类 Excel 复杂报表，切国内中大型企业。

### 12.2 自研 vs 依赖（结论）
- **自研成本极高（建议依赖成熟组件 / OSS 能力）**：① SQL Lab（编辑器 + 异步 + 历史 + 方言）；② 调度 / 订阅 / 告警（Celery/Redis 一整套 worker 基建）；③ 权限治理体系（RBAC + RLS + SSO + 审计）；④ 缓存 / 加速层。**这四块自研投入不亚于重做一个 Superset 后端，且是纯工程苦力、无差异化。**
- **DataRay 已在正确方向（应继续加注）**：查询语义层 / `QueryAST` / `QueryPlanner`——这恰是 Superset 相对薄弱之处，是"比肩乃至超越"的支点。
- **总体结论**：与 Superset/Metabase **正面自研全栈不划算**；合理路径是把差异化压在**语义层 + AST + 轻部署**，把门槛能力中的重活（调度、权限、SQL 编辑器、缓存）用成熟组件补齐。

---

## 十三、利益相关者沟通要点（来自路径）

**高管版**
1. 三个阶段让 DataRay 从"能画图"变成"组织能用"：身份合规 → 看板可信消费 → 分发嵌入。
2. 最短见价值约 **5–7 周**（2 人估算）即可对外演示"登录 + 8 图看板 + 口径可信 + 首屏 ≤2s"。
3. 差异化押注**轻部署**（Go 单二进制、无 Python/Celery 重栈），打 Superset 运维痛点，**不拼图表数量**。
4. 7 个待拍板决策中"**调度选型 + 部署形态**"直接决定后三阶段节奏。
5. 最大杀手是范围蔓延，已锁 Non-goals。

**工程版**
1. 依赖不可压缩：`R-01 → R-02 → R-03` 是硬地基；`R-04` 仅依赖 `R-01`，**可与 R-02/R-03 并行开工**。
2. RLS 必须在 `QueryPlanner.build` 注入 AST（不是拼 SQL 字符串），`raw.go` 同等拦截。
3. 缓存键 = `dataset + canonical_spec + tenant +（RLS 时 user）+ data_version`，注意 key 空间放大。
4. 调度用 `cmd/worker`（asynq + Redis）同镜像不同入口，**保留单机 compose**。
5. 31 端点接线靠 parity 契约测试兜底；迁移走 goose"加列 → 回填 → 约束"，**禁 big-bang**。

**设计版**
1. 看板画布先定 **12 列栅格 + 卡片最小尺寸**，再谈 @dnd-kit 拖拽体验，`layout_json` 持久化。
2. 全局筛选需前置定义**变量命名 / 作用域 / 默认值规范**（`filters_json` → 各卡 QuerySpec 的 `{{filter.x}}`）。
3. 375px 响应式虽在 P3，但**栅格断点应在 P1 看板设计时预留**以免返工。
4. 登录页 / 空态 / 加载态 / **无权限态**须在 P0–P1 统一设计——业务人员第 1 步流失常因找不到入口。

---

## ✅ 行动清单

| # | 行动 | 负责方 | 时间窗 |
|---|------|--------|--------|
| 1 | 拍板 7 项决策（认证方式 / 多租户模型 / 调度选型 / 指标层落点 / 缓存后端 / 部署形态 / 插件图表） | 项目负责人 | 立即（P0 开工前） |
| 2 | 开 P0 身份地基：R-01 认证 → R-02 RBAC → R-03 RLS，同步 T-01/T-02/T-03 | 后端 | P0 首段 |
| 3 | 并行开工 R-04 看板容器（仅依赖 R-01，复用现有 chart 渲染） | 前端 + 后端 | 与 R-02/R-03 并行 |
| 4 | 建立 TPC-DS（1GB/10GB/100GB）可复现性能基线与压测脚本 | 工程 | P0 期间 |
| 5 | 用现有 parity 契约测试扩出鉴权回归面（31 端点 × 401/403） | 后端 | P0 期间 |
| 6 | 交付 M0（两账号隔离 + RLS 录屏证据）并通过数析权限域 ≥70% 验收 | 工程 + 产品 | P0 末 |

---

## ⚠️ 待确认 / 假设 / Non-goals

### 待确认问题（需项目负责人拍板）
| # | 决策点 | 选项 | 团队推荐 |
|---|---|---|---|
| 1 | 认证方式 | 内置账号（JWT）优先 vs OIDC/SSO 优先 | **先内置 JWT，预留 OIDC 适配层** |
| 2 | 多租户模型 | 共享库 + `tenant_id` vs schema 隔离 vs 库隔离 | **共享库 + tenant_id**（成本低），大客户再 schema 隔离 |
| 3 | 调度选型 | Go 原生 worker（asynq）+ Redis vs 引入 Python/Celery | **Go 原生**，契合"轻部署"差异化 |
| 4 | 指标层落点 | 独立 metric 实体 vs dataset 内表达式 | **独立实体 + 编译到 QueryAST** |
| 5 | 缓存后端 | Redis vs 进程内 vs PG 物化表 | **Redis（命中）+ 物化表（重查询）** |
| 6 | 目标部署形态 | 单机 Docker 优先 vs K8s | 影响调度/缓存选型，**建议先单机 all-in-one** |
| 7 | 是否开放插件式自定义图表 | 是 / 否 | **短期不推荐**（投入大） |

### Non-goals（明确不做什么）
- 不正面复刻 Superset 40–50 图表长尾（核心集 + 插件扩展点即可）。
- 不自研调度引擎（依赖成熟队列/worker）。
- 不自研 OLAP / 查询加速引擎（复用 StarRocks / ClickHouse 或现成缓存）。
- 不做 80+ 数据源军备竞赛（聚焦国产 + 主流）。
- 不取代 Excel（受控导出，让用户在受控环境里用）。
- 不做移动端原生 App（响应式 Web）。
- v1 不做实时流式告警（批式）。
- 不做完整数据目录 / 治理平台（血缘轻量版）。

### 假设
- 上述人日均为**估算**，未含需求澄清、联调等待与返工；基于析客需求池区间汇总。
- 性能 SLO 中标注"团队建议值"的阈值需以基线压测后回填。
- 用户角色画像与场景断点基于公开调研 + 常识推断，**尚未做真实用户访谈**；建议补做 5–8 位目标用户访谈或可用性测试。
- 竞品能力基于官方文档与发布说明（2026-09 核实）；Superset 原生 NL2SQL 能力标注为"未核实"。

---

# 第二轮竞品调研（2026-09-19）——扩展至三阵营 25 家

> **本节为第二轮调研成果，并入本文件（未另建文档，遵守文档集「禁止再建增量修订版本」纪律）。**
> **范围说明**：第一轮只对标 Superset / Metabase 两家 + 5 家参照。第二轮扩至**三阵营共 25 家**，并新增两个对 DataRay 最贴身/最标杆的对手：**DataEase**（同技术栈开源竞品）与 **Metabase**（交互体验标杆）。
> **方法**：优先一手来源（官网 / 官方文档 / release notes / GitHub / 定价页）；**所有事实标注时间与来源域名**；二手转述显式标注「二手」并要求多源交叉；未验证项显式标注，**不作为结论依据**。

---

## 十四、三阵营全景（25 家）

### 14.1 开源 / 可自托管阵营（9 家）

| 竞品 | 定位 | 开源 / 商业边界 | 分享机制 | AI 能力 | 定价 |
|------|------|----------------|----------|---------|------|
| **Apache Superset** | Apache 基金会开源探索+可视化平台；**有数据工程师在编**的技术团队 | Apache 2.0，**无商业版、无付费 tier、无用量限制** | 默认需登录 permalink；**匿名直链需同时命中 4 项配置**（`AUTH_ROLE_PUBLIC` + `PUBLIC_ROLE_LIKE` + `DASHBOARD_RBAC` + 给 Public role 授 6 类权限）+ 跑 `superset init`；正式嵌入走 **guest token(JWT)** + RLS 注入。**全流程无「密码保护分享」概念** | 6.0（2025-12）**MCP 集成**，agent 可查数据集/取图表数据；**同月修复 MCP `execute_sql` 未应用 RLS 的安全漏洞** | 自托管免费；Preset 托管付费（档位未验证） |
| **Metabase** | 非技术用户自助 BI 标杆；「zero to dashboard in 5 minutes」 | OSS **AGPLv3**，免费**不限用户**；Pro/Ent 与 OSS **同一份软件**（官方 FAQ "Feature-wise, zero"）；Pro 锁行列级权限/SSO/缓存/审计/多租户嵌入/白标；Ent 独有 air-gap + 1 天 SLA | 公共链接 `/public/dashboard/<uuid>` **无密码**；**撤销后重新生成会得到不同链接、旧链接永久失效**；官方警告「公共链接访客可改 filter，不能靠 filter 藏数据」；嵌入走 JWT + React SDK | **v60（2026-05）"AI just went open source"**：AI 下放**所有套餐**——Metabot 问数、SQL/transform 生成与调试、**官方 MCP server**、Slack 内 Metabot、图表一键摘要、Agent API；**支持 BYO Anthropic key** | OSS $0；Starter **$100/mo**(含 5 席)+$6/席；Pro **$575/mo**(含 10 席)+$12/席；Ent 起 **$20,000/yr**；AI token $3.75/1M |
| **Redash** | SQL-first 轻量查询+看板+调度；工程师/分析师 | **BSD-2-Clause**，自托管免费；**云版 2021 已停售** | `public_url_token` 开启后**任何人凭 URL 可看、无密码、默认无过期**；**有跨组织数据泄露的生产复盘实证**（修复建议：审计并置 NULL / 按组织禁用 / 加 CIDR 白名单） | **无**（三源一致） | 自托管 $0；AWS 基础设施实测 $25–40/月；无官方付费档 |
| **Grafana** | 时序/可观测看板事实标准；SRE/DevOps | **AGPLv3**（2021 由 Apache 2.0 重许可）；OSS 免费，Enterprise 需 license | 三路径差异大：direct link **需登录**；**Snapshot 免认证可看 + 可设过期 + 可在列表页删除**；**Public dashboards（10.2 GA，2023-10）免登录，全版本可用且不加 license**。**无密码分享** | Assistant **2025-08 公测 → 2025-10 GA**；Investigations 同场推出（token 计量）；12.2 **AI SQL expressions（NL→SQL + 查询解释）Public Preview** | Cloud Free；Pro $19/mo 平台费 + 按量；Ent 最低承诺 **$25,000/yr** |
| **Lightdash** | dbt-native、metrics-as-code；analytics engineer | **MIT core + `ee/` 独立许可**（「开源」需此 caveat）；自托管实测缺口硬：**scheduling / smart caching / AI / CSV export 全在 Cloud Pro**；SSO 自托管仅 Google OAuth | tokenized share link；React SDK 嵌入（JWT 带 `projectUuid`）；**无密码分享** | AI query agents + MCP，**仅付费 cloud tier**——自托管**拿不到 AI** | 自托管 $0；**Cloud Pro $3,000/月 flat、不限用户** |
| **Evidence.dev** | BI-as-code：SQL + Markdown → 静态站点；会 SQL 且用 Git 的开发者 | **MIT**；OSS 自托管免费，Studio Cloud 商业 | 产物是静态站点，可托管任意位置。**无密码分享** | **Studio（2025-06）**内置 schema-aware AI dev agent + 自服务 viewer；**OSS 自托管无 AI analyst** | 自托管 $0；Cloud 按用量（档位未验证） |
| **Rill Data** | DuckDB/ClickHouse 上的秒级 operational OLAP 探索 | **Apache 2.0**；Rill Developer 开源，Rill Cloud 商业 | Rill Cloud + embed iframe API（状态追踪/预设筛选/动态高度） | **2025 AI Chat**（对话式查**语义层**，非自由格式）；**2026 初 MCP Server** | OSS $0；Cloud 免费/试用 + 付费档（价格未验证） |
| **Cube** | **headless 语义层** / 通用指标平台；做嵌入式分析与 AI 体验。**不是 dashboard 工具** | **Cube Core Apache 2.0 完全可自托管**；Ent 含 BYOC/BYOLLM/SAML 2.0/99.990% SLA | **无内置看板分享**，靠 REST/GraphQL/Semantic SQL API 自建 | **D3（2025-06-02）**：官方称首个建立在通用语义层上的 agentic analytics 平台；`Semantic SQL` 的 `MEASURE()` 定位为 **AI agent 查询的治理机制**；原生 MCP server | Core $0；Cloud Starter **$40/dev/mo**、Premium $80；CCU $0.10–0.40/unit；企业年费中位数 **$37,200/yr**（2026-06 口径） |
| **DataEase**（本轮新增·**最贴身**） | 国产「人人可用」拖拽 BI + 数据大屏；中小企业/政企。定位 = 帆软开源平替 | **Apache-2.0** 社区版 + 企业版；社区版边界在**权限管理/开放集成/部署方式/技术支持**四项（2025-09-01）。GitHub ~23.9k star | **本阵营最强，且是 DataRay 现有能力的超集**：公共链接 = **有效期 + 密码 + 自定义后缀（8–16 位）+ Ticket 机制**（单资源多 token、参数绑定如 `{"国家":"Lebanon"}`、每 ticket 独立有效期、可设「必选」）；另有 iframe 嵌入、二维码、PDF/图片导出 | v2.10.13 LTS（**2025-09-16**）引入 **SQLBot 开源智能问数** | 社区版免费；企业版付费（价格未公开验证） |

### 14.2 国内商业 BI 阵营（11 家）

| 竞品 | 定位/客户 | 差异化 | AI 能力 | 分享机制 | 定价口径 |
|------|-----------|--------|---------|----------|----------|
| **火山引擎 DataWind**（体验标杆） | VeDI 旗下增强型 ABI；中大型企业 | 万亿级明细亚秒查询；可视化建模 40+ 算子（2025-11-18） | 分析助手+知识库：NL 取数、生成图表/仪表盘框架、归因指标树；支持豆包/DeepSeek R1、V3；**增值模块**。**AI 直连数据集仅支持 CK/ByteHouse/Doris——PG/MySQL 不在列** | 快速嵌出 / Iframe / SDK；external 链接免登但权限仍由 DataWind 侧控制；**未见匿名公开链接+密码/有效期** | 标准版 5 万/年、专业版 10 万/年、云托管 4.5/9 万/年、个人版 688 元/年（2025-08-28 生效） |
| **帆软 FineBI / FineReport** | 市占第一的自助 BI + 中国式报表 | 7.0（**2025-09**）以「指标中心」重建基座：指标拆解树、语义模型、全链路血缘 | FineChatBI / Dora：NL2DSL 混合多模型 + 自研 FineLLM 小模型 | **公共链接免登录、无需数据权限即可看**；可设密码 + 有效期；**默认永久有效**；**需管理员先开分享权限** | V6.1 标准版(5 设计/10 查看) 30 万/年 + 设计/查看各 1 万/年/个（2025-02-20） |
| **观远数据** | 现代化 BI；零售/快消/500 强 | 行业场景化模板（最硬护城河） | ChatBI「问数-问知-问策」；案例：新东方、自然堂（取数准确率 60%→92%） | **6.6 版起支持公开链接匿名访问、无需登录**（2024-09-06）；免密 SSO + iframe | 华为云独立部署：基础版 16.5 万/年(25 用户)、企业版 51 万/年(20 用户) |
| **阿里云 Quick BI** | 云生态 BI；中小企业 | 阿里云数据源绑定 + 组织级协同授权 | 智能小Q：问数/报告/洞察 Agent/解读，**按席位加购** | **公开链接所有人可访问且无需登录**，可设截止日期；私密链接需登录；嵌入用 accessTicket（有效期 ≤240 分钟） | 智能问数 200 元/个（**2025-08-07 起阶梯降至 50–200 元**）、小Q报告 600 元/个、洞察 Agent 15 万/年 |
| **腾讯云 BI / 腾讯有数** | 腾讯云 BI=中小企云原生；**有数=微信生态经营分析平台，非自助拖拽 BI** | 企微/微信生态打通，多端自适应 | ChatBI（混元 / DeepSeek） | 嵌入第三方系统 + 企微/钉钉推送；有数走 token 集成页面免登 | 个人版 9.9 元/年、基础版 4675 元/年(10 用户)、专业版 10.8 万/年(50 用户)（二手） |
| **网易有数** | 集团型/敏捷 BI；多租户分域 | 高性能 MPP + 跨引擎物化视图 | 有数 ChatBI / 网易知数（2025 品牌焕新） | **发布-分享生成的链接无需登录可直接访问**，可设密码/失效时间；**可在项目中心统一管理分享链接有效性** | 未见公开定价 |
| **Smartbi** | 金融/央国企合规场景 | 全栈信创 + Agent BI | AIChat 白泽 V4（2025）：三类智能体 + MCP/A2A | 公共链接（不加密）/ 访问限制（**自动生成 4 位密码** + 有效期）/ 二维码 | 华为云：白泽 106 万/年、Insight 6.8 万/年（2025-09） |
| **永洪 BI** | 高性能大数据分析；金融/制造/政务 | VooltDB 计算引擎，10 亿级秒查 | Megrez 智能问数（天权）、Dubhe（天枢） | 公开分享；**加密分享密码仅 1–8 位**、有效期 1–365 天（默认 7 天）；**早期嵌入靠 URL 明文传账号密码，8.7 版起才默认禁止**（官方自承不安全） | 社区版免费，企业版定制报价 |
| **衡石科技** | **嵌入式 BI PaaS，卖给 ISV/SaaS，不直接卖终端业务用户** | 分析即服务；零代码嵌入 + 多租户 SaaS | SENSE 6.0 Agentic BI；**HENGSHI CLI 让 Claude Code / Codex 等编码 Agent 直接执行 BI 工程** | 以 SDK/嵌入与 API 为主，非公开链接 | 定制报价（二手口径 50 万起）；无免费个人版 |
| **DataFocus** | 搜索式分析；中小企业 | 搜索替代拖拽（「让图形适配数据而非相反」） | FocusGPT 数据分析智能体（多轮对话 + 自动出图 + 分析总结） | 未见公开信息 | 云标准版 5000 元/年(5 用户/5GB)；宣传「468 元/年起」 |
| **百度 Sugar BI**（补充·内网定价锚） | 零代码报表+大屏；私有化便宜 | 150+ 图表、ECharts 血统 | 文心驱动的对话式查询/归因/洞察/预测/总结 | 嵌入式集成 + 细粒度访问控制 | 私有买断 2 账号 15 万 / 30 账号 45 万(+1.5 万/账号) / 不限 150 万；按年 3 / 10 / 20 万 |

### 14.3 国际巨头 + AI 原生阵营（8 家）

| 产品 | 定位/客户 | 核心差异化 | AI（2025–26） | 定价口径 |
|------|-----------|------------|----------------|----------|
| **Tableau**（Salesforce） | 可视化标杆；数据成熟的大企业 | 可视化深度；社区模板生态；VizQL 拖拽 | **Tableau Agent**（原 Einstein Copilot）；Agent in Pulse 已换 **GPT-5.2**（400k 上下文）；Dashboard 内对话分析 GA；**Tableau MCP** 接入 Claude/ChatGPT/Codex 与 Slackbot（2026-07） | Cloud Standard **$15** / Enterprise **$35** / Cloud+ 面议；Tableau Next **$40**；均年付（2026-09 核） |
| **Power BI / Fabric**（Microsoft） | 微软生态默认标准 | 与 M365/Teams/Excel 深度耦合；OneLake 单一事实存储 | Fabric IQ / Copilot：NL 生成 DAX、语义模型、整页报告；**Copilot 从 F64+ 下放至全部付费 F 容量（含 F2，~$263/mo）** | Pro **$14**/人/月（2025-04-01 由 $10 上调）、PPU **$24** |
| **Looker**（Google） | GCP/BigQuery 大客户 | **LookML 语义层**；API-first | **Visualization Assistant（GA，2026-04）**：说「改成堆叠条形、按区域上色」就改图；Expression Assistant 说人话写 LEXP；Conversational Analytics 嵌入式 **GA**；Next'26 发 Dashboard Agents / Agentic Workflows（均 **Preview**） | 三档全「Call sales」；**AI 按 Gemini Data Tokens 计量**，**2026-10-01** 起超量 $3/M 输入、$20/M 输出 |
| **Qlik** | 制造/金融等有 QlikView 存量的大企业 | 联想引擎（无需预定义路径）；Talend 数据集成 | **Qlik Answers**（RAG 多智能体，带引用与推理页）；Discovery Agent；MCP Server 各档均含 | 容量制：Starter **$300/mo**(10 人)、Standard **$825/mo**(不限人)、Premium **$2,750/mo**；Answers 配额 25/200/1,000 问 |
| **Sigma Computing** | 仓库原生、表格习惯的业务用户 | 类 Excel UI 直查仓库（无抽取）；writeback | Sigma Assistant / AI Query（2025-12）/ **Sigma Agents**（2026-04，能写回、调 API）；双向 MCP；**AI 展示每一步决策逻辑** | 官方页无价格；第三方口径冲突（$25 Essential / $75 Business 每人月 vs $300/mo 不限人） |
| **ThoughtSpot** | 非技术用户为主的搜索式自助 | 搜索优先而非仪表板优先；SpotIQ 自动发现 | **Spotter** + **Spotter Semantics**（2026-03，加确定性推理）；新推 **AgentSpot**；Mode 已并入 Analyst Studio | Essentials **$25**/人/月、Pro **$50**（含 Spotter **25 次/人/月**）、Enterprise 面议；Pro 另有 **$0.10/query** |
| **Wren AI** | 语义层 + Text2SQL 开源 | MDL 语义层 + AI context layer；**Text2SQL 带 dry-plan 校验**；22+ 源 | 2026-05-07 Wren Engine 并入主仓，**旧 chat-first BI 产品转为 `legacy/v1`** → 明确把「生成 SQL→部署 dashboard→分享」当作新范式 | **Apache 2.0**（~17.4k star） |
| **Vanna 2.0** | SQL 生成 | RAG（DDL/文档/例句向量化）+ Tool Memory 自学习；LLM 可换 Ollama | v2.0.2（2026-02-02）；只产出 SQL/DataFrame/图，**无画布，前端被公认不够精** | **MIT**（23k+ star）；商业版 Explorer 20 问/天起 |
| **Julius AI** | 分析+出图+出稿，绕开数据仓库 | **不走语义层**：文件上传 + Python 沙箱（32GB）→ PPT/网站/视频 | 产物**不可编辑成 BI 资产** | Free / Plus $20 / Pro $45 / Max $200 / Business $450（二手） |
| **Hex** | 探索分析 + 交付 | 笔记本（SQL/Python）+ Notebook Agent；语义层**复用** dbt MetricFlow / Cube | AI 生成代码后**人工接管修改** | 闭源、**无自托管**；Community 免费 / Professional $36 / Team $75 每 editor/月 |
| **Omni** | 建模 + AI 底座 | **语义层即产品**（governed context graph，Git 版本控制），本地与外部 agent 共用定义 | **2026-04-23 Series C $120M @ $1.5B 估值**（ICONIQ 领投，营收同比 4x）；MCP 让 Claude/ChatGPT/Cursor/VS Code 查同一定义 | 闭源，需自带仓库算力 |
| **Mode** | **已消失** | — | 2023-06 被 ThoughtSpot **$200M** 收购，现为 Analyst Studio | — |

---

## 十五、特别点评：「拖拽 → 即时出图」摩擦度排名

| 排名 | 产品 | 凭什么 |
|------|------|--------|
| **1** | **Metabase** | ① **不要求你先「拖」**——渐进披露式点选（选表 → Summarize → 选分组 → 选图型），每步只有少量选项；② **每改一处图表立刻重绘，中间没有「Run query」这道割裂**。这是相对 Superset 的**结构性优势**（用户永远在连续反馈回路里调参） |
| **2** | **DataEase** | 纯拖拽 + 200+ 大屏模板，「五分钟数据变大屏」是真的快。但优化终点是「大屏」而非「回答一个问题」，且**与 DataRay 同栈（ECharts/AntV）**→ 必须靠「问题导向」错位 |
| **3** | **Superset** | 有字段拖拽，但**图表类型是前置选择**、配置项极多，很多视图需 Run 才见结果——用灵活性换了摩擦 |
| — | Grafana | 拖的是「面板布局」不是「字段到图」，且核心面向时序 |
| — | Redash | **必须先写 SQL** 才有图，拖拽只用于 dashboard 布局 |
| — | Rill / Evidence / Lightdash | 摩擦点在「先写 YAML / SQL」，本质不是拖拽工具 |
| — | Cube | 无可视化建模界面，业务用户不友好 |

> **结论**：DataRay 应学 **Metabase 的「任一改动即时出图」**，不是学 Superset 的「先选图型再配置」。**DataRay 已有自动查询 effect** —— 这是既有优势，Phase A 必须守住（不得为了性能把它改成显式 Run）。

---

## 十六、趋势判断与 table-stakes 分级

| # | 趋势 | 证据（时间 / 来源） |
|---|------|---------------------|
| 1 | **语义层复兴，身份从「BI 建模」变成「AI 基础设施」** | Gartner MQ 2026 把 **Semantic Modeling 列为平台必备能力**（报告 2026-06-29）；Snowflake Semantic Views（2026-03 GA）、Databricks Metric Views（2026-04 GA）；Omni 以 **$1.5B 估值**为此定价（2026-04-23） |
| 2 | **Agentic analytics / 「数据找人」** | 同份 Gartner 把 **Agentic Insights、Conversational Analytics 列为入场券**；Snowflake 与 Databricks 2026-06 双峰会均以 agent 为主角 |
| 3 | **Headless / embedded BI 与「去界面化」** | Looker 把 Conversational Analytics 开放嵌入式 **GA**（2026-04）；嵌入式分析市场 $78.53B(2025) → $89.25B(2026)，13.65% CAGR 至 2031（2026-07，二手）；G2 嵌入式 BI 平均采用率 49%→53%（2021→2026 夏） |
| 4 | **指标即代码 + AI 用量计量化** | 指标进 YAML/Git 成常态；**所有大厂都把 AI 改成按量计费**——Looker token（2026-10-01 起收费）、ThoughtSpot $0.10/query + 25 次 Spotter 上限、Qlik 按提问数配额；**G2 2026 新设 "Semantic Layer Tools" "Agentic Analytics" 两个类目**（需求侧最硬信号） |

**分级裁定**

| 分级 | 项 | 对 DataRay 的动作 |
|------|-----|-------------------|
| **12 个月内 table-stakes** | ① 自然语言问数（六强中五家已默认打包）② 受治理的指标/语义定义（Gartner 列入必备，仓库侧已 GA）③ 轻量分发与匿名访问（Qlik 把匿名公开访问打包进 Premium → 已从卖点变基线） | ③ **DataRay 已具备**（免登录分享）· ② → **Phase B 的 R-06** · ① → **Phase D 的 R-70** |
| **仍是早期噪音** | ① 完全自主的多步 agent 工作流（Looker Agentic Workflows / Dashboard Agents **均仍 Preview**）② agent 自主建仪表板（2026 才出）③ **公开 AI 准确率**（各家数字不可横向比较） | **明确不投入** |

**必须打的折扣（反证）**：BIRD 人工基线 92.96%，榜首仅 81.95%（2025-12 提交）、GPT-5.5-xhigh 72.55%（2026-04）；**BIRD-Interact 多轮对话任务最好约 16%**；同一 agent 在 Spider 1.0 拿 91.2%、**Spider 2.0 只剩 21.3%**；CIDR 2026 论文指出 **29.7% BIRD Mini-Dev 题目本身有歧义**、52.8% 标注存疑【均二手转述】。

**但同一批数据显示结论的关键转折**：接上语义层后准确率**跳变**——dbt Labs 2026 benchmark 中语义层把分数从 90.0% → **98.2%**（Claude Sonnet 4.6）、84.1% → **100%**（GPT-5.3-Codex）；裸 schema 64.5% vs 语义层 **72.7%**；BIRD 基准裸 text-to-SQL **52.4%** → 带 Ontology **84.5%**（Databricks Genie，2026-06）；Snowflake Cortex Sense 把结构化上下文任务准确率 **24% → 86%**（2026-06）。

> **结论：瓶颈不在模型，在定义层。** 6 组独立数据同向，虽为二手，**足以支撑一条排期判断——语义层（R-06）必须先于 AI 问数（R-70）**。

---

## 十七、AI 问数切入点裁定：最佳 / 最差

### ✅ 最佳：在画布内部、以自然语言改「当前这张图」——NL → **局部操作**（换维度、换图型、加筛选、调排序）

1. **上下文成本最低**：当前画布的字段、图型、筛选就是最强的 prompt，**绕开了「把 800 列 schema 塞进上下文」**这一多轮对话崩盘主因（BIRD-Interact ≈16%）。
2. **失败可回滚**：改错了一次 ⌘Z 就回去。→ 把「真实仓库 16%–21% 准确率」这个**正确性问题降解为交互问题**——这是**不做语义层大工程前唯一能交付价值的形式**。
3. **有官方同构证据**：Looker Visualization Assistant（说人话改图表，**GA 2026-04**）、Tableau Show Me 推荐图型、Sigma 向用户暴露 AI 所用公式与筛选、Hex 生成代码后人工接管。**六强里真正把 AI 做进拖拽建图流程的，都是这种「局部改写」，而不是「绕过画布给个答案」。**

### ❌ 最差：直接做「用一句话生成整套看板 / 自主多步 agent 分析」

1. **前置条件你恰好全没有**：Tableau Next 靠 Tableau Semantics + Agentforce、Looker Agentic Workflows 靠 LookML + 权限、Sigma Agents 靠仓库治理。这条路的第一性要求是**账号体系、权限、指标治理**——DataRay MVP 明确全无。
2. **准确率证据最差、失败代价最高**：BIRD-Interact ≈16%、Spider 2.0 21.3%；而输出是一份**「看着很对」的看板，没人会重跑**——这与「改错了一拖就修好」是两个量级的风险。
3. **它是对大企业 RFP 的门票，不是内网小工具的入场券**：Gartner 把 Agentic Insights 列为必备能力，针对的是**买方市场里的六强**。单团队内部工具提前做，等于抄了别人 3 年前的企业税，还抄在最难的一环上。

### ⚠️ 次差（应明确回避）：全局聊天框式 Text2SQL 问答（无画布锚点）

每一次提问都在与 schema 歧义搏斗——CIDR 2026 显示 25% 的 Spider 2.0-Snow 题目**本身就有歧义**，模型只能猜一个读法然后自信作答。

---

## 十八、三栏定位结论（第二轮修订版）

### A. 该抄的（table-stakes）

| # | 项 | 证据 |
|---|----|------|
| 1 | **分享链接「撤销即永久失效 + 轮换」** | Metabase：关闭后重新分享会生成**不同**链接、旧链接**永久失效**（metabase.com/learn，2026-05） |
| 2 | **分享清单 + 一键失效** | 网易有数项目中心统一管理；Quick BI「不再公开」；永洪取消分享。**国内厂商普遍缺、DataRay 也缺** |
| 3 | **「任一改动即时出图」**（守住，不要退化） | Metabase 凭此拿到摩擦度第一（见 §十五） |
| 4 | **过期默认值 ≠ 永久** | FineBI 默认永久有效、Smartbi 自动 4 位密码、永洪密码 1–8 位——都是被吐槽的妥协 |
| 5 | **至少接一个 MCP server** | Metabase v60（2026-05）· Superset 6.0（2025-12）· Cube（2025-06）· Rill（2026 初）。成本远低于自研 NL2SQL |
| 6 | **「快照即真相」——不可变快照分享** | 所有人都在卖「定义一次、到处一致」，**没人卖「这张图当时的数据长什么样」** |

### B. 不该抄的（过度设计）

多租户 / RLS / SSO / SCIM（Metabase 压到 $575/mo + $20k/yr）· 重型语义层（Cube 的手写 YAML/JS + pre-aggregation 是「ongoing discipline」+ 单模型只解析单源；反证：**DataEase 用弱语义层就跑通了中小企业**）· 数据大屏 / Canvas 像素布局（与「分析」主干正交）· 定时调度订阅（Lightdash 自托管要额外拉 browserless + SMTP + S3）· BI-as-code 路线（Evidence 明确是 **publishing 而非 exploration**）· pre-aggregation/缓存引擎（CCU 计费不可预测）。

### C. 反向定位机会

| # | 机会 | 论证 |
|---|------|------|
| 1 | **「内网也按最小暴露原则分享」** | 反例有实证：Redash 的 `public_url_token` 无密码无过期 → **跨组织数据泄露生产复盘**；Metabase 官方警告公共链接「不能靠 filter 藏数据」。**本阵营仅 DataEase 与国内少数几家有密码** → DataRay 的「密码 + 过期」**正确且稀缺** |
| 2 | **把 AI 放进免费自托管** | Lightdash / Evidence 把 AI 锁商业云档；**Metabase 反向操作**（v60 全套餐开源 + BYO key），官方理由：「用户想要 AI 就会绕过你，把数据贴进 ChatGPT」 |
| 3 | **零依赖 / 零运维** | Superset 匿名分享要 4 项配置 + `superset init` + 授 6 类权限，guest token 还要 JWT secret + CORS + SameSite 全对齐（社区大量「嵌入面板闪一下被踢回登录页」）；Lightdash 绝对依赖 dbt；Cube 语义模型不可视化。→ **Go 单二进制 + 四方言直连 + 不锁建模工具** |
| 4 | **免登录的人也能问一句** | ThoughtSpot Pro 限 **25 次 Spotter/人/月**、Qlik 免费档仅 25 次提问 → 小团队买不起。**巨头因定价结构无法跟进这个位置** |

### 本节的诚实边界（必须同时成立）

1. **免登录分享在国内早已是标配，不是差异点**——Quick BI / FineBI / 观远 6.6 / 网易有数 / Smartbi / 永洪，主流 6 家全有，5 家同时支持密码与有效期。**真正的差异点在「默认行为 + 不可猜 + 零配置」，不在「有没有」。**
2. **差异化窗口只在无鉴权阶段存在**。一旦上多租户或 RBAC，这套机制会立刻退化成国内同款「管理员开关 + 合规补丁」。→ 排期上 **Phase A/B 要快**。
3. **DataEase 是唯一在分享机制上强于 DataRay 的对手**（有效期 + 密码 + 自定义后缀 + Ticket 参数绑定）。DataRay 无法在分享机制上取胜，**只能靠「问题导向 vs 大屏导向」与「零配置」错位**。
4. **最强对手可能不是产品，是范围蔓延**：图型已 13 种、越过断点，**继续加图型的边际收益趋近于零**。

---

## 📚 第二轮数据来源

- **开源阵营（竞析，2026-09 核）**：superset.apache.org；preset.io（6.0 发布说明 2025-12、MCP `execute_sql` RLS 漏洞修复 2025-12）；github.com/apache/superset discussion #31949（2025-01，匿名分享配置）；npmjs.com `@superset-ui/embedded-sdk`；metabase.com/pricing（2026-09 核）；metabase.com/releases/metabase-60（2026-05）；metabase.com/learn + github.com/metabase docs（公共链接轮换，2026-05）；codatum.jp（Redash 版本对照 v25.01/25.08/26.03）；perun.au（Redash 泄露生产复盘）；usagepricing.com（Grafana Assistant 时间线 + AGPLv3 重许可）；grafana.com/pricing（2026-09 核）；docs.aws.amazon.com（Grafana v10 Snapshot 分享）；adriennevermorel.com（Lightdash 自托管实战缺口）；portalzine.de（Evidence Studio 2025-06、Rill MCP）；atlan.com（Headless BI 横评、Lightdash MIT core + ee/）；colrows.com（Cube 定价与运营代价，2026-06）；dev.to（Cube D3 对比）；dataease.cn / dataease.io（公共链接与 Ticket 机制、v2.10 版本节奏、社区版四项边界，2025-09-01）；deepwiki.com/dataease（源码分析）
- **国内阵营（竞析，2026-09 核）**：各厂商官网 / 帮助文档 / 定价页；火山引擎 DataWind 官方文档（自定义插件 2026-07-03、AI 直连数据集支持范围）；帆软 7.0 发布（2025-09）；观远社区帖（公开链接匿名访问，2024-09-06）；阿里云 Quick BI 计费文档（智能问数阶梯降价 2025-08-07）；腾讯云社区（腾讯云 BI 定价，二手）；网易有数 / Smartbi / 永洪 / 衡石 / DataFocus / Sugar BI 官网与华为云市场页（2025-09）；百度 Sugar BI 私有化报价
- **国际 + AI 原生（竞析，2026-09 核）**：tableau.com/pricing（2026-09 抓取）+ help.tableau.com（Show Me）+ tableau.com（Tableau MCP，2026-07）；aiagentsquare.com 引官方（Fabric Copilot 下放，2026-07）；cloud.google.com 经 colrows.com（Looker Gemini Data Tokens 计量，2026-06）+ discuss.google.dev（Visualization Assistant GA，2026-04）；qlik.com/us/pricing（2026-09 抓取）；thoughtspot.com/pricing（2026-09 抓取）；sigmacomputing.com；github.com/canner/wrenAI（2026-05-07 仓库并归）；everydev.ai / aicloudbase.com（Vanna 2.0.2，2026-02-02）；forwardfuture.ai（Julius，2026-08）；delv.tools（Hex 无自托管）；exploreomni.com（Series C 2026-04-23）；thoughtspot.com 新闻稿（Mode 被收购 2023-06）；shahvatsal.com（BIRD/Genie Ontology，2026-06）；pointfive.co（Snowflake Cortex Sense，2026-06）；mantissaai.com / promethium.ai（AI 准确率折扣，二手）；algorcomp.pl（dbt Labs 语义层 benchmark，二手）
- **已知未验证项（不作为结论依据）**：Preset 托管云档位 · Evidence Hosted/Cloud 档位 · Rill Cloud 具体价格 · DataEase 企业版价格 · Superset GitHub star 数（官网 72,804 vs 第三方 63,000，口径冲突）· Sigma 官方定价（官方页无价格，两处第三方口径冲突）· 火山 DataWind 是否提供「匿名公开链接 + 密码/有效期」· 少数国内厂商定价（标「二手」）
- **方法警示（重要）**：2025–2026 大量第三方对比文宣称 Redash「已消失 / 停滞 / 被行业除名」，GitHub 事实是它按月在发版（v25.01 / v25.08 / v26.03）。→ **竞品事实一律以官方文档 + GitHub release 为准，第三方对比文只作体验口碑参考。**

---

## 📚 数据来源 & 成员产出索引

- **方向明（主理人）**：仓库实测取证（**下述为 2026-09-19 复核后的口径，已修正第一轮快照的过时项**）——`api/openapi.yaml` 共 23 个 path；9 个前端页面；4 个后端业务领域 + `queryrecord`；`redis|asynq|celery|cron` 全仓 0 命中；**无鉴权**（`backend/internal/` 下无 auth 中间件，`jwt|session|login` 仅在测试文件出现）；`backend/migrations/` 现有 **3 个迁移**（00001 初始 / 00002 软删除 / 00003 `bi_query`）；`owner_id`/`tenant_id` **仅 `model/query.go:34-35` 有**——既有 4 张业务表仍未落库（不可逆欠账）；**图型 13 种 / 聚合 7 种**（`backend/internal/query/types.go:7-21` 与 `frontend/src/components/ChartBuilder/chartDefinitions.ts` 双端一致）；`/api/queries` **实测 404**（curl `:23352`）→ 查询记录端点未接线。
  - ⚠️ **第一轮快照已过时，勿再引用**：原文记「仅 1 个迁移文件」「31 个 API 端点」「9 个前端页面」，其中迁移数与端点数均已变化；「`tenant` 在 `backend/internal/**` 0 命中」在 `bi_query` 落地后**不再成立**。
- **竞析（竞品分析师）**：
  - Superset 官网 https://superset.apache.org/（高）；6.0 发布说明 https://preset.io/blog/apache-superset-6-0-release（高）；5.0 说明 https://preset.io/blog/superset-5-0-0-release-notes/（高）
  - Metabase 版本支持页 https://www.metabase.com/version-support（高）；v59 https://www.metabase.com/releases/metabase-59（高）；AI/Data Studio 汇总 https://www.metabase.com/releases-ai（高）
  - Redash 现状、Lightdash 语义层对比、开源 BI 治理矩阵、Grafana 对比、国内 BI 对比（均为二手中信度，多源交叉验证）
  - **未核实/低置信**：Superset 原生 AI（InsightGPT / Ask Data）来自疑似 AI 生成博客，与官方版本线不符，**判定不可信**；Superset 精确图表类型数（40+ 与 50+ 口径不一）为中低置信。
- **瑞思（用户研究员）**：BI 平均采用率约 26%、多数回退 Excel（全球调研转引，中置信）；80%+ 组织部署商业 BI 后仍依赖 Excel（LinkedIn 从业者引 surveys，中低置信）；艾瑞《2025 中国 BI 市场落地调研报告》转引" >70% 企业 BI 上线后核心经营报表仍在线下 Excel"（二手，中置信）；"10–15 秒加载 = 弃用"（从业者共识，中置信）；Metabase 权限粒度粗 / Superset 学习曲线陡（CSDN + Domo 对比表，中置信）；自研 BI 最大隐性成本是核心开发离职 → 系统冻结（定性，中低置信）；60%+ 看板 6 个月无人查看、最受欢迎功能是"Download as CSV"（单一咨询方，低-中置信）。**角色反应、场景断点与门槛分级为分析推断，需真实访谈校准。**
- **数析（数据分析师）**：评分卡与 SLO 中的二手中值来自 Metabase / Superset 厂商博客与第三方评测（置信中低，需本机压测复算）；冷缓存 P95、超时率/失败率阈值、可靠性表目标、TTFV 阈值均为**团队建议值**（置信低）；DataRay 现状自评来自主理人快照。建议用 TPC-DS 三档数据集建立唯一权威测量入口。
- **析客（需求分析师）**：差距矩阵、R-01~R-18 需求池、T-01~T-06 技术改造项、Non-goals、7 项待确认问题。
- **路径（路线图规划师）**：五阶段路线图、工作量与节奏测算、关键路径与并行机会、风险登记表、M0–M4 里程碑、三版利益相关者沟通要点。

---

> 本报告由产品战略团队 AI 协作生成，重要决策请由产品负责人审定。
