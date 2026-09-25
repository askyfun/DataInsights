# 图表构建器设计与现状

> 最后更新：2026-09-26

> 本文原为 2026-05 的《Chart Builder 增强实现计划》，其中绝大多数内容已落地。
> 现改写为"设计 + 现状"文档：以下描述以仓库当前代码为准（以 `backend/internal/query/`、
> `frontend/src/lib/`、`frontend/src/components/ChartBuilder/` 为准），尚未落地的能力显式标注。

## 一、架构与数据流

图表查询是一条"语义分层"链路：前端图表实例配置 → 结构化 ChartSpec → QuerySpec → SQL AST → 参数化 SQL → 按图型加工响应。

```
前端配置（ChartBuilder）
  ├── 维度组 / 指标组（槽位 + binding）
  ├── filters（含日期筛选意图）
  ├── sort / limit / pagination
  └── chartType

  ↓ POST /api/charts/query（v1 平铺协议 或 v2 槽位协议）

后端处理（backend/internal/query/）
  ├── executor.go：编排查询、按 spec_version 选择入口
  ├── chart_spec.go：ChartQueryRequest ↔ ChartSpec ↔ QuerySpec
  ├── planner.go：QuerySpec → PlannedQuery（兼容旧执行链）
  ├── ast.go：结构化 SQL AST
  ├── bun_builder.go：AST → 参数化 SQL（图表 SQL 的唯一出口）
  ├── datefilter.go：后端侧日期筛选意图解析（前端 lib/dateFilter.ts 的镜像）
  ├── executor：执行查询（Connection.Execute 参数化）
  └── processor.go：按 chartType 二次加工响应

  ↓

前端渲染（ECharts / Table 组件）
```

> 说明：早期计划里的"手写 `builder.go` / `QueryBuilder` 字符串拼 SQL"已作废。
> `dialect.go` 的手写字符串 SQL builder（`SQLBuilder`/`baseSQLBuilder`/`BuildQueryString`）已作为死代码删除，
> 仅保留 `DialectType` / `ParseDialect` / `BuildQueryStringWithBun`；图表 SQL 现由 `bun_builder.go` 统一生成。

## 二、API 契约

`api/openapi.yaml` 是前后端接口的单一事实源，`make api-gen` 生成 `backend/internal/idls/gen_types.go` 与
`frontend/src/idls/gen_types.ts`。

**请求**: `POST /api/charts/query`

v1（平铺协议）与 v2（槽位协议）共用同一超集结构（`backend/internal/domain/entity/chart.go` 的 `ChartQueryRequest`）：
`spec_version` 缺失或 `!=2` 时消费 `dims` / `metrics`；`spec_version=2` 时消费 `dimension_groups` / `metric_groups`（每组带 `binding_id`）。

```typescript
interface ChartQueryRequest {
  dataset_id: number;
  chart_type: string;
  // v1 平铺
  dims?: string[];
  metrics?: MetricConfig[];
  // v2 槽位
  spec_version?: number;
  dimension_groups?: DimensionGroup[];
  metric_groups?: MetricGroup[];
  // 共用
  filters: FilterConfig[];
  pagination?: Pagination;
  sort?: SortConfig;
}

interface MetricConfig {
  field: string;
  agg: 'sum' | 'avg' | 'count' | 'max' | 'min';
  alias?: string;
}

interface FilterConfig {
  fieldId: string;   // 引用列的稳定 id（DatasetColumn.id），不是列名
  op: 'eq' | 'neq' | 'gt' | 'gte' | 'lt' | 'lte' | 'like' | 'in' | 'between' | ...;
  value: unknown;
  valueEnd?: unknown;
  logic: 'and' | 'or';
}
```

**响应**: 按 chartType 返回不同 shape（`ChartQueryResponse`）——
表格含分页元数据、饼图/漏斗返回 `{name,value,percentage}`、轴类返回 `xAxis`+`series`、
透视表返回行列结构、KPI 返回单值、直方图返回补全的 `bins[]`。兼容期内旧响应结构仍保留。

## 三、契约模型（现状）

早期规划的 ChartDefinition / ChartSpec / QuerySpec 分层模型已落地：

### ChartDefinition（前端 registry）

`frontend/src/components/ChartBuilder/chartDefinitions.ts` 声明每种图表需要哪些输入：
图表类型、维度组、指标组、结果 shape、字段数量与类型约束。`ChartBuilder` 据此动态渲染槽位标签与空态文案，
切换图型时按定义补齐最小维度/指标组数量。已覆盖表格、柱状、折线、面积、饼、散点、透视等基础图型。

### ChartSpec / QuerySpec（后端）

`backend/internal/query/chart_spec.go`：

- `ChartSpec` 属于 Chart 语义层，表达实例绑定：`ChartType` + `DimensionGroups` + `MetricGroups` + `Style` + `QueryOptions`。
  维度绑定 `DimensionField{Field, Label, Granularity, BindingID}`，指标绑定 `MetricField{Field, Label, Agg, Alias, Unit, Format, BindingID}`。
- `QuerySpec` 属于 Query 语义层，只表达查询语义（不含视觉类型）：`Dimensions []DimensionExpr`、`Metrics []MetricExpr2`、
  `Filters`、`Sort`、`Pagination`、`Limit`；`DimensionExpr` / `MetricExpr2` 携带 `GroupName`（来源槽位）与 `BindingID`，供 processor 按槽位消费。
- 转换函数：`QuerySpecFromRequest`（v1 平铺 → QuerySpec 兼容 adapter）、`ChartSpecFromRequestV2`（v2 请求 → ChartSpec，原样映射槽位名与 binding_id）、
  `QuerySpecFromChartSpecV2`（v2 ChartSpec → QuerySpec，保留 GroupName/BindingID）。

