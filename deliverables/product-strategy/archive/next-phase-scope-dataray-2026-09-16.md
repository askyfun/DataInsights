> ⚠️ **已归档（superseded）——本文件不再指导实现，仅作决策轨迹留档。**
> 现行文档：`../README.md`（索引）· `../prd-chart-query-share-2026-09-16.md`（产品规格）· `../roadmap-data-insights-2026-09-16.md`（路线图）· `../decision-log-data-insights-2026-09-16.md`（决策台账）
> 被取代原因：其 Phase 0 / Phase 1 清单在看板移出当期、分享机制切换为查询记录落库后已整体重排。

---

# Data Insights 下一阶段功能清单（Phase 0 地基 + Phase 1 体验核心）

**日期**：2026-09-16
**类型**：开工清单（自路线图 v2 收敛）
**参与成员**：析客（需求分析师）· 路径（路线图规划师）· 数析（数据分析师）· 瑞思（用户研究员）· 竞析（竞品分析师）
**来源**：`roadmap-update-data-insights-experience-first-2026-09-16.md`（体验优先版路线图 v2）

---

## 📌 TL;DR（执行摘要）
- 下一阶段 = **Phase 0「当期地基」+ Phase 1「体验核心」**，合计约 **102–145 人日（估算）**。Phase 0 不做完不要碰 Phase 1——它含 3 项**一票否决门禁**与 2 项**不可逆埋点**，逾期补不回来。
- Phase 0 共 **8 项**：3 门禁（expr 校验 / 单条 SELECT+只读账号 / 凭据脱敏）+ 虚拟字段引用校验 + 使用留痕 + owner/tenant 埋点 + 统一 filter-state + 统一错误契约。
- Phase 1 共 **7 组功能**：指标语义层（度量级表达式）、看板 + 全局筛选、跨图联动/级联、拖拽建图增强、图表内多维度分组与钻取、表达式编辑器、逐阶段预览诊断。
- 关键路径：**G1 校验器 → 错误契约/filter-state → 指标层 → 看板 → 联动级联**；其中 **3 组可与指标层并行**（表达式编辑器 / 逐阶段预览 / 分组钻取）。
- 有 2 项是「后端已就绪、只差接线」的便宜活（时间粒度 UI、TopN+"其他" 死链），可随时插入。

---

## 🎯 核心结论卡片

| 项目 | 内容 |
|------|------|
| 推荐方案 | 先 Phase 0 全量（8 项，含 3 门禁 + 2 不可逆埋点），再 Phase 1 体验核心（7 组） |
| 优先级 | G1/G2/G3 门禁与 R-08 留痕为**准入项**；Phase 1 中 R-06 指标层与 R-04 看板为关键路径先头 |
| 预期影响 | 通关后：看板域 5% → ≥75%；建模/查询/可视化/看板四域各 ≥75%；加权自评 49.3% → ≥75% |
| 资源需求 | Phase 0 **29–42 人日**；Phase 1 **73–103 人日**；合计 **102–145 人日（估算）** |
| 风险等级 | **高**——主要来自 expr 校验做成黑名单被绕过、窗口函数方言未确认、无缓存下 P95 不达标 |

---

## 一、Phase 0：当期地基（约 29–42 人日）

> 这一批与"体验优先还是权限优先"无关。不做，后面全是返工或违规。

### 1.1 门禁 G1 — 表达式通路校验器（T-07）｜**已有需补**｜M 5–8 人日
- **做什么**：给 `expr` 通路补白名单校验（当前通路已有、**校验为零**）。
- **落到哪**：新建 `backend/internal/expr/validate.go`（`Validator`）；`service/dataset/impl.go:206 UpdateColumns` 落库前校验；`query/bun_builder.go:26 WithColumnMappings` 注入前二次校验（防旁路）；复用/扩展 `query/sanitizer.go` 的 `IsValidIdentifier`。
- **配套**：`POST /api/expr/validate` → `{valid, errors[], normalized}`（Phase 1 的表达式编辑器直接复用）。
- **验收**：注入用例（子查询 / `;` 堆叠 / `--`、`/* */` 注释 / `UNION` / DDL / DML）**100% 被拒**并返回契约化错误；先写失败测试再改。
- **注意**：必须是**白名单 + AST 复核**，黑名单会被绕过。

### 1.2 门禁 G2 — 统一查询执行 + 只读账号（T-08）｜**需新建**｜M 4–6 人日
- **做什么**：查询出口只能出**单条 SELECT**；连接强制只读账号。
- **落到哪**：`service/query.Guard`（新）；`service/chart/impl.go` 查询出口统一过 Guard。
- **验收**：`;DROP` / `;UPDATE` / `into outfile` / `load_file` 全部拒绝。

