# 图表扩展架构方案（基础对比 · 表格 · 统计分布）

> **状态**：技术方案，待实施  
> **日期**：2026-09-17  
> **范围**：R-21 多维度分组引擎 · R-48 count distinct · R-50 按值排序 · R-51 KPI 卡 · R-52 环形图 · R-53 透视表小计/合计 · R-54 中位数/百分位 · R-57 直方图 · R-58 双轴组合 · R-59 漏斗图 · R-62 雷达图 · 横条图（待编号）  
> **依据**：`deliverables/product-strategy/chart-coverage-gap-data-insights-2026-09-16.md`

---

## 一、现状诊断（代码实测）

### 1.1 当前架构分层

```
前端拖拽 (ChartBuilder.tsx)
  → composeChartQueryRequest()          # 把 dimensionGroups/metricGroups 平铺成 dims[]/metrics[]
  → POST /api/charts/query              # wire: { dataset_id, chart_type, dims[], metrics[], filters[], sort?, pagination? }
  → service/chart.executeQueryOnConn()  # entity → QuerySpec → QueryPlanner.PlanAST() → QueryAST
  → query.Executor.Execute()            # AST → BunQueryBuilder → SQL + args → Connection.Execute()
  → GetProcessor(chartType).Process()   # rows → AxisResponse / PieResponse / TableResponse / PivotResponse / ScatterResponse
  → 前端 ChartCanvas / TableChart       # ECharts option 或 antd Table
```

### 1.2 关键缺陷（阻碍扩展的结构性问题）

| # | 问题 | 位置 | 影响 |
|---|------|------|------|
| D1 | **wire 协议丢失槽位语义**：`dims[]` 是平铺字符串数组，processor 按 `dims[0]`/`dims[1:]` 猜测语义 | `entity/chart.go:18`、`processor.go:207-232` | 堆叠需要区分 x 轴维度与颜色分组维度，当前无法表达 |
| D2 | **fieldMeta 按列名索引，同字段多聚合冲突**：`Record<columnName, ChartMeta>` | `store/index.ts:141-144`、`chartConfigSchema.ts:53` | 同一字段拖入两次（如 sum 和 avg）共享同一份 meta，别名/单位互相覆盖 |
| D3 | **ChartBuilder 和 ShareView 各自重复构建 ECharts option** | `ChartBuilder.tsx:280-481`、`ShareView.tsx:173-391` | 新增图型必须改两处，容易遗漏；样式扩展无法共享 |
| D4 | **PivotProcessor 只做行透传，无真实交叉表** | `processor.go:351-368` | 透视表实际上与明细表行为相同，无行列交叉、无小计 |
| D5 | **无颜色/系列分组槽位** | `chartDefinitions.ts:41-59`（bar 只有 x_axis + values） | 堆叠/百分比堆叠无法配置 |
| D6 | **聚合白名单无 count distinct / 百分位** | `bun_builder.go:389`（`aggExprPattern`）、`types.go:19-25` | R-48、R-54 无法实现 |
| D7 | **store.executeChartQuery 把 AxisResponse 拆成平铺行** | `store/index.ts:815-828`（推断） | ChartCanvas 消费的是 `data: any[]`（行数组），而非结构化 AxisResponse；ShareView 消费的是结构化响应；两端数据形状不一致 |

---

## 二、目标架构

### 2.1 分层职责（不变）

```
ChartDefinition（声明式）
  → 槽位定义（id / kind / label / constraints）
  → 结果形状（resultShape）
  → 样式 schema（styleSchema）

ChartSpec（实例绑定）
  → 每个槽位的 BindingInstance[]（bindingId + field + agg + alias）
  → 样式值（style）
  → 查询选项（queryOptions）

QuerySpec（查询语义，数据库无关）
  → DimensionExpr[]（含 granularity / bucket）
  → MetricExpr[]（含 agg / alias）
  → FilterConfig[] / SortConfig / Pagination / Limit

QueryAST（SQL 中间表示）
  → 由 QueryPlanner 从 QuerySpec 生成
  → BunQueryBuilder 按方言渲染 SQL + args

Processor（结果整形）
  → 按 resultShape 把 rows 转换为前端可直接消费的结构

ChartRenderer（前端共享）
  → 纯函数：(chartType, processorResult, style, labels) → ECharts option | React node
  → ChartBuilder 和 ShareView 共用
```

### 2.2 BindingInstance：解决 D2

```typescript
// 每个拖入槽位的字段实例，有唯一 bindingId
interface BindingInstance {
  bindingId: string;   // 唯一标识，如 "b-1"、"b-2"（顺序递增，不用 UUID）
  field: string;       // 列名（稳定引用）
  agg?: AggType;       // 仅指标槽位
  alias?: string;
  unit?: string;
  format?: string;
  label?: string;      // 显示名
}

// fieldMeta 改为按 bindingId 索引
type FieldMeta = Record<string /* bindingId */, ChartMeta>;
```