`backend/internal/query/planner.go`：`QueryPlanner` 把 `QuerySpec` 规划为旧执行链可消费的 `PlannedQuery`（维度、聚合、线性过滤、排序、分页），
为后续更复杂规划能力预留入口。

### 图表 config 文档（前端，version:2）

`frontend/src/lib/chartConfigSchema.ts` 的 `ChartConfigDocument` 是 `bi_chart.config` 的持久化文档（`version: 2`），
`migrateChartConfig` 把任意历史结构（旧无 version / v1 / 损坏输入）无损转 v2：

- 字段组 `fields: string[]` 升级为 `bindings: BindingInstance[]`，每个拖入槽位的字段实例持有全局唯一 `bindingId`（`b-0`/`b-1`…）；
- `fieldMeta` 的键由列名改为 `bindingId`——同一列拖入两个组得到两个独立 binding 与各自元数据；
- `sort` 以 `bindingId` 引用排序目标；`bindings[].fieldId` 与 `filters[].fieldId` 引用列的稳定 id（`DatasetColumn.id`），列改名不切断已存图表。

### 日期筛选（双端镜像）

- 前端语义单一事实源：`frontend/src/lib/dateFilter.ts`（`resolveDateFilter`、`expandDateFilterIntent` 等）。
- 后端镜像：`backend/internal/query/datefilter.go` 解析同一份"日期意图"，故分享页 / 仪表盘读 config 的路径同样是动态日期。
- 防漂移：双端共读用例表 `frontend/src/lib/__fixtures__/dateFilterCases.json`（`datefilter_test.go` + `dateFilter.cases.test.ts` 各跑一遍）。改语义必须同时改两边。

## 四、SQL 生成（现状）

`bun_builder.go` 是图表 SQL 的唯一出口：AST 经 bun ORM 编译为**参数化** SQL，值一律通过 `args` 传给
`Connection.Execute(ctx, sql, args...)`，不再手写字符串模板。安全约束：

- 标识符白名单校验（裸名 `datasource.IsValidIdentifier`；query 包 `safeIdentifier` 额外允许成对引号包裹的标识符）。
- 聚合表达式收紧为显式函数白名单（`count|sum|avg|min|max`），杜绝经列 `FieldExpr` 注入任意函数名。
- 透视图在 GROUPING SETS 缺失时自动回退 UNION ALL；受能力门控的图型（如 boxplot）按数据源方言能力落地。

> 早期计划在本文档第五节留下的"手写 SQL 基础模板 / agg→SQL 映射 / op→SQL 映射"表已删除——
> 现由 `bun_builder.go` 参数化生成，保留会误导。

## 五、查询执行与图表处理器（现状）

`executor.go` 编排 Builder → 数据源执行 → Processor（直方图为 `executeHistogram` 两阶段分支，箱线走 `bun_builder_boxplot`）。
`processor.go` 的 `GetProcessor(chartType)` 按图型分派：

| ChartType | 处理器 | 加工职责 |
|-----------|--------|----------|
| table | TableProcessor | 分页、total count |
| pie | PieProcessor | 百分比计算、（可选）长尾合并 |
| funnel | PieProcessor（复用） | 返回 `{name,value}`，降序由前端注入的 ORDER BY 决定 |
| bar / line / area | AxisProcessor | 透传数据 |
| combo | AxisProcessor（复用） | primary/secondary 区分在槽位名，processor 不感知 |
| scatter | ScatterProcessor | 透传数据 |
| pivot | PivotProcessor | 行列转换 |
| kpi | KpiProcessor | 单值 |
| histogram | 两阶段分箱 + HistogramProcessor | bins 补全（空 bin 以 0 占位） |
| boxplot / radar | 对应 processor | 受数据源方言能力门控 |

## 六、饼图"其他"合并：能力保留但未接线

`query.PieProcessor` 具备按比例阈值把小占比类目并入"其他"的能力（`MergeOtherBelowRatio`，`Process` 内消费，单测
`TestPieProcessor_WithMergeOtherBelowRatio` 覆盖）。但图表查询的**线协议已不再携带**该阈值——
早期"饼图 `query_options` 前后端闭环、后端消费阈值"的说法不成立：Batch 3 已移除这条从未真正生效的死链，
`NewPieProcessor()` 在查询路径上以默认 `MergeOtherBelowRatio: 0`（即不合并）构造。该能力保留但未接线，
重新接线需要恢复请求侧的阈值字段。

## 七、各图表类型落地要点

- **table**：`COUNT(*)` 取总数，`LIMIT/OFFSET` 经参数化下推；前端 Ant Design Table + 分页器。
- **pie**：聚合后按首维度分组算百分比；长尾合并见第六节（未接线）。
- **bar / line / area**：基础查询，首维度作 X 轴、多指标作 series；ECharts 标准配置。
- **scatter**：2 个指标作 X/Y。
- **pivot**：GROUPING SETS 行列转换（缺失回退 UNION ALL）。
- **histogram**：两阶段分箱（MIN/MAX/COUNT → 分箱计数），`query_options.bin_count` / `bin_width` 经 v1 请求传入并生效。

## 八、配置项

### 8.1 Pie 图表配置

```typescript
interface PieChartConfig {
  showPercentage: boolean; // 显示百分比
  otherLabel: string;      // 其他标签，默认 "Other"
  // mergeOtherBelowRatio：阈值能力存在但未接线（见第六节）
}
```

### 8.2 Table 图表配置

```typescript
interface TableChartConfig {
  pageSize: number;        // 每页行数
  showPagination: boolean; // 显示分页
  showTotal: boolean;      // 显示总数
}
```
