package query

// ChartType 可视化图表类型
type ChartType string

const (
	ChartTypeTable   ChartType = "table"
	ChartTypeBar     ChartType = "bar"
	ChartTypeLine    ChartType = "line"
	ChartTypePie     ChartType = "pie"
	ChartTypeArea    ChartType = "area"
	ChartTypeScatter ChartType = "scatter"
	ChartTypePivot   ChartType = "pivot"

	ChartTypeCombo     ChartType = "combo"
	ChartTypeHistogram ChartType = "histogram"
	ChartTypeBoxplot   ChartType = "boxplot"
	ChartTypeFunnel    ChartType = "funnel"
	ChartTypeRadar     ChartType = "radar"
	ChartTypeKpi       ChartType = "kpi"
)

// AggregationType 聚合函数类型
type AggregationType string

const (
	AggSum   AggregationType = "sum"
	AggAvg   AggregationType = "avg"
	AggCount AggregationType = "count"
	AggMax   AggregationType = "max"
	AggMin   AggregationType = "min"

	// AggCountDistinct 生成 COUNT(DISTINCT field)：buildSelectParts 对该值走专用分支
	// （不套用通用 "%s(%s)" 模板），GetAggFunc() 返回防御性的 "COUNT(DISTINCT"（见其注释）。
	AggCountDistinct AggregationType = "count_distinct"
)

// FilterOperator 过滤条件操作符
type FilterOperator string

const (
	FilterEq        FilterOperator = "eq"
	FilterNeq       FilterOperator = "neq"
	FilterGt        FilterOperator = "gt"
	FilterGte       FilterOperator = "gte"
	FilterLt        FilterOperator = "lt"
	FilterLte       FilterOperator = "lte"
	FilterLike      FilterOperator = "like"
	FilterIn        FilterOperator = "in"
	FilterBetween   FilterOperator = "between"
	FilterIsNull    FilterOperator = "isNull"
	FilterIsNotNull FilterOperator = "isNotNull"
)

// MetricConfig 指标配置
type MetricConfig struct {
	Field string          `json:"field"`
	Agg   AggregationType `json:"agg"`
	Alias string          `json:"alias,omitempty"`
}

// FilterConfig 过滤条件配置
type FilterConfig struct {
	Field    string         `json:"field"`
	Op       FilterOperator `json:"op"`
	Value    interface{}    `json:"value"`
	ValueEnd interface{}    `json:"value_end,omitempty"`
	Logic    string         `json:"logic"`
}

// Pagination 分页配置
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// SortConfig 排序配置
type SortConfig struct {
	Field string `json:"field"`
	Order string `json:"order"` // asc | desc
}

// ChartQueryRequest 图表查询请求
type ChartQueryRequest struct {
	DatasetID  int            `json:"dataset_id"`
	ChartType  ChartType      `json:"chart_type"`
	Dims       []string       `json:"dims"`
	Metrics    []MetricConfig `json:"metrics"`
	Filters    []FilterConfig `json:"filters"`
	Pagination *Pagination    `json:"pagination,omitempty"`
	Sort       *SortConfig    `json:"sort,omitempty"`
	PlannedAST *QueryAST      `json:"-"`

	// QueryOptions 查询选项扩展袋（R-57）：histogram 的 bin_count（默认 20）/
	// bin_width（可选覆盖）经此下传，executor 的 histogram 分支直接读取——
	// 不贯穿 QuerySpec/QueryAST/planner（最小 churn 路径，与 pivot 分支从 req
	// 直读 ChartType/Dims/Metrics 同款）。
	QueryOptions map[string]any `json:"query_options,omitempty"`
}

// ChartQueryResponse 图表查询响应
type ChartQueryResponse interface{}

// TableResponse Table 图表响应
type TableResponse struct {
	Columns    []string         `json:"columns"`
	Data       []map[string]any `json:"data"`
	Pagination TablePagination  `json:"pagination"`
}

// TablePagination Table 分页信息
type TablePagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// PieResponse Pie 图表响应
type PieResponse struct {
	Data  []PieDataItem `json:"data"`
	Other *PieDataItem  `json:"other,omitempty"`
}

// PieDataItem Pie 数据项
type PieDataItem struct {
	Name       string  `json:"name"`
	Value      float64 `json:"value"`
	Percentage float64 `json:"percentage"`
}

// AxisResponse 坐标轴图表响应 (Bar, Line, Area)
type AxisResponse struct {
	XAxis  []string     `json:"x_axis"`
	Series []AxisSeries `json:"series"`
}

// AxisSeries 系列数据
type AxisSeries struct {
	Name string `json:"name"`
	Data []any  `json:"data"`
}

// ScatterResponse Scatter 图表响应
type ScatterResponse struct {
	Data [][]float64 `json:"data"`
}

