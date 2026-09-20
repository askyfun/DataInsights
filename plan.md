# Data Insights 图表查询与可视化语义演进计划

## 今日完成总结

- [x] 将 `plan.md` 从说明文档重构为可执行待办清单。
- [x] 明确本轮改造遵循的原则：
  - [x] Chart 模块负责图表语义。
  - [x] Query 模块负责查询语义。
  - [x] 改代码必须走 TDD。
  - [x] 遇到语义不清先和用户确认。
- [x] 与用户确认过滤条件 `logic` 语义：
  - [x] `logic` 表示“当前条件与前一个条件的连接方式”。
  - [x] 第一个条件忽略 `logic`。
  - [x] 当前阶段只支持线性表达式，不支持括号分组。
- [x] 完成图表查询目标契约草案文档：
  - [x] `docs/api-spec.md`：补充图表查询契约演进草案。
  - [x] `docs/api.md`：补充图表查询兼容协议说明与目标契约草案。
  - [x] `docs/chart-builder-plan.md`：补充契约演进方向。
  - [x] `docs/architecture.md`：补充图表查询与可视化语义分层。
  - [x] `AGENTS.md`：更新文档索引说明。
- [x] 完成过滤条件 `AND/OR` 问题修复（TDD）：
  - [x] 先新增失败测试：`TestBunQueryBuilder_WithFilterLogicOr`。
  - [x] 先新增失败测试：`TestBunQueryBuilder_WithMixedFilterLogic`。
  - [x] 修复 `backend/internal/query/bun_builder.go` 中 WHERE 拼接逻辑。
  - [x] 通过针对性测试回归。
  - [x] 通过 `go test ./internal/query` 回归。

## 当前总体目标

- [ ] 建立统一的图表语义契约，明确 Chart 模块负责视觉语义，Query 模块负责查询语义。
- [ ] 在兼容现有 `dims` / `metrics` 请求的前提下，引入更强的 `ChartSpec` / `QuerySpec` / `QueryAST`。
- [ ] 统一前后端响应结构和文档，支持未来复杂图表扩展。
- [ ] 所有代码改造继续使用 TDD：先写失败测试，再实现，再回归验证。

## 下一步执行计划（本轮）

本轮目标：不继续扩散新能力，先把已经定义好的 `ChartSpec / QuerySpec` 真正接入后端执行链路，并为后续 `QueryPlanner` 留出稳定落点。

### Step 1：补齐 service 层接入测试，锁定兼容目标

- [x] 先为 `backend/internal/service/chart` 补失败测试，覆盖 `ChartQueryRequest -> QuerySpec -> SQL/响应` 的真实链路。
- [x] 至少覆盖表格、折线图、散点图三类基础图表，确保兼容现有 `dims / metrics / filters / sort / pagination`。
- [x] 明确成功标准：接入新链路后，现有接口响应 shape 不变、SQL 行为不回退。

验证：运行 `go test ./internal/service/chart ./internal/query`，并确认新增测试在改代码前失败、改后通过。

### Step 2：将 QuerySpec adapter 接入 chart service 主路径

- [x] 在 `backend/internal/service/chart/impl.go` 中收敛旧的平铺参数组装逻辑，统一改为先生成 `QuerySpec`，再走兼容 adapter 下沉到 builder。
- [x] 保持 handler / IDL / 前端请求结构不变，不在本轮引入新的外部 API 字段。
- [x] 清理本次改动引入的重复转换逻辑，避免 service 层同时维护两套语义入口。

验证：运行 `go test ./internal/service/chart ./internal/query`，并抽查生成 SQL 与当前契约一致。

### Step 3：补一个最小 QueryPlanner 雏形，而不是直接做完整 AST 升级

- [x] 新增最小 `QueryPlanner` 入口，职责仅限 `QuerySpec -> 旧 builder 入参` 或 `QueryAST` 的兼容规划。
- [x] 当前只支持已有能力：基础维度、基础聚合、线性 filters、sort、pagination。
- [x] 暂不实现时间粒度、分桶、过滤组嵌套，只为后续 P2 留清晰扩展点。

验证：为 planner 补单元测试，确保输出与 `QuerySpecToBuildArgs` 当前行为等价。

### Step 4：同步文档，更新“已完成/下一步”状态

- [x] 更新 `docs/architecture.md`，明确 service 层已从“直接平铺请求”切到“先 QuerySpec 再规划”。
- [x] 更新 `docs/api.md` / `docs/api-spec.md`，说明当前仍是兼容期，对外请求结构未变。
- [x] 回写 `plan.md` 当前状态，标记本轮完成项和下一轮入口。

验证：人工检查文档描述与代码入口一致，不出现“文档说已接入、代码还没接”的失配。

## 本轮边界

- [ ] 不修改前端请求协议。
- [ ] 不落地统一响应 Envelope。
- [ ] 不实现时间粒度、分桶、复杂过滤组。
- [ ] 不顺手重构 processor / handler 无关代码。