**迁移策略**：`chartConfigSchema.ts` 的 `migrateChartConfig` 增加 v1→v2 路径：v1 的 `fieldMeta[columnName]` 在迁移时按组内出现顺序生成 bindingId（`b-0`、`b-1`…），旧列名键丢弃。v2 文档 `version: 2`。

### 2.3 ChartSpec v2 wire 格式：解决 D1

**新增请求体**（与旧 `ChartQueryRequest` 并存，旧格式继续支持）：

```json
{
  "dataset_id": 1,
  "chart_type": "bar",
  "spec_version": 2,
  "dimension_slots": [
    {
      "slot": "x_axis",
      "bindings": [{ "binding_id": "b-0", "field": "region", "label": "地区" }]
    },
    {
      "slot": "color_group",
      "bindings": [{ "binding_id": "b-1", "field": "product", "label": "产品" }]
    }
  ],
  "metric_slots": [
    {
      "slot": "values",
      "bindings": [{ "binding_id": "b-2", "field": "amount", "agg": "sum", "alias": "销售额" }]
    }
  ],
  "filters": [...],
  "sort": { "binding_id": "b-2", "order": "desc" },
  "pagination": { "page": 1, "page_size": 100 },
  "style": { "stack": "normal", "orientation": "vertical" }
}
```

**后端处理**：
- `handler/chart.go` 新增 `chartSpecQueryIn` struct（`form:"-"` 全字段），与旧 `chartQueryIn` 并存
- `service/chart.executeQueryOnConn` 检测 `spec_version`：v2 走新路径，v1/缺失走旧路径
- v2 路径：`ChartSpecFromRequestV2()` → `QuerySpec` → `QueryPlanner.PlanAST()` → `Executor`
- `QueryAST` 新增 `DimensionSlots []DimensionSlotAST`（保留槽位名），`Processor` 按槽位名消费

**SQL 别名规则**：每个 binding 的 SQL 别名 = `bindingId`（如 `b-2`），processor 按 bindingId 查找行值，彻底解决大小写折叠问题（`b-2` 是纯 ASCII，PG/MySQL/CH 折叠行为一致）。

### 2.4 共享渲染器：解决 D3、D7

新建 `frontend/src/lib/chartOptions.ts`：

```typescript
// 纯函数，不依赖 store，不依赖 React
export function buildChartOption(
  chartType: ChartType,
  data: ChartDataResult,      // processor 输出的结构化响应
  style: ChartStyleConfig,
  labels: Record<string, string>,  // bindingId → 显示名
): EChartsOption | null;
```

`ChartBuilder.tsx` 的 `ChartCanvas` 和 `ShareView.tsx` 的 `getChartOption` 都调用此函数，删除各自的 switch/case。

**store.executeChartQuery 修正**：不再把 AxisResponse 拆成行数组；`chartData` 改为存储原始 `ChartDataResult`（结构化响应），ChartCanvas 直接传给 `buildChartOption`。

---

## 三、各图型实现规格

### 3.1 基础对比族

#### 堆叠 / 百分比堆叠（R-21 核心）

**槽位变更**（`chartDefinitions.ts`）：

```typescript
bar: {
  type: 'bar',
  label: '柱状图',
  fieldGroups: [
    { id: 'x_axis',      kind: 'dimension', label: 'X 轴维度',   minGroups: 1, maxFields: 1 },
    { id: 'color_group', kind: 'dimension', label: '颜色分组',   minGroups: 0, maxFields: 1, optional: true },
    { id: 'values',      kind: 'metric',    label: '数值',       minGroups: 1 },
  ],
},
// line / area 同理增加 color_group
```

**style 扩展**：

```typescript
interface ChartStyleConfig {
  colors: string[];
  smooth: boolean;
  tableRowSize: 'small' | 'middle' | 'large';
  // 新增
  stack?: 'none' | 'normal' | 'percent';   // 堆叠模式
  orientation?: 'vertical' | 'horizontal'; // 横条图
  showDataLabels?: boolean;                // 数据标签（R-55）
}
```

**后端 processor 变更**：

`AxisProcessor` 增加对 `color_group` 槽位的感知：
- 无 color_group：现有逻辑（series = metrics）
- 有 color_group：series = color_group 值 × metrics（现有 `dims[1:]` 逻辑已覆盖，但需要槽位名而非位置推断）

百分比堆叠在**前端** `buildChartOption` 中计算（processor 返回原始值，前端按 x 轴位置归一化），不在后端做——避免后端需要知道展示意图。

**ECharts option**：

```typescript
// stack: 'normal'
series: [{ stack: 'total', ... }]
// stack: 'percent'
series: [{ stack: 'total', data: percentData, ... }]
// orientation: 'horizontal'
xAxis: { type: 'value' }, yAxis: { type: 'category', data: categories }
```

#### 横条图（待编号）

不新增 chartType，是 `bar` + `style.orientation = 'horizontal'` 的渲染变体。`buildChartOption` 中交换 xAxis/yAxis。

#### 环形图（R-52）

不新增 chartType，是 `pie` + `style.donut = true` 的渲染变体：