### 1.3 门禁 G3 — 凭据加密 + 响应脱敏（T-09）｜**已有需补**｜S 2–3 人日
- **现状**：`crypto`（AES-GCM）与 `password json:"-"` 已在，本项是**核对全路径 + 测试固化**。
- **验收**：库中口令非明文；所有 API 响应零凭证字段。

### 1.4 虚拟字段引用校验（T-02）｜**已有需补**｜S 2–3 人日
- **做什么**：`expr` 里 `[field]` 引用的字段必须存在于 `bi_dataset.columns`；检测自引用/循环。
- **验收**：引用不存在的列 → **保存即报错**（而非查询时才报错、汇报当天图表空白）。

### 1.5 使用留痕（R-08）｜**需新建 · 不可逆**｜M 5–7 人日
- **为什么现在做**：查询历史**不记就永久丢失**，事后无法重建"当时谁看了什么"。
- **落到哪**：新迁移 `00002_query_log.sql` → 表 `bi_query_log(id, owner_id, tenant_id, dataset_id, chart_id, query_sql, source_type, row_count, duration_ms, action, ip, created_at)`；`chart/impl.go` 查询出口写日志（不阻塞主查询）；端点 `GET /api/audit/queries`。
- **验收**：任意一次查询/导出/分享查看可追溯"谁 / 何时 / 查了什么 / 多少行"。

### 1.6 owner / tenant 前置埋点（R-22 = T-02 的实现）｜**需新建 · 不可逆**｜S 3–4 人日
- **做什么**：**只加字段、不做鉴权**——把不可逆项转成可逆项。
- **落到哪**：`bi_dataset` / `bi_chart` / `bi_share` 加 `owner_id`、`tenant_id`（nullable）；goose 增量迁移"加可空列 → 回填 → 加约束"；存量默认归当前唯一账号。
- **现状核实**：全库仅 5 张表（`bi_datasource` / `bi_dataset` / `bi_dataset_lineage` / `bi_chart` / `bi_share`），**无任何 owner/tenant 维度**。
- **验收**：新实体落库即带 owner；不引入任何鉴权逻辑。

### 1.7 统一 filter-state 模型（R-25）｜**需新建**｜M 5–7 人日
- **为什么现在做**：看板全局筛选、跨图联动、级联筛选**全部依赖它**；后补等于重做看板。
- **落到哪**：前端 `FilterState` 类型 + 跨图共享的 store 抽象；后端筛选值参数化进入 `QuerySpec`。
- **验收**：多个图表组件共享同一份筛选状态并各自正确取值。

### 1.8 统一错误契约与可诊断性（R-26）｜**已有需补**｜S 3–4 人日
- **现状**：统一响应信封已有，缺**错误码 + 修复建议**字段。
- **落到哪**：`response/` 补 `error_code` 与 `fix_suggestion`；错误分类（语法 / 字段不存在 / 方言不支持 / 超时 / 权限）。
- **验收**：查询失败时前端能给出可执行的修复建议（错误可诊断性 ≥80%）。

**Phase 0 出口 Gate（M0）**：G1/G2/G3 用例全过 + 留痕写入断言 + 引用校验保存即报错 + 单测全绿。

---

## 二、Phase 1：体验核心（约 73–103 人日）

> 目标：从"能画图"变成"愿意天天用"。出口验收对齐数析的用例集（SC-x / MC-x）。

### 2.1 指标语义层 + 命名 metric 实体（R-06）｜**已有需补**｜XL 15–20 人日 ← 关键路径先头
- **缺口本质**：现有 `MetricField{Field, Agg, Alias, Unit, Format}` = **单字段 + 单聚合**，无法表达 `sum(a)/sum(b)`、同环比、累计、占比。行级复合（`amount*0.3` 被 `sum()` 包住）**已可用**，缺的是**聚合后表达式层**。
- **落到哪**：新包 `service/metric` + `domain/entity/metric.go`；表 `bi_metric(id, dataset_id, name, expr, metric_type, agg, format, unit, owner_id, tenant_id)`；`metric_type` 枚举 `simple | ratio | percent_of_previous | running_total | percent_of_total`；`QuerySpec` 增 `metric_refs`；`query/ast.go` 加 `ApplyMetricMappings` 扩展 `GetMetricFieldExpr`；`QueryPlanner` 把 metric 编译为 AST 片段；端点 `/api/metrics` CRUD；前端 `pages/Metrics.tsx`。
- **验收（SC-2）**：两图引用同一指标；改一次口径两图同步生效；数值与手工 SQL 一致；**改口径手动操作次数 = 0**。
- **前置**：窗口函数方言探针（见 §四）。