## 本轮主要风险

- [ ] `chart service` 里可能已有隐式依赖旧参数顺序，接入 `QuerySpec` 后若测试覆盖不足，容易出现图表类型回归。
- [ ] `sort` 既支持原字段也支持聚合别名，若 planner/adapter 处理不一致，可能再次引入排序错误。
- [ ] 散点图语义已特殊约定为“两个 metric”，接入新链路时要防止被通用维度/指标映射误伤。

## 本轮完成标准

- [x] `ChartSpec / QuerySpec` 不再只是类型定义和测试样例，而是真正进入 chart 查询主路径。
- [x] 基础图表查询链路保持兼容，无新增外部接口破坏。
- [x] 新增测试能覆盖 service 入口和 planner 兼容层。
- [x] 文档与 `plan.md` 状态同步完成。

## 下一次开工优先顺序

### P0：继续修当前确定问题

- [x] 第三步：修复散点图前后端语义不一致，采用 TDD。
  - [x] 先读当前散点图前端渲染逻辑。
  - [x] 先读当前散点图后端 `ScatterProcessor` 逻辑。
  - [x] 明确当前契约到底是”两个 metric”还是”一个 dim + 一个 metric”。→ 确认方案 A：两个 metric。
  - [x] 与用户确认后改代码。
  - [x] 先补失败测试，复现前后端不一致问题。
  - [x] 最小改动修复实现。
  - [x] 跑针对性测试。
  - [x] 跑 `go test ./internal/query` 回归。
  - [x] 同步更新文档。

- [x] 第四步：修复分页与排序规则，采用 TDD。
  - [x] 梳理前端表格排序请求是否真正传给后端。
  - [x] 梳理后端分页时为什么跳过 `ORDER BY`。
  - [x] 先明确目标规则：分页查询是否必须带排序、默认排序字段是什么。
  - [x] 先补失败测试。
  - [x] 最小改动修复实现。
  - [x] 跑针对性测试。
  - [x] 跑 query / service 相关回归测试。
  - [x] 同步更新文档。（行为修复，文档已正确描述 sort 参数，无需更新）

### P1：后端内部模型升级

- [x] 新增内部 `ChartSpec` 类型。
  - [x] 定义基础结构：`chart_type`、`dimension_groups`、`metric_groups`、`style`、`query_options`。
  - [x] 为字段绑定预留附加属性：`label`、`granularity`、`agg`、`alias`、`unit`。
  - [x] 为该结构补单元测试或序列化测试。
- [x] 新增旧请求 → `ChartSpec` 的 adapter。
  - [x] 先补 adapter 测试。
  - [x] 保证旧 `dims` / `metrics` 能映射到默认维度组和指标组。
- [x] 新增 `QuerySpec` 类型。
  - [x] 定义结构化维度表达式。
  - [x] 定义结构化指标表达式。
  - [x] 定义过滤、排序、分页、limit 能力。
- [x] 新增 `ChartSpec` → `QuerySpec` 的转换。
  - [x] 先补转换测试。
  - [x] 保证基础图表在兼容模式下生成 SQL 不变。

### P2：增强 QueryAST

- [x] 将维度从字符串升级为结构化表达式。
  - [x] 字段名。
  - [x] 展示名。
  - [x] 时间粒度。
  - [ ] 分桶。
  - [ ] 格式化信息。
- [x] 将指标升级为结构化表达式。
  - [x] 聚合。
  - [x] 别名。
  - [x] 单位。
  - [x] 数值格式。
  - [ ] 是否累计 / 占比。
- [ ] 为日期维度增加统一时间粒度能力。
  - [x] PostgreSQL 方言测试。
  - [x] MySQL 方言测试。
  - [x] ClickHouse 方言测试。
- [ ] 增强过滤能力。
  - [ ] 过滤组结构设计。
  - [ ] 时间范围过滤。
  - [ ] 空值过滤。
  - [ ] 字段类型感知。
- [ ] 引入 `QueryPlanner`。
  - [x] 定义最小 `QuerySpec -> 兼容旧执行链路入参` 规划流程。
  - [x] 为基础图表补规划测试。
  - [x] 落地第一阶段 `QuerySpec -> QueryAST` 规划流程。
  - [x] 打通最小执行链路：service 附带 `PlannedAST`，executor 优先直接消费 `QueryAST`。
  - [ ] 后续移除平铺参数兼容回退，收敛到统一 AST 执行链路。

### P3：前端 ChartBuilder 升级

- [x] 前端新增 `ChartDefinition` registry。
  - [x] 折线图定义。
  - [x] 透视表定义。
  - [ ] 双轴图定义。
  - [x] 饼图定义。
  - [x] 表格 / 柱状图 / 面积图 / 散点图基础定义。