```typescript
// style 扩展
donut?: boolean;
// ECharts
series: [{ type: 'pie', radius: donut ? ['40%', '70%'] : '50%' }]
```

#### 双轴组合图（R-58）

新增 chartType `combo`：

```typescript
combo: {
  type: 'combo',
  label: '组合图',
  fieldGroups: [
    { id: 'x_axis',          kind: 'dimension', label: 'X 轴维度',   minGroups: 1, maxFields: 1 },
    { id: 'color_group',     kind: 'dimension', label: '颜色分组',   minGroups: 0, maxFields: 1, optional: true },
    { id: 'primary_values',  kind: 'metric',    label: '主轴指标',   minGroups: 1 },
    { id: 'secondary_values',kind: 'metric',    label: '次轴指标',   minGroups: 1 },
  ],
},
```

**resultShape**：复用 `AxisResponse`（x_axis + series），series 中前 N 个来自主轴、后 M 个来自次轴，由 `buildChartOption` 按 slot 名分配 yAxisIndex。

**后端**：`AxisProcessor` 已支持多 metrics；combo 的 primary/secondary 区分在 `ChartSpec` 的 slot 名中，processor 不需要感知——两个 slot 的 metrics 合并进同一个 `metrics[]` 传给 processor，slot 信息保留在 `ChartSpec` 中供前端渲染器使用。

### 3.2 表格族

#### 真实透视表 + 小计/合计（R-53）

**当前 PivotProcessor 问题**：只返回 `rows`，列名从 `rows[0]` 的 map 键遍历（顺序不保证），无交叉表逻辑。

**目标 PivotResponse v2**：

```go
type PivotResponse struct {
    RowHeaders   []string              `json:"row_headers"`    // 行维度列名
    ColHeaders   []string              `json:"col_headers"`    // 列维度值（交叉后的列）
    MetricNames  []string              `json:"metric_names"`   // 指标别名
    Cells        []PivotRow            `json:"cells"`          // 数据行（含小计行）
    GrandTotal   *PivotRow             `json:"grand_total"`    // 合计行（null 表示未请求）
}

type PivotRow struct {
    RowKey    []string          `json:"row_key"`     // 行维度值
    IsSubtotal bool             `json:"is_subtotal"` // 是否小计行
    Values    map[string]float64 `json:"values"`     // colHeader+metricAlias → 值
}
```

**后端实现策略**：

1. **GROUPING SETS 优先**（PG / MySQL 8.0+ / ClickHouse）：
   ```sql
   SELECT row_dim, col_dim, metric,
          GROUPING(row_dim) as is_row_subtotal,
          GROUPING(col_dim) as is_col_subtotal
   FROM source
   GROUP BY GROUPING SETS (
     (row_dim, col_dim),  -- 明细
     (row_dim),           -- 行小计
     ()                   -- 合计
   )
   ```
2. **UNION ALL 回退**（StarRocks / 旧 MySQL）：分别查明细、行小计、合计，Go 端合并。

**方言能力探测**：在 `datasource/driver.go` 的 `Connection` 接口新增：

```go
// Capabilities 返回数据源的查询能力（懒加载，首次调用时探测并缓存）
Capabilities(ctx context.Context) (*DialectCapabilities, error)

type DialectCapabilities struct {
    SupportsGroupingSets bool
    SupportsPercentileCont bool   // PG / ClickHouse
    SupportsWindowFunctions bool  // MySQL 8+ / StarRocks
    PercentileStrategy string     // "percentile_cont" | "quantilesExact" | "window_ntile" | "unsupported"
}
```

探测方式：执行 `SELECT 1` 级别的轻量探针 SQL（如 `SELECT GROUPING(1)` 在 PG 上成功即支持），结果缓存在 Connection 实例上（连接生命周期内不重复探测）。

**小计的聚合语义**（关键正确性约束）：
- `sum`：直接加总 ✓
- `count`：直接加总 ✓
- `avg`：**不能加总**，必须用 `SUM(field) / COUNT(field)` 重算，或 `AVG(field)` 在 GROUPING SETS 中自动重算 ✓（GROUPING SETS 的 AVG 是对整个分组重算，不是对子分组平均求平均）
- `count distinct`：**不能加总**，必须在每个 grouping level 重新 `COUNT(DISTINCT field)` ✓（GROUPING SETS 自动处理）
- `min` / `max`：直接取极值 ✓

**前端**：`TableChart.tsx` 增加 `pivotMode` prop，消费 `PivotResponse` 构建 antd Table 的 `columns`（含 colSpan 合并）和 `dataSource`（含小计行样式）。

### 3.3 统计分布族

#### 直方图（R-57）

**查询策略**（两阶段）：

1. **阶段一**：查 min/max（`SELECT MIN(field), MAX(field), COUNT(*) FROM source WHERE ...`）
2. **阶段二**：按 bin 宽度分组（`SELECT FLOOR((field - min) / bin_width) AS bin, COUNT(*) AS cnt FROM source WHERE ... GROUP BY bin ORDER BY bin`）