### 2.2 看板实体 + 全局筛选（R-04）｜**需新建**｜XL 18–25 人日 ← 最大缺口
- **落到哪**：表 `bi_dashboard(id, name, owner_id, tenant_id, layout jsonb, global_filters jsonb)` + `bi_dashboard_chart(dashboard_id, chart_id, position)`；包 `service/dashboard`；端点 `/api/dashboards` CRUD + `/api/dashboards/{id}/render`（带筛选参数）；前端 `pages/Dashboard.tsx` + `DashboardEdit`（12 列栅格 + @dnd-kit）。
- **全局筛选模型**：照 Metabase——**控件 → 卡片列映射 + 自动连接含同字段的其它卡片 + 默认值**（比 Superset 的 scope/DataMask 易实现）。
- **验收（SC-3A）**：8 图拼装保存后刷新持久化；筛选作用于 ≥2 张卡；首屏热 P95 ≤2s。

### 2.3 跨图联动 + 级联筛选（R-05）｜**需新建**｜L 8–12 人日
- **落到哪**：cross-filter 事件总线 + 已有 filter-state；级联**放宽为认 dataset 字段**（含虚拟字段/join 别名），不依赖物理 FK——这是 Metabase Linked filters 的已知坑。
- **验收（SC-3B / 3C）**：点击图中维度元素联动其它图；父筛选变化后子筛选选项收敛于父域。

### 2.4 拖拽建图增强（R-08）｜**已有需补**｜L 8–12 人日
- **做什么**：字段列表分区 + 语义类型图标 + **搜索** + 折叠；拖拽入槽高亮 / 空满态 / 可重排；**时间粒度与数值分箱改为 inline 点选**。
- **便宜活**：**时间粒度后端已就绪**（`QueryAST` 支持 `day`，PG/MySQL/CH 方言已测，契约生成物有 `granularity`），但前端 `pages/`、`components/`、`store/`、`api/` **全部零引用**——只差接线。
- **验收**：建图 P50 ≤3min、≤5 步；80+ 列数据集字段定位 P50 ≤15s、滚动 ≤2 屏。

### 2.5 图表内多维度分组与钻取（R-21）｜**已有需补**｜L 10–14 人日 ← 你说的"分组"
- **现状**：维度组/指标组与透视表已有；**缺**多维度 breakout、透视小计/合计/行列交换、TopN+"其他"、下钻。
- **落到哪**：`query/ast.go` 支持 breakout 数组（同列多粒度）+ 排序影响 cumulative/offset；透视扩 rows/cols total·subtotal 与 transpose（照 Superset Pivot v2）；`chart/impl.go` 返回 drill 元数据；构建器加 breakout 槽 / 小计开关 / TopN 控件。
- **便宜活**：**TopN+"其他" 是死链**——后端 `processor.go` 的 `MergeOtherBelowRatio` 有能力、store 有 `pieMergeOtherBelowRatio` 字段，**但前端页面从不发送**。接线即可。
- **验收（MC-3 / SC-3D）**：透视小计 = 明细和（数值闭合）；TopN 末位含"其他"；点击维度可下钻。

### 2.6 表达式编辑器（R-19）｜**需新建**｜M 6–8 人日
- **落到哪**：`components/ExprEditor.tsx` —— 函数浏览器 + autocomplete + **实时校验（复用 G1 校验器）** + auto-format + **非法时禁用 Done**。
- **验收**：非法表达式在编辑期即被拦下并给出可读原因。

### 2.7 逐阶段预览与"为什么查不出来"诊断（R-20）｜**需新建**｜L 8–12 人日
- **为什么重要**：这是 **expr 零校验的非权限解药**——把"静默失败"变成"看得见的错在哪儿"。
- **落到哪**：`chart/impl.go` 返回 `stages[]`；前端每阶段小三角预览 + View SQL + 错误面板（依赖统一错误契约）。
- **验收（SC-1B）**：故意输入 `[amount] *` → 由当前"静默失败"变为红字提示 + 可执行修复建议。

---

## 三、开工顺序与并行机会

```
Phase 0（先做，全量）
  G1 校验器 ─┬─→ 错误契约 ─┐
             │             ├─→ Phase 1
  G2/G3 ─────┘  filter-state ┘
  留痕 / owner 埋点 / 引用校验（独立，可穿插）

Phase 1 关键路径：  R-06 指标层 → R-04 看板 → R-05 联动级联
并行轨（只依赖 G1，可与 R-06 同时开工）：R-19 表达式编辑器 · R-20 逐阶段预览 · R-21 分组钻取
```