- [x] ChartBuilder 根据图表定义动态渲染维度组和指标组。
  - [x] 第一阶段仅落地“定义驱动标签/空态/最小 group 数量补齐”。
  - [x] 已补齐多 group 基础绑定：不同槽位的添加/删除/重排可作用于对应 group。
  - [x] 已补页面级回归测试入口：覆盖拖入不同 group 的基础行为。
  - [ ] 后续继续补齐多 group 精确拖拽交互与更完整组级操作。
- [ ] 支持维度字段属性配置。
  - [ ] 日期精度。
  - [x] 展示名（已支持保存到图表配置并在编辑时恢复，表格/图表预览可消费）。
- [ ] 支持指标属性配置。
  - [x] 聚合（已支持保存到图表配置并在编辑时恢复）。
  - [x] 别名（已支持保存到图表配置并在编辑时恢复）。
  - [x] 单位（已支持保存到图表配置并在编辑时恢复，图表预览可消费）。
  - [x] 格式化（已支持保存到图表配置并在编辑时恢复，当前先完成配置持久化）。
- [ ] 支持图表样式配置。
  - [x] 颜色。
  - [x] 线型。
  - [x] 表格行高。
- [ ] 支持图表特有查询配置。
  - [x] 饼图低比例合并为“其他”（前后端最小闭环已完成，后端已消费该 query option）。

### P4：统一响应结构落地

- [ ] 后端返回统一响应 Envelope，并兼容旧的 `select_sql` / `count_sql` 顶层字段。
  - [ ] 定义 `shape`。
  - [ ] 定义 `meta`。
  - [ ] 定义 `fields`。
  - [ ] 定义 `sql`。
- [ ] 前端 Store 按 `shape` 解析响应。
- [ ] SQL 查看弹窗改为从统一 `sql` 字段读取。

## 文档同步待办

- [x] 更新 `docs/api.md`。
- [x] 更新 `docs/api-spec.md`。
- [x] 更新 `docs/architecture.md`。
- [x] 更新 `docs/chart-builder-plan.md`。
- [x] 更新 `AGENTS.md` 中与图表构建、查询契约相关的说明。
- [ ] 后续每做完一个代码阶段，再同步更新对应文档和示例。

## 测试与验收规则

- [x] 每一个 bug 修复都先补失败测试。
- [ ] 每一个契约升级都补前后端类型/单元测试。
- [ ] 后端关键阶段提交前运行目标测试，必要时运行 `go test -race ./...`。
- [ ] 前端改造阶段运行相关 Vitest 测试。
  - [ ] 当前受本地 Node/pnpm 与 Vitest/jsdom ESM 环境问题影响，自动测试待环境修复后补跑。
  - [x] 已补页面级与 store 级测试用例，等待环境修复后执行。
- [x] 本轮静态与后端验证已完成：`biome check`、`tsc --noEmit`、`go test ./internal/query`、`go test ./internal/service/chart`。
- [ ] 每完成一个阶段，立即把对应任务标记为 `[x]`。

## 今天停在这里时的状态

- [x] 文档契约草案已完成。
- [x] `AND/OR` 过滤问题已完成并验证。
- [x] 散点图语义一致性已完成（方案 A：两个 metric，前端已修复）。
- [x] 分页与排序规则已完成（后端 ORDER BY 不再被分页跳过，前端 sort 参数已接入请求链路）。
- [x] `ChartSpec` / `QuerySpec` 类型定义与 adapter 已完成（`chart_spec.go`）。
- [x] `ChartSpec` → `QuerySpec` 已接入 `chartService.Query` 主路径。
- [x] 前端 `ChartDefinition` registry 第一阶段已落地，`ChartBuilder` 已可按图表定义动态渲染组标签与空态文案。
- [x] 前端 `ChartBuilder` 第二阶段已完成第一批：多组字段基础绑定已接通。
- [x] 前端 `ChartBuilder` 已补齐指标属性配置的最小持久化：`agg/alias` 可随图表配置保存并在编辑时恢复。
- [x] 前端 `ChartBuilder` 已补齐一轮最小配置模型：维度 `label`、指标 `unit/format`、样式配置、饼图 query option 均可保存并恢复。
- [ ] 前端 `ChartBuilder` 后续待做：维度 `granularity`、更完整的配置 UI、后端继续消费 `unit/format/style` 等配置项，以及更完整的多组拖拽交互。
- [x] 最小 `QueryPlanner` 已落地，并已接入 `QuerySpec -> 旧 executor 请求` 的兼容规划链路。
- [x] `QueryPlanner.PlanAST` 已落地，`QueryAST` 第一阶段已支持结构化维度/指标、`limit` 和 `day` 粒度多方言 SQL 生成。
- [x] `chartService.Query -> query.Executor` 已打通最小 `PlannedAST` 直连路径，executor 在内部优先消费 AST 生成 SQL。
- [x] service 层兼容测试已覆盖 table / line / scatter 主链路。
- [ ] 下一轮优先移除平铺参数兼容回退，继续收敛到统一 `QueryAST` 执行链路，再推进更多时间粒度、分桶、复杂过滤组能力。