bin 宽度由前端配置（`binCount` 默认 20，或 `binWidth` 手动指定），传给后端作为 `queryOptions`。

**方言差异**：`FLOOR` 在 PG/MySQL/CH/StarRocks 均支持，无方言问题。整数除法需注意：PG 的 `integer / integer` 是整数除法，需要 `FLOOR((field - min)::numeric / bin_width)` 或 `FLOOR((field - min) * 1.0 / bin_width)`。统一用 `* 1.0` 强制浮点。

**新增 chartType**：`histogram`

```typescript
histogram: {
  type: 'histogram',
  label: '直方图',
  fieldGroups: [
    { id: 'value', kind: 'metric', label: '数值字段', minGroups: 1, maxFields: 1 },
  ],
  queryOptions: { binCount?: number; binWidth?: number },
},
```

**resultShape**：

```go
type HistogramResponse struct {
    Bins []HistogramBin `json:"bins"`
}
type HistogramBin struct {
    BinStart float64 `json:"bin_start"`
    BinEnd   float64 `json:"bin_end"`
    Count    int64   `json:"count"`
}
```

#### 箱线图（依赖 R-54）

**前置**：R-54 中位数/百分位。

**百分位方言策略**（需探针确认，以下为设计目标，不是已有事实）：

| 方言 | 策略 | 备注 |
|------|------|------|
| PostgreSQL | `percentile_cont(0.25) WITHIN GROUP (ORDER BY field)` | 标准 SQL，精确 Type-7 |
| ClickHouse | `quantilesExactInclusive(0.25, 0.5, 0.75)(field)` | 精确，Type-7 兼容 |
| MySQL 8+ | 窗口函数：`PERCENT_RANK()` + 插值，或 `NTILE(4)` 近似 | 需探针确认版本 |
| StarRocks | 继承 MySQL 策略，但版本能力需独立探针 | **不能假设与 MySQL 相同** |

**探针 SQL**（在 `Capabilities()` 中执行）：

```sql
-- PG / CH：直接执行，成功即支持
SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY 1)

-- MySQL / StarRocks：检测窗口函数
SELECT PERCENT_RANK() OVER (ORDER BY 1) FROM (SELECT 1) t
```

**查询策略**（单次查询，不分阶段）：

```sql
-- PG
SELECT
  MIN(field) AS whisker_low,
  percentile_cont(0.25) WITHIN GROUP (ORDER BY field) AS q1,
  percentile_cont(0.5)  WITHIN GROUP (ORDER BY field) AS median,
  percentile_cont(0.75) WITHIN GROUP (ORDER BY field) AS q3,
  MAX(field) AS whisker_high
FROM source WHERE ...

-- 离群点（IQR 法）：单独查询
SELECT field FROM source
WHERE field < q1 - 1.5*(q3-q1) OR field > q3 + 1.5*(q3-q1)
LIMIT 1000  -- 展示截断，返回 total_outliers 计数
```

**新增 chartType**：`boxplot`

```go
type BoxplotResponse struct {
    WhiskerLow  float64   `json:"whisker_low"`
    Q1          float64   `json:"q1"`
    Median      float64   `json:"median"`
    Q3          float64   `json:"q3"`
    WhiskerHigh float64   `json:"whisker_high"`
    Outliers    []float64 `json:"outliers"`
    OutlierTotal int64    `json:"outlier_total"`  // 截断前的总数
    Truncated   bool      `json:"truncated"`
}
```

#### 漏斗图（R-59）

**新增 chartType**：`funnel`

槽位：`stages`（维度，有序）+ `value`（指标，单值）。

```typescript
funnel: {
  type: 'funnel',
  label: '漏斗图',
  fieldGroups: [
    { id: 'stages', kind: 'dimension', label: '阶段', minGroups: 1, maxFields: 1 },
    { id: 'value',  kind: 'metric',    label: '数值', minGroups: 1, maxFields: 1 },
  ],
},
```

**resultShape**：复用 `PieResponse`（name + value），前端按 value 降序排列后渲染 ECharts funnel series。

**排序**：漏斗图强制按 value 降序（`sort: { field: value_binding_id, order: 'desc' }`），在 `composeChartQueryRequest` 中自动注入，用户不可覆盖。

#### 雷达图（R-62）

**新增 chartType**：`radar`

槽位：`indicators`（维度）+ `values`（指标，可多个系列）。

```typescript
radar: {
  type: 'radar',
  label: '雷达图',
  fieldGroups: [
    { id: 'indicators',   kind: 'dimension', label: '指标维度', minGroups: 1, maxFields: 1 },
    { id: 'series_group', kind: 'dimension', label: '系列分组', minGroups: 0, maxFields: 1, optional: true },
    { id: 'values',       kind: 'metric',    label: '数值',     minGroups: 1, maxFields: 1 },
  ],
},
```

**resultShape**：