- **必串行**：G1 → 错误契约/filter-state → R-06 → R-04 → R-05。
- **可并行**：R-19 / R-20 / R-21 与 R-06 无强依赖；R-05 与 R-04 可部分并行（filter-state 已在 Phase 0 定型）。
- **建议插空的便宜活**：时间粒度 UI 接线、TopN+"其他" 死链接线、透视小计照 Pivot v2。
- **节奏参考（估算）**：2 人并行 → Phase 0 约 4–5 周、Phase 1 约 8–11 周；3 人并行 → Phase 0 约 3–4 周、Phase 1 约 5–7 周。

---

## ✅ 行动清单

| # | 行动 | 负责方 | 时间窗 |
|---|------|--------|--------|
| 1 | 跑窗口函数方言探针（四种数据源）——决定 R-06 是否拆期 | 工程 | 立即（阻塞 R-06） |
| 2 | 拍板度量级表达式形态：类型枚举 + 表达式兜底（推荐） | 产品负责人 | 立即 |
| 3 | 落地 G1 校验器 + 写失败测试（TDD），同步补 `POST /api/expr/validate` | 工程 | Phase 0 首批 |
| 4 | 落地 G2 Guard + G3 脱敏核对 | 工程 | Phase 0 首批 |
| 5 | 建 `bi_query_log` 与 owner/tenant 迁移（不可逆项） | 工程 | Phase 0 首批 |
| 6 | 定 filter-state 与统一错误契约的类型（前端/后端各一份） | 工程 | Phase 0 |
| 7 | Phase 0 出口验收 M0，再启动 Phase 1 | 产品负责人 | Phase 0 末 |
| 8 | Phase 1 按「指标层 → 看板 → 联动」推进，并行开 3 条轨 | 工程 | Phase 1 |

---

## ⚠️ 待确认 / 假设 / Non-goals

**待确认（阻塞或影响拆分）**
1. **窗口函数支持**：`running_total` / `percent_of_previous` 依赖它，四种驱动方言**未确认**。不支持 → R-06 拆两期（先 `simple` + `ratio`）。
2. **度量级表达式形态**：纯自由文本 vs **类型枚举 + 表达式兜底**。推荐后者（可校验、可 certified、UI 能提示）。
3. **看板全局筛选模型**：Metabase 式「控件→卡片映射」（推荐）vs Superset scope/DataMask。
4. **导出是否本期**：受控导出（限行 2000/10000 + 限字段 + 留痕）可放 Phase 2，**前提是留痕已上线**；否则与权限同批。

**假设**
- 当前仍为 L0 阶段（账号不出圈、单团队内网），故权限治理继续后置；一旦出现「第 2 个敏感度不同的数据集」即启动权限立项。
- 人日均为估算，未含需求澄清与联调等待。

**Non-goals（本期不做）**
- 权限治理（登录 / RBAC / RLS / 多租户）——触发式，见路线图 v2 §4。
- SQL Lab、嵌入 SDK、调度订阅、移动端原生 App、Jinja 模板引擎、ML 图表推荐、全量撤销/重做（先做 Reset）。
- 不追 Superset 的 40–50 图表长尾与 80+ 数据源军备竞赛。

---

## 📚 数据来源 & 成员产出索引
- 析客（需求分析师）：R-01~R-29 / T-01~T-09 新旧优先级映射表、需求池、三处裁定（导出受控、权限触发式、技术项不可后置）。
- 路径（路线图规划师）：Phase 0–4 路线图 v2、关键路径与并行机会、触发式阶段专项设计、M0–M4 里程碑、工作量 224–329 人日。
- 数析（数据分析师）：体验优先版权重与达标线（P0 四域各 ≥75%）、Gate G1–G3、体验可测量指标、验收用例集 v1.0（17/17）。
- 瑞思（用户研究员）：取舍边界、"电脑在你自己手上"判据、不可逆项（留痕 / owner 字段）、导出与权限同批建议。
- 竞析（竞品分析师）：17 条体验 table-stakes 与 Data Insights 逐条对照、度量级表达式层判定、可复用做法建议。
- **主理人仓库实测**：表结构（5 张表，无 owner/tenant）、`service/dataset/impl.go:206 UpdateColumns` 零校验、`query/bun_builder.go:26` 原样收 expr、前端零引用 `granularity`、`pieMergeOtherBelowRatio` 死链、`backend/internal/` 无 expr 校验包。

---

> 本报告由产品战略团队 AI 协作生成，重要决策请由产品负责人审定。