// PivotResponse Pivot 图表响应
type PivotResponse struct {
	Columns []string         `json:"columns"`
	Data    []map[string]any `json:"data"`
}

// PivotResponseV2 透视表 v2 响应（R-53，plan §3.2）：交叉表头 + 含小计的数据行 + 合计行。
// 字段与 api/openapi.yaml 的 ChartPivotResponseV2 schema 逐一对应。
// 由 PivotProcessorV2 返回：GROUPING SETS 与 UNION ALL 两条 builder 路径都产此形状
// （executor 按数据源 Capabilities().SupportsGroupingSets 选择 builder）；
// v1 平铺请求 / PlannedAST 为 nil / 槽位不可解析时仍走旧 PivotResponse 平铺形状。
type PivotResponseV2 struct {
	RowHeaders  []string   `json:"row_headers"`  // 行维度列名
	ColHeaders  []string   `json:"col_headers"`  // 列维度值（交叉后的列，多列维度时用 " - " 连接）
	MetricNames []string   `json:"metric_names"` // 指标别名
	Cells       []PivotRow `json:"cells"`        // 数据行（明细行按 RowKey 交叉合并，含小计行）
	GrandTotal  *PivotRow  `json:"grand_total"`  // 合计行；null 表示未请求/无数据
}

// PivotRow 透视表 v2 数据行，与 openapi ChartPivotRow schema 对应。
// Values 是扁平 map：键为 "<colHeader>|<metricAlias>"（明细行）或
// "<PivotSubtotalColKey>|<metricAlias>"（小计/合计行，见 processor_pivot.go）。
type PivotRow struct {
	RowKey     []string           `json:"row_key"`
	IsSubtotal bool               `json:"is_subtotal"`
	Values     map[string]float64 `json:"values"`
}

// KpiResponse KPI 单值卡响应（无维度，标量聚合结果）。
// 字段与 api/openapi.yaml 的 ChartKpiResponse schema 逐一对应：
// value/label 必填，unit/format 为 omitempty（缺省时不出现在 JSON）。
type KpiResponse struct {
	Value  float64 `json:"value"`
	Label  string  `json:"label"`
	Unit   string  `json:"unit,omitempty"`
	Format string  `json:"format,omitempty"`
}

// HistogramResponse 直方图响应（R-57，plan §3.3）：Bins 为补全后的完整分箱序列
// （空 bin 以 Count=0 占位，x 轴连续），sum(Bins[i].Count) == 参与分箱的数值行数。
// 字段与 api/openapi.yaml 的 ChartHistogramResponse schema 逐一对应。
type HistogramResponse struct {
	Bins []HistogramBin `json:"bins"`
}

// HistogramBin 直方图分箱：[BinStart, BinEnd) 半开区间（最后一个 bin 的 BinEnd
// 因浮点边界值归箱钳制而实际闭区间），Count 为落入该箱的行数。
// 与 openapi ChartHistogramBin schema 对应。
type HistogramBin struct {
	BinStart float64 `json:"bin_start"`
	BinEnd   float64 `json:"bin_end"`
	Count    int64   `json:"count"`
}

// ResolveAlias 解析字段别名
func (m *MetricConfig) ResolveAlias() string {
	if m.Alias != "" {
		return m.Alias
	}
	return m.Field
}

// GetAggFunc 获取聚合函数名
func (a AggregationType) GetAggFunc() string {
	switch a {
	case AggSum:
		return "SUM"
	case AggAvg:
		return "AVG"
	case AggCount:
		return "COUNT"
	case AggMax:
		return "MAX"
	case AggMin:
		return "MIN"
	case AggCountDistinct:
		// COUNT(DISTINCT field) 不是简单的 "FUNC(field)" 形态，不能套用调用方的
		// "%s(%s)" 模板（会拼出括号不配对的畸形 SQL）。buildSelectParts 已对该值走
		// 专用分支，正常不会到达此处；返回故意不带右括号的 "COUNT(DISTINCT" 作为防御性
		// 信号——若将来有新调用点误用通用模板，会立刻产出明显畸形 SQL 而在测试中暴露，
		// 而不是静默降级成 SUM。
		return "COUNT(DISTINCT"
	default:
		return "SUM"
	}
}

// ToString 将 FilterOperator 转换为 SQL 操作符
func (op FilterOperator) ToString() string {
	switch op {
	case FilterEq:
		return "="
	case FilterNeq:
		return "<>"
	case FilterGt:
		return ">"
	case FilterGte:
		return ">="
	case FilterLt:
		return "<"
	case FilterLte:
		return "<="
	case FilterLike:
		return "LIKE"
	case FilterIn:
		return "IN"
	case FilterBetween:
		return "BETWEEN"
	case FilterIsNull:
		return "IS NULL"
	case FilterIsNotNull:
		return "IS NOT NULL"
	default:
		return "="
	}
}