```go
type RadarResponse struct {
    Indicators []RadarIndicator `json:"indicators"` // 维度值列表
    Series     []RadarSeries    `json:"series"`
}
type RadarIndicator struct {
    Name string  `json:"name"`
    Max  float64 `json:"max"`  // 该维度的最大值（用于 ECharts radar indicator.max）
}
type RadarSeries struct {
    Name   string    `json:"name"`
    Values []float64 `json:"values"` // 与 Indicators 等长
}
```

### 3.4 KPI 单值卡（R-51）

**新增 chartType**：`kpi`

```typescript
kpi: {
  type: 'kpi',
  label: 'KPI 卡',
  fieldGroups: [
    { id: 'value', kind: 'metric', label: '指标', minGroups: 1, maxFields: 1 },
  ],
},
```

**无维度**，查询退化为 `SELECT AGG(field) AS b-0 FROM source WHERE ... LIMIT 1`。

**resultShape**：

```go
type KpiResponse struct {
    Value     float64 `json:"value"`
    Label     string  `json:"label"`
    Unit      string  `json:"unit,omitempty"`
    Format    string  `json:"format,omitempty"`
}
```

**前端渲染**：不用 ECharts，用 antd `Statistic` 组件。`buildChartOption` 对 `kpi` 返回 `null`，ChartBuilder/ShareView 走独立的 `KpiCard` 组件分支。

### 3.5 按值排序（R-50）

**当前问题**：`composeChartQueryRequest` 的自动查询 effect 从不发送 `sort`（`includeSort: false`），只有手动点"执行查询"才带 sort。

**修正**：
- `sort` 的 `field` 改为引用 `bindingId`（而非列名），后端 `renderSortRef` 按 bindingId 查找对应的 SQL 别名
- 自动查询 effect 改为 `includeSort: true`
- TableChart 的 `onSortChange` 回调更新 `queryConfig.sort`，触发自动重查

### 3.6 count distinct（R-48）

**后端**：
- `AggregationType` 新增 `AggCountDistinct = "count_distinct"`
- `GetAggFunc()` 返回 `"COUNT(DISTINCT"` + 特殊处理（因为 `COUNT(DISTINCT field)` 不是简单的 `FUNC(field)` 形式）
- `bun_builder.go` 的 `buildSelectParts` 增加 `count_distinct` 分支：`COUNT(DISTINCT safeIdentifier(fieldExpr)) AS alias`
- `aggExprPattern` 白名单增加 `count_distinct`（或改为在 `buildSelectParts` 中直接处理，不经过 pattern）

**前端**：`FieldPill.tsx` 的 `AGGREGATION_OPTIONS` 增加 `{ value: 'count_distinct', label: '去重计数' }`。

---

## 四、数据库可移植性设计

### 4.1 方言能力协商（DialectCapabilities）

在 `datasource/driver.go` 的 `Connection` 接口新增 `Capabilities()` 方法（见 §3.2）。各驱动实现：

```go
// postgresql.go
func (c *PostgreSQLConnection) Capabilities(ctx context.Context) (*DialectCapabilities, error) {
    return &DialectCapabilities{
        SupportsGroupingSets:   true,   // PG 9.5+，Data Insights 最低支持 PG 12
        SupportsPercentileCont: true,
        SupportsWindowFunctions: true,
        PercentileStrategy:     "percentile_cont",
    }, nil
}

// clickhouse.go
func (c *ClickHouseConnection) Capabilities(ctx context.Context) (*DialectCapabilities, error) {
    return &DialectCapabilities{
        SupportsGroupingSets:   true,   // CH 21.x+
        SupportsPercentileCont: false,
        SupportsWindowFunctions: true,
        PercentileStrategy:     "quantilesExactInclusive",
    }, nil
}

// mysql.go / starrocks.go：需要运行时探针
func (c *MySQLConnection) Capabilities(ctx context.Context) (*DialectCapabilities, error) {
    if c.caps != nil { return c.caps, nil }  // 缓存
    caps := &DialectCapabilities{}
    // 探针：GROUPING SETS
    if _, err := c.Execute(ctx, "SELECT 1 FROM (SELECT 1 AS x) t GROUP BY GROUPING SETS ((x), ())"); err == nil {
        caps.SupportsGroupingSets = true
    }
    // 探针：窗口函数
    if _, err := c.Execute(ctx, "SELECT PERCENT_RANK() OVER (ORDER BY 1) FROM (SELECT 1) t"); err == nil {
        caps.SupportsWindowFunctions = true
        caps.PercentileStrategy = "window_ntile"
    } else {
        caps.PercentileStrategy = "unsupported"
    }
    c.caps = caps
    return caps, nil
}
```

**StarRocks 不能继承 MySQL 的 Capabilities**：StarRocks 的 `ParseDialect` 归入 `DialectMySQL`（`dialect.go`），但 StarRocks 的窗口函数和 GROUPING SETS 支持版本与 MySQL 不同，必须独立探针。

### 4.2 查询计划降级策略

