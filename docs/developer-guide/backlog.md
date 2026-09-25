# 活跃待办与能力矩阵（Backlog）

> 最后更新：2026-09-26
> 只放仍在推进的**活跃项**；已完成项当日移出，避免出现"仪表盘 ❌"式的自我矛盾。
> 长期产品路线见 [概览](../getting-started/overview.md) 的「对外 Roadmap」。

## 1. 数据源方言能力矩阵

> 4 种外部数据源（PostgreSQL / MySQL / ClickHouse / StarRocks）的连接与元数据读取均已实现；
> 本清单跟踪**方言能力探测**与**能力门控图表**（箱线图 boxplot、透视 pivot）在各后端的落地与验证状态。

### 1.1 能力 → 图表依赖

| 能力字段 | 门控图表 | 不支持时行为 |
|---|---|---|
| `SupportsGroupingSets` | 透视 pivot | `executor.go` 自动回退 UNION ALL，**仍能出图** |
| `PercentileStrategy != "unsupported"` | 箱线图 boxplot | 前置门 `CheckPercentileSupport` 拦截，**不出图** |
| connect / GetTables / GetColumns / Execute | 所有图表基础 | — |

### 1.2 四源验证状态

图例：✅ 真实例实测 · 🟡 文档推断未跑实例 · ⬜ 保守 false · 🔎 懒探针（无实例走基线）

| 源 | GroupingSets | Percentile | Window | 探测机制 |
|---|---|---|---|---|
| PostgreSQL | ✅ | ✅ `percentile_cont` | ✅ | **真实懒探针 + sync.Once**（唯一） |
| StarRocks | ⬜（探针可升） | ✅ `percentile_cont_args_first`（已实例实测，作基线不重探） | ⬜（探针可升） | 🔎 只升不降 + nil 保底 |
| ClickHouse | 🟡（文档-true，loud） | ⬜（CH 无 `percentile_cont`，事实） | 🟡 | **刻意静态**（无可安全只升探测的布尔能力；percentile 需语义校验） |
| MySQL | ⬜（无 GROUPING SETS，文档事实） | ⬜（无标量 percentile 路径） | ⬜→🔎（8+ 探针可升 true） | 🔎 只升不降 + nil 保底 |

受能力门控的图表落地参差：**boxplot 仅 PostgreSQL 端到端可用**；ClickHouse 待语义校验；MySQL 无标量 percentile 路径，暂不支持；StarRocks percentile 策略已实测但翻转需真实实例。pivot 全源可用（数据源不支持 GROUPING SETS 时自动回退 UNION ALL）。

## 2. 活跃能力缺口

- **日期筛选「包含空日期」的 OR 优先级**：意图展开为两条平铺条件（区间 `AND` + `isNull` `OR`），`bun_builder` 按平铺列表拼 WHERE，SQL 优先级下等价 `(其他条件 AND 区间) OR isNull`——日期为 NULL 的行会绕过其他所有筛选。双端（前端 `expandDateFilterIntent` / 后端 `datefilter.go`）同形状，修法需要条件组嵌套进 AST，属查询链路改造，勿在前端单点打补丁。
- **`window_ntile` 实现**（独立、较大，**需实例验证**）：MySQL 8+ / StarRocks 的 percentile 需**子查询 / lateral** 重塑 builder（现有单标量表达式模型容纳不下），先出设计方案再落地，不可盲写。
- **ClickHouse percentile 语义校验**（**需 CH 实例**）：`quantileExactInclusive` 的插值是否与预期 Type-7 一致，须用**已知数据集 → 预期 Q1/median/Q3** 的语义探针验证（语法探针不足以证伪静默错值），通过后才把 CH 的 `PercentileStrategy` 翻到 `quantilesExactInclusive`。
- **接实例翻转 flag**：MySQL 8+ 窗口、StarRocks GROUPING SETS / 窗口在真实实例上跑探针实测（脚手架已备：`TEST_MYSQL_URL` / `TEST_STARROCKS_URL` + `go test -tags integration`）。
- **端到端冒烟**：每种源各建一张 boxplot + pivot，确认出图正确（PG 现成；StarRocks percentile 可测；CH 待语义校验；MySQL boxplot 暂不支持）。
- 能力探针实现见 `backend/internal/datasource/` 各驱动的 `Capabilities()`。

## 3. 扩容候选（新驱动，未排期）

按画像优先级：SQL Server（企业 / 政务 BI 存量）> DuckDB（嵌入式分析，纯 Go 成本低）> Trino / Presto（湖仓）> Oracle（纯 Go 生态不友好）> Snowflake / BigQuery / Redshift（出海才需）。

注意：加驱动的真实瓶颈不是"能否连接"，而是 `bun_builder` 目前是 **PG 中心设计**（占位符 `$1`、标识符引号、percentile 语法各方言不同）能否方言化到新引擎。

## 4. 公共遗留项（未获授权不动）

- **代码格式**：少量文件未过 `gofmt -l`（`internal/datasource/driver.go`、`internal/query/*.go`）。
- **antd 弃用**：`message.*` 静态调用、`Alert message` / `Spin tip` 已弃用。
- **StarRocks**：`Capabilities()` 保守返全 false（保留 TODO）。
- **biome**：曾清零 error（原有 2 处 error 已修：`FilterDropZone` 过滤行改原生 button、`ChartBuilder` 去掉未用的 `setMetricAlias`）；仅剩 `styles/index.css` 的 noDescendingSpecificity 警告。**别再让 error 复活**，否则提交会被钩子挡死。
- ⚠️ **列表页 100 条截断（未修，需动分页契约）**：前端 `datasetsApi.getAll()` 不带 `limit` → 后端 handler 默认 `limit=100`（`handler/dataset.go` 的 `pagination()`），而前端是**纯本地分页**（`Table pagination.pageSize=10`）→ 数据集超过 100 个时**第 101 条起永远查不到**，"共 N 条"也只统计前 100 条。修它必须同时把排序与分页搬到服务端（客户端 `sorter` 只能救急）。
- ⚠️ **过滤算子笔误**：现行契约（`api/openapi.yaml`）对**未知算子**静默回退 `=`，因此 `ge` / `GT` / `>=` 等写法会全变等值、**返回错误数据**；契约层面当前**尚无**归一化或显式报错的处理。