```
QueryPlanner.PlanAST()
  → 检查 AST 中是否有需要特殊能力的节点（percentile / grouping_sets）
  → 调用 conn.Capabilities()
  → 按能力选择 SQL 生成策略：
      grouping_sets: GROUPING SETS 路径
      no grouping_sets: UNION ALL 路径（多次查询，Go 端合并）
      percentile_cont: 直接生成
      window_ntile: 窗口函数路径
      unsupported: 返回明确错误（不静默降级为近似值）
```

**原则**：能力不足时**显式报错**，不静默降级。错误信息告知用户"当前数据源不支持箱线图，需要 PostgreSQL / ClickHouse / MySQL 8+"。

### 4.3 资源保护

- 直方图/箱线图的离群点查询有 `LIMIT 1000`，返回 `truncated: true` + `outlier_total`
- 百分位查询在大表上可能很慢：在 `queryOptions` 中增加 `maxScanRows`（默认 10,000,000），超过时拒绝并提示用户加过滤条件
- 所有统计查询走 `Connection.Execute(ctx, sql, args...)`，ctx 携带超时（当前 service 层已有 30s 超时）

---

## 五、前端架构变更

### 5.1 ChartDefinition 扩展

```typescript
// chartDefinitions.ts
export interface ChartFieldGroupDefinition {
  id: string;
  kind: FieldGroupKind;
  label: string;
  emptyText: string;
  minGroups: number;
  // 新增
  maxFields?: number;    // 该槽位最多接受几个字段（undefined = 无限）
  optional?: boolean;    // 是否可选槽位（minGroups=0 时自动为 true）
}

export interface ChartDefinition {
  type: BuilderChartType;
  label: string;
  fieldGroups: ChartFieldGroupDefinition[];
  // 新增
  resultShape: ResultShape;           // 'axis' | 'pie' | 'table' | 'pivot' | 'scatter' | 'histogram' | 'boxplot' | 'radar' | 'kpi'
  icon: React.ReactNode;              // 从 ChartBuilder.tsx 的 chartTypeOptions 移入
  styleSchema?: Partial<ChartStyleConfig>; // 该图型支持哪些样式属性（用于 ConfigPanel 条件渲染）
}
```

### 5.2 QueryConfig 变更

```typescript
// store/index.ts
export interface FieldGroup {
  id: string;
  bindings: BindingInstance[];  // 替代 fields: string[]
}

export interface QueryConfig {
  dimensionGroups: FieldGroup[];
  metricGroups: FieldGroup[];
  filters: FilterCondition[];
  sort?: { bindingId: string; order: 'asc' | 'desc' };  // 改为引用 bindingId
  limit?: number;
}
```

**迁移**：`migrateChartConfig` v1→v2 把 `fields: string[]` 转换为 `bindings: BindingInstance[]`（bindingId 按顺序生成 `b-0`、`b-1`…），`fieldMeta[columnName]` 转换为 `fieldMeta[bindingId]`。

### 5.3 ConfigPanel 样式扩展

`ConfigPanel` 按 `chartDefinition.styleSchema` 条件渲染样式控件：
- `stack`：仅 bar/line/area/combo 显示
- `orientation`：仅 bar 显示
- `donut`：仅 pie 显示
- `binCount`：仅 histogram 显示（放在 queryOptions 而非 style）
- `tableRowSize`：仅 table/pivot 显示

### 5.4 新增图表类型注册

`ChartConfig['chartType']` 联合类型扩展：

```typescript
chartType: 'table' | 'line' | 'bar' | 'pie' | 'area' | 'scatter' | 'pivot'
         | 'combo' | 'histogram' | 'boxplot' | 'funnel' | 'radar' | 'kpi';
```

后端 `query/types.go` 同步新增常量：

```go
ChartTypeCombo     ChartType = "combo"
ChartTypeHistogram ChartType = "histogram"
ChartTypeBoxplot   ChartType = "boxplot"
ChartTypeFunnel    ChartType = "funnel"
ChartTypeRadar     ChartType = "radar"
ChartTypeKpi       ChartType = "kpi"
```

`GetProcessor` 新增对应分支；`chartConfigSchema.ts` 的 `CHART_TYPES` 数组同步更新。

---

## 六、实施阶段

### Phase 0：基础设施（必须先行，约 4–6 人日）

| 任务 | 文件 | 说明 |
|------|------|------|
| 0-1 | `frontend/src/lib/chartOptions.ts`（新建） | 提取 `buildChartOption` 纯函数，ChartBuilder + ShareView 共用 |
| 0-2 | `frontend/src/store/index.ts` | `FieldGroup.fields` → `FieldGroup.bindings`；`chartData` 改存结构化响应 |
| 0-3 | `frontend/src/lib/chartConfigSchema.ts` | v1→v2 迁移（bindingId 生成）；`ChartConfigDocument.version: 2` |
| 0-4 | `frontend/src/components/ChartBuilder/chartDefinitions.ts` | 增加 `maxFields`、`optional`、`resultShape`、`icon`、`styleSchema` |
| 0-5 | `backend/internal/query/types.go` | 新增 ChartType 常量；`AggregationType` 新增 `count_distinct` |
| 0-6 | `backend/internal/query/chart_spec.go` | `ChartSpecFromRequestV2()`；`DimensionSlotAST` 保留槽位名 |
| 0-7 | `backend/internal/datasource/driver.go` | `Connection` 接口新增 `Capabilities()` |
| 0-8 | `api/openapi.yaml` + `make api-gen` | 新增 v2 请求体 schema；新增各 Response schema |

**Phase 0 验收**：现有 7 种图型行为不变（所有现有测试通过）；`buildChartOption` 被 ChartBuilder 和 ShareView 共用；`Capabilities()` 在 PG 驱动上返回正确值。

### Phase 1：基础对比 + KPI + 排序 + count distinct（约 6–9 人日）

| 任务 | 依赖 | 说明 |
|------|------|------|
| 1-1 | Phase 0 | bar/line/area 增加 `color_group` 槽位；`style.stack` 渲染 |
| 1-2 | 1-1 | `style.orientation = 'horizontal'`（横条图） |
| 1-3 | Phase 0 | `style.donut`（环形图） |
| 1-4 | Phase 0 | `combo` 图型（双轴） |
| 1-5 | Phase 0 | `kpi` 图型 + `KpiCard` 组件 |
| 1-6 | Phase 0 | `count_distinct` 聚合（后端 + 前端 FieldPill） |
| 1-7 | Phase 0 | 按值排序（sort 改为 bindingId；自动查询 effect 带 sort） |

### Phase 2：表格增强（约 5–8 人日）

| 任务 | 依赖 | 说明 |
|------|------|------|
| 2-1 | Phase 0（Capabilities） | PivotProcessor v2：GROUPING SETS 路径 |
| 2-2 | 2-1 | UNION ALL 回退路径（MySQL/StarRocks 无 GROUPING SETS 时） |
| 2-3 | 2-1 | `PivotResponse` v2 + `TableChart.tsx` pivotMode |
| 2-4 | 2-1 | 小计/合计的 avg/count_distinct 正确性测试 |

### Phase 3：统计分布（约 8–13 人日）

| 任务 | 依赖 | 说明 |
|------|------|------|
| 3-0 | Phase 0（Capabilities） | **方言探针**：在 PG/MySQL/CH/StarRocks 上实测 percentile 和 GROUPING SETS 支持度，结果写入 `Capabilities()` 实现 |
| 3-1 | 3-0 | `histogram` 图型（两阶段查询 + HistogramProcessor） |
| 3-2 | 3-0 | `funnel` 图型（复用 PieProcessor + 强制降序） |
| 3-3 | 3-0 | `radar` 图型（RadarProcessor） |
| 3-4 | 3-0 | R-54 百分位聚合（`percentile_cont` / `quantilesExact` / 窗口回退） |
| 3-5 | 3-4 | `boxplot` 图型（BoxplotProcessor + 离群点截断） |

**Phase 3 前置门禁**：3-0 探针必须在真实数据库实例上运行，不能只靠代码推断。探针结果记录在 `docs/architecture.md` 的方言能力矩阵中。

---

## 七、文件变更清单

### 新建文件

| 文件 | 职责 |
|------|------|
| `frontend/src/lib/chartOptions.ts` | 共享 ECharts option 构建纯函数 |
| `frontend/src/components/ChartBuilder/KpiCard.tsx` | KPI 单值卡渲染组件 |
| `frontend/src/components/ChartBuilder/PivotTable.tsx` | 真实透视表渲染（从 TableChart 分离） |
| `backend/internal/query/processor_pivot.go` | PivotProcessor v2（从 processor.go 分离） |
| `backend/internal/query/processor_stats.go` | HistogramProcessor / BoxplotProcessor / RadarProcessor |
| `backend/internal/query/capabilities.go` | DialectCapabilities 类型 + 各驱动探针逻辑 |
| `backend/internal/query/percentile.go` | 百分位 SQL 生成（按方言策略） |

### 修改文件

| 文件 | 变更 |
|------|------|
| `frontend/src/store/index.ts` | FieldGroup.bindings；chartData 结构化；chartType 联合扩展 |
| `frontend/src/lib/chartConfigSchema.ts` | v2 文档；bindingId 迁移 |
| `frontend/src/components/ChartBuilder/chartDefinitions.ts` | 新图型定义；maxFields/optional/resultShape/icon/styleSchema |
| `frontend/src/pages/ChartBuilder.tsx` | composeChartQueryRequest v2；ChartCanvas 调用 buildChartOption；ConfigPanel 样式扩展 |
| `frontend/src/pages/ShareView.tsx` | 删除重复 option 构建，调用 buildChartOption |
| `frontend/src/components/ChartBuilder/TableChart.tsx` | pivotMode prop；小计行样式 |
| `frontend/src/components/ChartBuilder/FieldPill.tsx` | count_distinct 选项 |
| `backend/internal/query/types.go` | 新 ChartType 常量；AggCountDistinct |
| `backend/internal/query/chart_spec.go` | ChartSpecFromRequestV2；DimensionSlotAST |
| `backend/internal/query/ast.go` | DimensionSlots 字段 |
| `backend/internal/query/planner.go` | PlanAST 保留槽位名 |
| `backend/internal/query/bun_builder.go` | count_distinct SQL 生成；百分位 SQL 生成 |
| `backend/internal/query/processor.go` | GetProcessor 新分支；AxisProcessor 槽位感知 |
| `backend/internal/query/executor.go` | Capabilities 调用；统计查询路径 |
| `backend/internal/datasource/driver.go` | Connection 接口新增 Capabilities() |
| `backend/internal/datasource/postgresql.go` | Capabilities 实现 |
| `backend/internal/datasource/mysql.go` | Capabilities 实现（含探针） |
| `backend/internal/datasource/clickhouse.go` | Capabilities 实现 |
| `backend/internal/datasource/starrocks.go` | Capabilities 实现（独立探针，不继承 MySQL） |
| `backend/internal/service/chart/impl.go` | chartDataQueryFromConfig v2 路径；spec_version 检测 |
| `backend/internal/handler/chart.go` | chartSpecQueryIn struct |
| `api/openapi.yaml` | v2 请求体；新 Response schemas |

---

## 八、验收标准

### 正确性

- [ ] 同一字段拖入两个指标槽位，分别设 sum 和 avg，两个值独立正确（D2 修复验证）
- [ ] 堆叠柱状图：color_group 有值时 series 按颜色分组；无值时退化为普通柱状图
- [ ] 百分比堆叠：各 x 轴位置的 series 值之和 = 100%（浮点误差 < 0.01）
- [ ] 透视表小计：avg 指标的小计 = 该分组所有行的 AVG（不是子分组 AVG 的平均）
- [ ] 透视表小计：count_distinct 指标的小计 = 该分组所有行的 COUNT(DISTINCT)（不是子分组 COUNT DISTINCT 之和）
- [ ] 箱线图：Q1/Q3 与 PG `percentile_cont` 结果一致（Type-7）
- [ ] 直方图：所有 bin 的 count 之和 = 总行数（无遗漏、无重复）
- [ ] 漏斗图：stages 按 value 降序排列，用户无法通过 sort 配置覆盖
- [ ] KPI 卡：无维度时查询不报错，返回单值

### 可移植性

- [ ] PG：GROUPING SETS 路径通过；percentile_cont 路径通过
- [ ] MySQL 8+：探针确认 GROUPING SETS 和窗口函数支持；UNION ALL 回退路径通过
- [ ] ClickHouse：quantilesExactInclusive 路径通过
- [ ] StarRocks：独立探针（不继承 MySQL 结果）；能力不足时返回明确错误而非静默降级
- [ ] 能力不足时：前端显示"当前数据源不支持此图表"，不崩溃

### 兼容性

- [ ] 旧 v1 配置（`bi_chart.config`）加载后自动迁移为 v2，图表正常渲染
- [ ] 旧 `ChartQueryRequest`（无 `spec_version`）继续走旧路径，行为不变
- [ ] 分享链接（旧图表）正常渲染

### 共享渲染

- [ ] ChartBuilder 预览和 ShareView 对同一图表产生相同的 ECharts option（snapshot 测试）
- [ ] `buildChartOption` 是纯函数，不依赖 store 或 React context

---

## 九、待确认事项（实施前必须裁定）

| # | 问题 | 影响 | 建议 |
|---|------|------|------|
| Q1 | 双轴归属：R-21 验收子项 vs R-58 独立项 | combo 图型的槽位设计 | 建议归 R-58（combo 是独立图型，不是 bar 的变体）；R-21 验收子项仅含堆叠/百分比堆叠 |
| Q2 | count distinct 放 R-48 独立编号还是并入 R-06 指标语义层 | 聚合白名单扩展位置 | 建议 R-48 独立（R-06 是更大的指标语义层，count distinct 是其子集，先独立交付） |
| Q3 | MySQL/StarRocks 的百分位探针结果 | 箱线图在这两个数据源上是否可用 | Phase 3-0 探针后裁定；若不支持，箱线图在这两个数据源上显示"不支持"错误 |
| Q4 | `chartData` 从行数组改为结构化响应是否破坏现有测试 | Phase 0 风险 | 需要同步更新 `store/index.ts` 的 `executeChartQuery` 和所有消费 `chartData` 的测试 |
| Q5 | 横条图的编号 | 需求池纪律 | 建议下一轮路线图更新时补号（R-69 或插入现有空位） |

---

## 十、Non-goals（本方案明确不做）

- 地图（R-66，合规前置，触发式）
- 桑基图（R-67）、甘特图（R-68）
- 长尾六项（词云/子弹/人口金字塔/迷你图/OKR 表/玫瑰图）
- 条件格式（R-63，P2，不在三族范围内）
- 参考线/目标线（R-56，P1，不在三族范围内）
- 数据标签（R-55，P1，style 扩展已预留 `showDataLabels` 字段，实现不在本方案）
- 3D / 动效图表
- 新图表库（ECharts 已全量引入）
